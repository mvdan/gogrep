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

var fileTempl = template.Must(template.New("stmts").Parse(`
package _gogrep

func _gogrep() {
	{{ . }}
}`))

func asFile(src string) string {
	var buf bytes.Buffer
	if err := fileTempl.Execute(&buf, src); err != nil {
		panic(err)
	}
	return buf.String()
}

func parse(src string) (ast.Node, error) {
	// try as an expr first, as our template method breaks when
	// given only types
	if expr, err := parser.ParseExpr(src); err == nil {
		return expr, nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", asFile(src), 0)
	if err != nil {
		return nil, err
	}
	return f.Decls[0].(*ast.FuncDecl).Body.List[0], nil
}
