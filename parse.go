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

var tmplTypes = template.Must(template.New("exprs").Parse(`
package _gogrep

var _gogrep func({{ . }})`))

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

func parse(src string) (ast.Node, error) {
	fset := token.NewFileSet()
	var mainErr error

	// try as expressions first
	asExprs := execTmpl(tmplExprs, src)
	if f, err := parser.ParseFile(fset, "", asExprs, 0); err == nil {
		if n := f.Decls[0].(*ast.GenDecl).Specs[0]; noBadNodes(n) {
			return n, nil
		}
	}

	// then try as statements
	asStmts := execTmpl(tmplStmts, src)
	if f, err := parser.ParseFile(fset, "", asStmts, 0); err == nil {
		if n := f.Decls[0].(*ast.FuncDecl).Body; noBadNodes(n) {
			return n, nil
		}
	} else {
		// statements is what covers most cases, so it will give
		// the best overall error message
		mainErr = err
	}

	// types as a last resort, for e.g. chans and interfaces
	asTypes := execTmpl(tmplTypes, src)
	if f, err := parser.ParseFile(fset, "", asTypes, 0); err == nil {
		return f.Decls[0].(*ast.GenDecl).Specs[0], nil
	}
	return nil, mainErr
}
