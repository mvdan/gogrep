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

func execTmpl(tmpl *template.Template, src string) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, src); err != nil {
		panic(err)
	}
	return buf.String()
}

func parse(src string) (ast.Node, error) {
	// TODO: remove this once we can do any number of types with a file
	if expr, err := parser.ParseExpr(src); err == nil {
		return expr, nil
	}
	fset := token.NewFileSet()

	// try as an expr first, as our template method breaks when
	// given only types
	asExprs := execTmpl(tmplExprs, src)
	if f, err := parser.ParseFile(fset, "", asExprs, 0); err == nil {
		return f.Decls[0].(*ast.GenDecl).Specs[0], nil
	}

	asStmts := execTmpl(tmplStmts, src)
	f, err := parser.ParseFile(fset, "", asStmts, 0)
	if err != nil {
		return nil, err
	}
	return f.Decls[0].(*ast.FuncDecl).Body.List[0], nil
}
