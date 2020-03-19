// Copyright (c) 2018, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package nls

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"reflect"

	"mvdan.cc/gogrep/gsyntax"
)

func (g *G) Replace(pattern string) {
	for i := range g.current {
		sub := &g.current[i]
		nodeCopy, _ := g.parseExpr(pattern)
		// since we'll want to set positions within the file's
		// FileSet
		scrubPositions(nodeCopy)

		g.fillParents(nodeCopy)
		nodeCopy = g.fillValues(nodeCopy, sub.Values)
		g.substNode(sub.Node, nodeCopy)
		sub.Node = nodeCopy
	}
}

// Suggest replaces the current set of matches with the given pattern. If the
// input came from files on disk, the files are updated. Otherwise, the modified
// matches are kept in the current set, to be shown as results.
func (g *G) Suggest(pattern string) {
	g.Replace(pattern)

	// below is the old Write

	seenRoot := make(map[nodePosHash]bool)
	filePaths := make(map[*ast.File]string)
	var next []Match
	for _, sub := range g.current {
		root := g.nodeRoot(sub.Node)
		hash := posHash(root)
		if seenRoot[hash] {
			continue // avoid dups
		}
		seenRoot[hash] = true
		file, ok := root.(*ast.File)
		if ok {
			path := g.Fset.Position(file.Package).Filename
			if path != "" {
				// write to disk
				filePaths[file] = path
				continue
			}
		}
		// pass it on, to print to stdout
		next = append(next, Match{Node: root})
	}
	for file, path := range filePaths {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			// TODO: return errors instead
			panic(err)
		}
		if err := printConfig.Fprint(f, g.Fset, file); err != nil {
			// TODO: return errors instead
			panic(err)
		}
	}
	g.current = next
}

var printConfig = printer.Config{
	Mode:     printer.UseSpaces | printer.TabIndent,
	Tabwidth: 8,
}

func (g *G) nodeRoot(node ast.Node) ast.Node {
	parent := g.parentOf(node)
	if parent == nil {
		return node
	}
	if _, ok := parent.(gsyntax.NodeList); ok {
		return parent
	}
	return g.nodeRoot(parent)
}

type topNode struct {
	Node ast.Node
}

func (t topNode) Pos() token.Pos { return t.Node.Pos() }
func (t topNode) End() token.Pos { return t.Node.End() }

func (g *G) fillValues(node ast.Node, values map[string]ast.Node) ast.Node {
	// node might not have a parent, in which case we need to set an
	// artificial one. Its pointer interface is a copy, so we must also
	// return it.
	top := &topNode{node}
	g.setParentOf(node, top)

	gsyntax.Inspect(node, func(node ast.Node) bool {
		id := fromWildNode(node)
		info := g.info(id)
		if info.name == "" {
			return true
		}
		prev := values[info.name]
		switch prev.(type) {
		case gsyntax.ExprList:
			node = gsyntax.ExprList([]ast.Expr{
				node.(*ast.Ident),
			})
		case gsyntax.StmtList:
			if ident, ok := node.(*ast.Ident); ok {
				node = &ast.ExprStmt{X: ident}
			}
			node = gsyntax.StmtList([]ast.Stmt{
				node.(*ast.ExprStmt),
			})
		}
		g.substNode(node, prev)
		return true
	})
	g.setParentOf(node, nil)
	return top.Node
}

