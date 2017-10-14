// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"text/template"
)

var tmplDecl = template.Must(template.New("").Parse(`` +
	`package p; {{ . }}`))

var tmplExprs = template.Must(template.New("").Parse(`` +
	`package p; var _ = []interface{}{ {{ . }}, }`))

var tmplStmts = template.Must(template.New("").Parse(`` +
	`package p; func _() { {{ . }} }`))

var tmplType = template.Must(template.New("").Parse(`` +
	`package p; var _ {{ . }}`))

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
	file := fset.AddFile("", fset.Base(), len(src))
	scan := scanner.Scanner{}
	scan.Init(file, []byte(src), nil, 0)
	if _, tok, _ := scan.Scan(); tok == token.EOF {
		return nil, fmt.Errorf("empty source code")
	}
	var mainErr error

	// first try as a whole file
	if f, err := parser.ParseFile(fset, "", src, 0); err == nil && noBadNodes(f) {
		return f, nil
	}

	// then as a declaration
	asDecl := execTmpl(tmplDecl, src)
	if f, err := parser.ParseFile(fset, "", asDecl, 0); err == nil {
		if dc := f.Decls[0]; noBadNodes(dc) {
			return dc, nil
		}
	}

	// then as value expressions
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
			return stmtList(bl.List), nil
		}
	} else {
		// Statements is what covers most cases, so it will give
		// the best overall error message. Show positions
		// relative to where the user's code is put in the
		// template.
		mainErr = subPosOffsets(err, posOffset{1, 1, 22})
	}

	// type expressions not yet picked up, for e.g. chans and interfaces
	asType := execTmpl(tmplType, src)
	if f, err := parser.ParseFile(fset, "", asType, 0); err == nil {
		vs := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
		if typ := vs.Type; noBadNodes(typ) {
			return typ, nil
		}
	}
	return nil, mainErr
}

type posOffset struct {
	atLine, atCol int
	offset        int
}

func subPosOffsets(err error, offs ...posOffset) error {
	list, ok := err.(scanner.ErrorList)
	if !ok {
		return err
	}
	for i, err := range list {
		for _, off := range offs {
			if err.Pos.Line != off.atLine {
				continue
			}
			if err.Pos.Column < off.atCol {
				continue
			}
			err.Pos.Column -= off.offset
		}
		list[i] = err
	}
	return list
}
