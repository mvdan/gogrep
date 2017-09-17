// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/gogrep"

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"log"
	"strings"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		log.Fatal("needs two args")
	}
	match, err := grep(args[0], args[1])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(match)
}

func grep(expr string, src string) (bool, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	for _, t := range toks {
		var s string
		switch {
		case t.tok == tokWildcard:
			s = wildName(t.lit)
		case t.lit != "":
			s = t.lit
		default:
			buf.WriteString(t.tok.String())
		}
		buf.WriteString(s)
		buf.WriteByte(' ') // for e.g. consecutive idents
	}
	astExpr, err := parser.ParseExpr(buf.String())
	if err != nil {
		return false, err
	}
	astSrc, err := parser.ParseExpr(src)
	if err != nil {
		return false, err
	}
	m := matcher{values: map[string]ast.Node{}}
	return m.node(astExpr, astSrc), nil
}

type matcher struct {
	values map[string]ast.Node
}

func (m *matcher) node(expr, node ast.Node) bool {
	if expr == nil || node == nil {
		return expr == node
	}
	switch x := expr.(type) {
	case *ast.Ident:
		if !isWildName(x.Name) {
			// not a wildcard
			y, ok := node.(*ast.Ident)
			return ok && x.Name == y.Name
		}
		name := fromWildName(x.Name)
		prev, ok := m.values[name]
		if !ok {
			// first occurrence, record value
			m.values[name] = node
			return true
		}
		// multiple uses must match
		return m.node(prev, node)

	// lits
	case *ast.BasicLit:
		// TODO: also try with resolved constants?
		y, ok := node.(*ast.BasicLit)
		return ok && x.Kind == y.Kind && x.Value == y.Value
	case *ast.CompositeLit:
		y, ok := node.(*ast.CompositeLit)
		return ok && m.node(x.Type, y.Type) && m.exprs(x.Elts, y.Elts)

	// types
	case *ast.ArrayType:
		y, ok := node.(*ast.ArrayType)
		return ok && m.node(x.Len, y.Len) && m.node(x.Elt, y.Elt)
	case *ast.MapType:
		y, ok := node.(*ast.MapType)
		return ok && m.node(x.Key, y.Key) && m.node(x.Value, y.Value)
	case *ast.StructType:
		y, ok := node.(*ast.StructType)
		return ok && m.fields(x.Fields, y.Fields)
	case *ast.Field:
		// TODO: tags?
		y, ok := node.(*ast.Field)
		return ok && m.idents(x.Names, y.Names) && m.node(x.Type, y.Type)

	// other exprs
	case *ast.UnaryExpr:
		y, ok := node.(*ast.UnaryExpr)
		return ok && x.Op == y.Op && m.node(x.X, y.X)
	case *ast.BinaryExpr:
		y, ok := node.(*ast.BinaryExpr)
		return ok && x.Op == y.Op && m.node(x.X, y.X) && m.node(x.Y, y.Y)
	case *ast.CallExpr:
		y, ok := node.(*ast.CallExpr)
		return ok && m.node(x.Fun, y.Fun) && m.exprs(x.Args, y.Args)
	case *ast.KeyValueExpr:
		y, ok := node.(*ast.KeyValueExpr)
		return ok && m.node(x.Key, y.Key) && m.node(x.Value, y.Value)
	case *ast.StarExpr:
		y, ok := node.(*ast.StarExpr)
		return ok && m.node(x.X, y.X)
	case *ast.SelectorExpr:
		y, ok := node.(*ast.SelectorExpr)
		return ok && m.node(x.X, y.X) && m.node(x.Sel, y.Sel)
	case *ast.IndexExpr:
		y, ok := node.(*ast.IndexExpr)
		return ok && m.node(x.X, y.X) && m.node(x.Index, y.Index)
	case *ast.SliceExpr:
		y, ok := node.(*ast.SliceExpr)
		return ok && m.node(x.X, y.X) && m.node(x.Low, y.Low) &&
			m.node(x.High, y.High) && m.node(x.Max, y.Max)
	case *ast.TypeAssertExpr:
		y, ok := node.(*ast.TypeAssertExpr)
		return ok && m.node(x.X, y.X) && m.node(x.Type, y.Type)

	default:
		panic(fmt.Sprintf("unexpected node: %T", x))
	}
}

func (m *matcher) exprs(exprs1, exprs2 []ast.Expr) bool {
	if len(exprs1) != len(exprs2) {
		return false
	}
	for i, e1 := range exprs1 {
		if !m.node(e1, exprs2[i]) {
			return false
		}
	}
	return true
}

func (m *matcher) idents(ids1, ids2 []*ast.Ident) bool {
	if len(ids1) != len(ids2) {
		return false
	}
	for i, id1 := range ids1 {
		if !m.node(id1, ids2[i]) {
			return false
		}
	}
	return true
}

func (m *matcher) fields(fields1, fields2 *ast.FieldList) bool {
	if len(fields1.List) != len(fields2.List) {
		return false
	}
	for i, f1 := range fields1.List {
		if !m.node(f1, fields2.List[i]) {
			return false
		}
	}
	return true
}

const wildPrefix = "_gogrep_"

func wildName(name string) string {
	// good enough for now
	return wildPrefix + name
}

func isWildName(name string) bool {
	return strings.HasPrefix(name, wildPrefix)
}

func fromWildName(name string) string {
	return strings.TrimPrefix(name, wildPrefix)
}
