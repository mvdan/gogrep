// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/gogrep"

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/kisielk/gotool"
)

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: gogrep pattern [pkg ...]

A pattern is a Go expression or statement which may include wildcards.

Example:

	gogrep 'if $x != nil { return $x }'
`)
	}
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "need at least one arg")
		os.Exit(2)
	}
	if err := grepArgs(args[0], args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func grepArgs(expr string, args []string) error {
	exprNode, typed, err := compileExpr(expr)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	paths := gotool.ImportPaths(args)
	var matches []ast.Node
	if !typed {
		nodes, err := loadPaths(wd, fset, paths)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			matches = append(matches, search(exprNode, node)...)
		}
	} else {
		prog, err := loadTyped(wd, fset, paths)
		if err != nil {
			return err
		}
		for _, pkg := range prog.InitialPackages() {
			for _, file := range pkg.Files {
				matches = append(matches, search(exprNode, file)...)
			}
		}
	}
	for _, n := range matches {
		fpos := fset.Position(n.Pos())
		if strings.HasPrefix(fpos.Filename, wd) {
			fpos.Filename = fpos.Filename[len(wd)+1:]
		}
		fmt.Printf("%v: %s\n", fpos, singleLinePrint(n))
	}
	return nil
}

type bufferJoinLines struct {
	bytes.Buffer
	last string
}

var rxNeedSemicolon = regexp.MustCompile(`([])}a-zA-Z0-9"'` + "`" + `]|\+\+|--)$`)

func (b *bufferJoinLines) Write(p []byte) (n int, err error) {
	if string(p) == "\n" {
		if rxNeedSemicolon.MatchString(b.last) {
			b.Buffer.WriteByte(';')
		}
		b.Buffer.WriteByte(' ')
		return 1, nil
	}
	p = bytes.Trim(p, "\t")
	n, err = b.Buffer.Write(p)
	b.last = string(p)
	return
}

func singleLinePrint(node ast.Node) string {
	var buf bufferJoinLines
	printNode(&buf, token.NewFileSet(), node)
	return buf.String()
}

func printNode(w io.Writer, fset *token.FileSet, node ast.Node) {
	switch x := node.(type) {
	case exprList:
		if len(x) == 0 {
			return
		}
		printNode(w, fset, x[0])
		for _, n := range x[1:] {
			fmt.Fprintf(w, ", ")
			printNode(w, fset, n)
		}
	default:
		err := printer.Fprint(w, fset, node)
		if err != nil && strings.Contains(err.Error(), "go/printer: unsupported node type") {
			// Should never happen, but make it obvious when it does.
			panic(fmt.Errorf("cannot print node: %v\n", node, err))
		}
	}
}

func compileExpr(expr string) (node ast.Node, typed bool, err error) {
	toks, err := tokenize(expr)
	if err != nil {
		return nil, false, fmt.Errorf("cannot parse expr: %v", err)
	}
	var buf bytes.Buffer
	for _, t := range toks {
		var s string
		switch {
		case t.tok == tokWild:
			s = wildPrefix + t.lit
		case t.tok == tokWildAny:
			s = wildPrefix + wildExtraAny + t.lit
		case t.lit != "":
			s = t.lit
		default:
			buf.WriteString(t.tok.String())
		}
		buf.WriteString(s)
		buf.WriteByte(' ') // for e.g. consecutive idents
	}
	// trailing newlines can cause issues with commas
	exprStr := strings.TrimSpace(buf.String())
	if node, err = parse(exprStr); err != nil {
		return nil, false, fmt.Errorf("cannot parse expr: %v", err)
	}
	return node, typed, nil
}

func search(exprNode, node ast.Node) []ast.Node {
	var matches []ast.Node
	match := func(exprNode, node ast.Node) {
		m := matcher{values: map[string]ast.Node{}}
		if m.node(exprNode, node) {
			matches = append(matches, node)
		}
	}
	visit := func(node ast.Node) bool {
		match(exprNode, node)
		for _, list := range exprLists(node) {
			match(exprNode, list)
		}
		return true
	}
	// ast.Walk barfs on ast.Node types it doesn't know, so
	// do the first level manually here
	if list, ok := node.(nodeList); ok {
		if e, ok := exprNode.(ast.Expr); ok {
			// otherwise "$*a" won't match "a; b", as the
			// former isn't a list unless we make it one
			match(exprList([]ast.Expr{e}), list)
		}
		match(exprNode, list)
		for i := 0; i < list.len(); i++ {
			ast.Inspect(list.at(i), visit)
		}
	} else {
		ast.Inspect(node, visit)
	}
	return matches
}
