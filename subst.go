// Copyright (c) 2018, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"go/ast"
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
	ast.Inspect(node, func(node ast.Node) bool {
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
	default:
		panic(fmt.Sprintf("unsupported substitution: %T", oldNode))
	}
}

func (m *matcher) nodePtr(node ast.Node) interface{} {
	if _, ok := node.(nodeList); ok {
		return nil
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
				if ifld.Interface() == node {
					return ifld.Addr().Interface()
				}
			}
		case reflect.Interface:
			if fld.Interface() == node {
				return fld.Addr().Interface()
			}
		}
	}
	return nil
}
