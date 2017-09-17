// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

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
		if name == "_" {
			// values are discarded, matches anything
			return true
		}
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
	case *ast.FuncLit:
		y, ok := node.(*ast.FuncLit)
		return ok && m.node(x.Type, y.Type) && m.node(x.Body, y.Body)

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
	case *ast.FuncType:
		y, ok := node.(*ast.FuncType)
		return ok && m.fields(x.Params, y.Params) &&
			m.fields(x.Results, y.Results)
	case *ast.InterfaceType:
		y, ok := node.(*ast.InterfaceType)
		return ok && m.fields(x.Methods, y.Methods)
	case *ast.ChanType:
		y, ok := node.(*ast.ChanType)
		return ok && x.Dir == y.Dir && m.node(x.Value, y.Value)

	// other exprs
	case *ast.Ellipsis:
		y, ok := node.(*ast.Ellipsis)
		return ok && m.node(x.Elt, y.Elt)
	case *ast.ParenExpr:
		y, ok := node.(*ast.ParenExpr)
		return ok && m.node(x.X, y.X)
	case *ast.UnaryExpr:
		y, ok := node.(*ast.UnaryExpr)
		return ok && x.Op == y.Op && m.node(x.X, y.X)
	case *ast.BinaryExpr:
		y, ok := node.(*ast.BinaryExpr)
		return ok && x.Op == y.Op && m.node(x.X, y.X) && m.node(x.Y, y.Y)
	case *ast.CallExpr:
		y, ok := node.(*ast.CallExpr)
		return ok && m.node(x.Fun, y.Fun) && m.exprs(x.Args, y.Args) &&
			m.noPos(x.Ellipsis, y.Ellipsis)
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

	// stmts
	case *ast.BadStmt:
		y, ok := node.(*ast.BadStmt)
		_, _ = y, ok
	case *ast.DeclStmt:
		y, ok := node.(*ast.DeclStmt)
		_, _ = y, ok
	case *ast.EmptyStmt:
		y, ok := node.(*ast.EmptyStmt)
		_, _ = y, ok
	case *ast.LabeledStmt:
		y, ok := node.(*ast.LabeledStmt)
		_, _ = y, ok
	case *ast.ExprStmt:
		y, ok := node.(*ast.ExprStmt)
		return ok && m.node(x.X, y.X)
	case *ast.SendStmt:
		y, ok := node.(*ast.SendStmt)
		_, _ = y, ok
	case *ast.IncDecStmt:
		y, ok := node.(*ast.IncDecStmt)
		_, _ = y, ok
	case *ast.AssignStmt:
		y, ok := node.(*ast.AssignStmt)
		_, _ = y, ok
	case *ast.GoStmt:
		y, ok := node.(*ast.GoStmt)
		_, _ = y, ok
	case *ast.DeferStmt:
		y, ok := node.(*ast.DeferStmt)
		_, _ = y, ok
	case *ast.ReturnStmt:
		y, ok := node.(*ast.ReturnStmt)
		_, _ = y, ok
	case *ast.BranchStmt:
		y, ok := node.(*ast.BranchStmt)
		_, _ = y, ok
	case *ast.BlockStmt:
		y, ok := node.(*ast.BlockStmt)
		return ok && m.stmts(x.List, y.List)
	case *ast.IfStmt:
		y, ok := node.(*ast.IfStmt)
		_, _ = y, ok
	case *ast.CaseClause:
		y, ok := node.(*ast.CaseClause)
		_, _ = y, ok
	case *ast.SwitchStmt:
		y, ok := node.(*ast.SwitchStmt)
		_, _ = y, ok
	case *ast.TypeSwitchStmt:
		y, ok := node.(*ast.TypeSwitchStmt)
		_, _ = y, ok
	case *ast.CommClause:
		y, ok := node.(*ast.CommClause)
		_, _ = y, ok
	case *ast.SelectStmt:
		y, ok := node.(*ast.SelectStmt)
		_, _ = y, ok
	case *ast.ForStmt:
		y, ok := node.(*ast.ForStmt)
		_, _ = y, ok
	case *ast.RangeStmt:
		y, ok := node.(*ast.RangeStmt)
		_, _ = y, ok

	default:
		panic(fmt.Sprintf("unexpected node: %T", x))
	}
	panic(fmt.Sprintf("unfinished node: %T", expr))
}

func (m *matcher) noPos(p1, p2 token.Pos) bool {
	return (p1 == token.NoPos) == (p2 == token.NoPos)
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
	if fields1 == nil || fields2 == nil {
		return fields1 == fields2
	}
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

func (m *matcher) stmts(stmts1, stmts2 []ast.Stmt) bool {
	if len(stmts1) != len(stmts2) {
		return false
	}
	for i, s1 := range stmts1 {
		if !m.node(s1, stmts2[i]) {
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
