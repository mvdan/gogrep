// Copyright (c) 2018, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
)

func (m *matcher) cmdSubst(cmd exprCmd, subs []submatch) []submatch {
	for _, sub := range subs {
		nodeCopy, _ := m.parseExpr(cmd.src)
		m.fillParents(nodeCopy)
		m.fillValues(nodeCopy, sub.values)
		m.substNode(sub.node, nodeCopy)
		sub.node = nodeCopy
	}
	return subs
}

func (m *matcher) fillValues(node ast.Node, values map[string]ast.Node) {
	inspect(node, func(node ast.Node) bool {
		id := fromWildNode(node)
		info := m.info(id)
		if info.name == "" {
			return true
		}
		m.substNode(node, values[info.name])
		return true
	})
}

func (m *matcher) substNode(oldNode, newNode ast.Node) {
	ptr := m.nodePtr(oldNode)
	switch x := ptr.(type) {
	case *ast.Expr:
		*x = newNode.(ast.Expr)
	case *[]ast.Expr:
		oldList := oldNode.(exprList)
		var first, last []ast.Expr
		for i, expr := range *x {
			if expr == oldList[0] {
				first = (*x)[:i]
				last = (*x)[i+len(oldList):]
				break
			}
		}
		*x = append(first, newNode.(exprList)...)
		*x = append(*x, last...)
	case *[]ast.Stmt:
		oldList := oldNode.(stmtList)
		var first, last []ast.Stmt
		for i, stmt := range *x {
			if stmt == oldList[0] {
				first = (*x)[:i]
				last = (*x)[i+len(oldList):]
				break
			}
		}
		*x = append(first, newNode.(stmtList)...)
		*x = append(*x, last...)
	default:
		panic(fmt.Sprintf("unsupported substitution: %T", oldNode))
	}
}

func (m *matcher) nodePtr(node ast.Node) interface{} {
	wantSlice := false
	if list, ok := node.(nodeList); ok {
		node = list.at(0)
		wantSlice = true
	}
	parent := m.parents[node]
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
// even for nodes that are slices like nodeList.
type nodePosHash struct {
	pos, end token.Pos
}

func (n nodePosHash) Pos() token.Pos { return n.pos }
func (n nodePosHash) End() token.Pos { return n.end }

func posHash(node ast.Node) nodePosHash {
	return nodePosHash{pos: node.Pos(), end: node.End()}
}