func (g *G) substNode(oldNode, newNode ast.Node) {
	parent := g.parentOf(oldNode)
	g.setParentOf(newNode, parent)

	ptr := g.nodePtr(oldNode)
	switch x := ptr.(type) {
	case **ast.Ident:
		*x = newNode.(*ast.Ident)
	case *ast.Node:
		*x = newNode
	case *ast.Expr:
		*x = newNode.(ast.Expr)
	case *ast.Stmt:
		switch y := newNode.(type) {
		case ast.Expr:
			stmt := &ast.ExprStmt{X: y}
			g.setParentOf(stmt, parent)
			*x = stmt
		case ast.Stmt:
			*x = y
		default:
			panic(fmt.Sprintf("cannot replace stmt with %T", y))
		}
	case *[]ast.Expr:
		oldList := oldNode.(gsyntax.ExprList)
		var first, last []ast.Expr
		for i, expr := range *x {
			if expr == oldList[0] {
				first = (*x)[:i]
				last = (*x)[i+len(oldList):]
				break
			}
		}
		switch y := newNode.(type) {
		case ast.Expr:
			*x = append(first, y)
		case gsyntax.ExprList:
			*x = append(first, y...)
		default:
			panic(fmt.Sprintf("cannot replace exprs with %T", y))
		}
		*x = append(*x, last...)
	case *[]ast.Stmt:
		oldList := oldNode.(gsyntax.StmtList)
		var first, last []ast.Stmt
		for i, stmt := range *x {
			if stmt == oldList[0] {
				first = (*x)[:i]
				last = (*x)[i+len(oldList):]
				break
			}
		}
		switch y := newNode.(type) {
		case ast.Expr:
			stmt := &ast.ExprStmt{X: y}
			g.setParentOf(stmt, parent)
			*x = append(first, stmt)
		case ast.Stmt:
			*x = append(first, y)
		case gsyntax.StmtList:
			*x = append(first, y...)
		default:
			panic(fmt.Sprintf("cannot replace stmts with %T", y))
		}
		*x = append(*x, last...)
	case nil:
		return
	default:
		panic(fmt.Sprintf("unsupported substitution: %T", x))
	}
	// the new nodes have scrubbed positions, so try our best to use
	// sensible ones
	fixPositions(parent)
}

func (g *G) parentOf(node ast.Node) ast.Node {
	list, ok := node.(gsyntax.NodeList)
	if ok {
		node = list.At(0)
	}
	return g.parents[node]
}

func (g *G) setParentOf(node, parent ast.Node) {
	list, ok := node.(gsyntax.NodeList)
	if ok {
		if list.Len() == 0 {
			return
		}
		node = list.At(0)
	}
	g.parents[node] = parent
}

func (g *G) nodePtr(node ast.Node) interface{} {
	list, wantSlice := node.(gsyntax.NodeList)
	if wantSlice {
		node = list.At(0)
	}
	parent := g.parentOf(node)
	if parent == nil {
		return nil
	}
	v := reflect.ValueOf(parent).Elem()
	for i := 0; i < v.NumField(); i++ {
		fld := v.Field(i)
		switch fld.Type().Kind() {
		case reflect.Slice:
			for i := 0; i < fld.Len(); i++ {
				ifld := fld.Index(i)
				if ifld.Interface() != node {
					continue
				}
				if wantSlice {
					return fld.Addr().Interface()
				}
				return ifld.Addr().Interface()
			}
		case reflect.Interface:
			if fld.Interface() == node {
				return fld.Addr().Interface()
			}
		}
	}
	return nil
}

// nodePosHash is an ast.Node that can always be used as a key in maps,
// even for nodes that are slices like NodeList.
type nodePosHash struct {
	pos, end token.Pos
}

func (n nodePosHash) Pos() token.Pos { return n.pos }
func (n nodePosHash) End() token.Pos { return n.end }

func posHash(node ast.Node) nodePosHash {
	return nodePosHash{pos: node.Pos(), end: node.End()}
}

var posType = reflect.TypeOf(token.NoPos)

func scrubPositions(node ast.Node) {
	gsyntax.Inspect(node, func(node ast.Node) bool {
		v := reflect.ValueOf(node)
		if v.Kind() != reflect.Ptr {
			return true
		}
		v = v.Elem()
		if v.Kind() != reflect.Struct {
			return true
		}
		for i := 0; i < v.NumField(); i++ {
			fld := v.Field(i)
			if fld.Type() == posType {
				fld.SetInt(0)
			}
		}
		return true
	})
}

// fixPositions tries to fix common syntax errors caused from syntax rewrites.
func fixPositions(node ast.Node) {
	if top, ok := node.(*topNode); ok {
		node = top.Node
	}
	// fallback sets pos to the 'to' position if not valid.
	fallback := func(pos *token.Pos, to token.Pos) {
		if !pos.IsValid() {
			*pos = to
		}
	}
	ast.Inspect(node, func(node ast.Node) bool {
		// TODO: many more node types
		switch x := node.(type) {
		case *ast.GoStmt:
			fallback(&x.Go, x.Call.Pos())
		case *ast.ReturnStmt:
			if len(x.Results) == 0 {
				break
			}
			// Ensure that there's no newline before the returned
			// values, as otherwise we have a naked return. See
			// https://github.com/golang/go/issues/32854.
			if pos := x.Results[0].Pos(); pos > x.Return {
				x.Return = pos
			}
		}
		return true
	})
}
