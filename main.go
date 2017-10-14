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

var recurse = flag.Bool("r", false, "match all dependencies recursively too")

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: gogrep pattern [pkg ...]

A pattern is a Go expression or statement which may include wildcards.

  -r   match all dependencies recursively too

Example: gogrep 'if $x != nil { return $x }'
`)
	}
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "need at least one arg")
		os.Exit(2)
	}
	m := matcher{
		out: os.Stdout,
		ctx: &build.Default,
	}
	err := m.fromArgs(args[0], args[1:], *recurse)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type matcher struct {
	out io.Writer
	ctx *build.Context

	typed, aggressive bool

	// information about variables (wildcards), by id (which is an
	// integer starting at 0)
	vars []varInfo

	// node values recorded by name, excluding "_" (used only by the
	// actual matching phase)
	values map[string]ast.Node
}

type varInfo struct {
	name string
	any  bool
}

func (m *matcher) info(id int) varInfo {
	if id < 0 {
		return varInfo{}
	}
	return m.vars[id]
}

func (m *matcher) fromArgs(expr string, args []string, recurse bool) error {
	exprNode, err := m.compileExpr(expr)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	var all []ast.Node
	loader := nodeLoader{wd, m.ctx, fset}
	if !m.typed {
		nodes, err := loader.untyped(args, recurse)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			all = append(all, m.matches(exprNode, node)...)
		}
	} else {
		prog, err := loader.typed(args, recurse)
		if err != nil {
			return err
		}
		// TODO: recursive mode
		for _, pkg := range prog.InitialPackages() {
			for _, file := range pkg.Files {
				all = append(all, m.matches(exprNode, file)...)
			}
		}
	}
	for _, n := range all {
		fpos := loader.fset.Position(n.Pos())
		if strings.HasPrefix(fpos.Filename, wd) {
			fpos.Filename = fpos.Filename[len(wd)+1:]
		}
		fmt.Fprintf(m.out, "%v: %s\n", fpos, singleLinePrint(n))
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

func (m *matcher) compileExpr(expr string) (node ast.Node, err error) {
	toks, err := m.tokenize(expr)
	if err != nil {
		return nil, fmt.Errorf("cannot tokenize expr: %v", err)
	}
	var offs []posOffset
	lbuf := lineColBuffer{line: 1, col: 1}
	addOffset := func(length int) {
		lbuf.offs -= length
		offs = append(offs, posOffset{
			atLine: lbuf.line,
			atCol:  lbuf.col,
			offset: length,
		})
	}
	if len(toks) > 0 && toks[0].tok == tokAggressive {
		toks = toks[1:]
		m.aggressive = true
	}
	for _, t := range toks {
		for lbuf.offs < t.pos.Offset {
			lbuf.WriteString(" ")
		}
		if t.lit == "" {
			lbuf.WriteString(t.tok.String())
			continue
		}
		if isWildName(t.lit) {
			// to correct the position offsets for the extra
			// info attached to ident name strings
			addOffset(len(wildPrefix) - 1)
		}
		lbuf.WriteString(t.lit)
	}
	// trailing newlines can cause issues with commas
	exprStr := strings.TrimSpace(lbuf.String())
	if node, err = parse(exprStr); err != nil {
		err = subPosOffsets(err, offs...)
		return nil, fmt.Errorf("cannot parse expr: %v", err)
	}
	return node, nil
}
