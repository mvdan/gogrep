// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"text/template"
)

var tmplExprs = template.Must(template.New("exprs").Parse(`
package _gogrep

var _gogrep = []interface{}{
	{{ . }},
}`))

var tmplStmts = template.Must(template.New("stmts").Parse(`
package _gogrep

func _gogrep() {
       {{ . }}
}`))

var tmplType = template.Must(template.New("exprs").Parse(`
package _gogrep

var _gogrep {{ . }}`))

func execTmpl(tmpl *template.Template, src string) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, src); err != nil {
		panic(err)
	}
	return buf.String()
}

func noBadNodes(node ast.Node) bool {
	any := false
	ast.Inspect(node, func(n ast.Node) bool {
		if any {
			return false
		}
		switch n.(type) {
		case *ast.BadExpr, *ast.BadDecl:
			any = true
		}
		return true
	})
	return !any
}

type exprList []ast.Expr

func (l exprList) Pos() token.Pos { return l[0].Pos() }
func (l exprList) End() token.Pos { return l[len(l)-1].End() }

func parse(src string) (ast.Node, error) {
	fset := token.NewFileSet()
	var mainErr error

	// try as value expressions first
	asExprs := execTmpl(tmplExprs, src)
	if f, err := parser.ParseFile(fset, "", asExprs, 0); err == nil {
		vs := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
		if cl := vs.Values[0].(*ast.CompositeLit); noBadNodes(cl) {
			if len(cl.Elts) == 1 {
				return cl.Elts[0], nil
			}
			return exprList(cl.Elts), nil
		}
	}

	// then try as statements
	asStmts := execTmpl(tmplStmts, src)
	if f, err := parser.ParseFile(fset, "", asStmts, 0); err == nil {
		if bl := f.Decls[0].(*ast.FuncDecl).Body; noBadNodes(bl) {
			if len(bl.List) == 1 {
				return bl.List[0], nil
			}
			return bl, nil
		}
	} else {
		// statements is what covers most cases, so it will give
		// the best overall error message
		mainErr = err
	}

	// type expressions as a last resort, for e.g. chans and interfaces
	asType := execTmpl(tmplType, src)
	if f, err := parser.ParseFile(fset, "", asType, 0); err == nil {
		vs := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
		if typ := vs.Type; noBadNodes(typ) {
			return typ, nil
		}
	}
	return nil, mainErr
}

func exprLists(n ast.Node) []exprList {
	var lists []exprList
	switch x := n.(type) {
	case *ast.CompositeLit:
		lists = append(lists, x.Elts)
	case *ast.CallExpr:
		lists = append(lists, x.Args)
	case *ast.AssignStmt:
		lists = append(lists, x.Lhs)
		lists = append(lists, x.Rhs)
	case *ast.ReturnStmt:
		lists = append(lists, x.Results)
	case *ast.CaseClause:
		lists = append(lists, x.List)
	case *ast.ValueSpec:
		lists = append(lists, x.Values)
	}
	return lists
}
