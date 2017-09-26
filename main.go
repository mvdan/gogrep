// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/gogrep"

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/printer"
	"go/token"
	"io"
	"os"
	"regexp"
	"strings"
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
	if err := grepArgs(os.Stdout, &build.Default, args[0], args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func grepArgs(w io.Writer, ctx *build.Context, expr string, args []string) error {
	exprNode, typed, err := compileExpr(expr)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	var all []ast.Node
	if !typed {
		nodes, err := loadUntyped(wd, ctx, fset, args)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			all = append(all, matches(exprNode, node)...)
		}
	} else {
		prog, err := loadTyped(wd, ctx, fset, args)
		if err != nil {
			return err
		}
		for _, pkg := range prog.InitialPackages() {
			for _, file := range pkg.Files {
				all = append(all, matches(exprNode, file)...)
			}
		}
	}
	for _, n := range all {
		fpos := fset.Position(n.Pos())
		if strings.HasPrefix(fpos.Filename, wd) {
			fpos.Filename = fpos.Filename[len(wd)+1:]
		}
		fmt.Fprintf(w, "%v: %s\n", fpos, singleLinePrint(n))
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
	case stmtList:
		if len(x) == 0 {
			return
		}
		printNode(w, fset, x[0])
		for _, n := range x[1:] {
			fmt.Fprintf(w, "; ")
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

type lineColBuffer struct {
	bytes.Buffer
	line, col, offs int
}

func (l *lineColBuffer) WriteString(s string) (n int, err error) {
	for _, r := range s {
		if r == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.offs++
	}
	return l.Buffer.WriteString(s)
}

func compileExpr(expr string) (node ast.Node, typed bool, err error) {
	toks, err := tokenize(expr)
	if err != nil {
		return nil, false, fmt.Errorf("cannot parse expr: %v", err)
	}
	var offs []posOffset
	lbuf := lineColBuffer{line: 1, col: 1}
	addOffset := func(length int) {
		offs = append(offs, posOffset{
			atLine: lbuf.line,
			atCol:  lbuf.col,
			offset: length,
		})
	}
	for _, t := range toks {
		for lbuf.offs < t.pos.Offset {
			lbuf.WriteString(" ")
		}
		var s string
		switch {
		case t.tok == tokWild:
			s = wildPrefix + t.lit
			lbuf.offs -= len(wildPrefix) - 1
			addOffset(len(wildPrefix) - 1) // -1 for the $
		case t.tok == tokWildAny:
			s = wildPrefix + wildExtraAny + t.lit
			lbuf.offs -= len(wildPrefix+wildExtraAny) - 1
			addOffset(len(wildPrefix+wildExtraAny) - 1) // -1 for the $
		case t.lit != "":
			s = t.lit
		default:
			s = t.tok.String()
		}
		lbuf.WriteString(s)
	}
	// trailing newlines can cause issues with commas
	exprStr := strings.TrimSpace(lbuf.String())
	if node, err = parse(exprStr); err != nil {
		err = subPosOffsets(err, offs...)
		return nil, false, fmt.Errorf("cannot parse expr: %v", err)
	}
	return node, typed, nil
}
