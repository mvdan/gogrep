// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package gsyntax

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Inspect is like ast.Inspect, but it supports our extra NodeList Node
// type (only at the top level).
func Inspect(node ast.Node, fn func(ast.Node) bool) {
	// ast.Walk barfs on ast.Node types it doesn't know, so
	// do the first level manually here
	list, ok := node.(NodeList)
	if !ok {
		ast.Inspect(node, fn)
		return
	}
	if !fn(list) {
		return
	}
	for i := 0; i < list.Len(); i++ {
		ast.Inspect(list.At(i), fn)
	}
	fn(nil)
}

type (
	ExprList  []ast.Expr
	IdentList []*ast.Ident
	StmtList  []ast.Stmt
	SpecList  []ast.Spec
	FieldList []*ast.Field
)

type NodeList interface {
	At(i int) ast.Node
	Len() int
	Slice(from, to int) NodeList
	ast.Node
}

func (l ExprList) Len() int  { return len(l) }
func (l IdentList) Len() int { return len(l) }
func (l StmtList) Len() int  { return len(l) }
func (l SpecList) Len() int  { return len(l) }
func (l FieldList) Len() int { return len(l) }

func (l ExprList) At(i int) ast.Node  { return l[i] }
func (l IdentList) At(i int) ast.Node { return l[i] }
func (l StmtList) At(i int) ast.Node  { return l[i] }
func (l SpecList) At(i int) ast.Node  { return l[i] }
func (l FieldList) At(i int) ast.Node { return l[i] }

func (l ExprList) Slice(i, j int) NodeList  { return l[i:j] }
func (l IdentList) Slice(i, j int) NodeList { return l[i:j] }
func (l StmtList) Slice(i, j int) NodeList  { return l[i:j] }
func (l SpecList) Slice(i, j int) NodeList  { return l[i:j] }
func (l FieldList) Slice(i, j int) NodeList { return l[i:j] }

func (l ExprList) Pos() token.Pos  { return l[0].Pos() }
func (l IdentList) Pos() token.Pos { return l[0].Pos() }
func (l StmtList) Pos() token.Pos  { return l[0].Pos() }
func (l SpecList) Pos() token.Pos  { return l[0].Pos() }
func (l FieldList) Pos() token.Pos { return l[0].Pos() }

func (l ExprList) End() token.Pos  { return l[len(l)-1].End() }
func (l IdentList) End() token.Pos { return l[len(l)-1].End() }
func (l StmtList) End() token.Pos  { return l[len(l)-1].End() }
func (l SpecList) End() token.Pos  { return l[len(l)-1].End() }
func (l FieldList) End() token.Pos { return l[len(l)-1].End() }

type bufferJoinLines struct {
	bytes.Buffer
	last string
}

var rxNeedSemicolon = regexp.MustCompile(`([])}a-zA-Z0-9"'` + "`" + `]|\+\+|--)$`)

func (b *bufferJoinLines) Write(p []byte) (n int, err error) {
	if string(p) == "\n" {
		if b.last == "\n" {
			return 1, nil
		}
		if rxNeedSemicolon.MatchString(b.last) {
			b.Buffer.WriteByte(';')
		}
		b.Buffer.WriteByte(' ')
		b.last = "\n"
		return 1, nil
	}
	p = bytes.Trim(p, "\t")
	n, err = b.Buffer.Write(p)
	b.last = string(p)
	return
}

func (b *bufferJoinLines) String() string {
	return strings.TrimSuffix(b.Buffer.String(), "; ")
}

var emptyFset = token.NewFileSet()

func PrintCompact(node ast.Node) string {
	var buf bufferJoinLines
	Inspect(node, func(node ast.Node) bool {
		bl, ok := node.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return true
		}
		if !strings.HasPrefix(bl.Value, "`") {
			return true
		}
		if !strings.Contains(bl.Value, "\n") {
			return true
		}
		bl.Value = strconv.Quote(bl.Value[1 : len(bl.Value)-1])
		return true
	})
	Print(&buf, emptyFset, node)
	return buf.String()
}

func Print(w io.Writer, Fset *token.FileSet, node ast.Node) {
	switch x := node.(type) {
	case ExprList:
		if len(x) == 0 {
			return
		}
		Print(w, Fset, x[0])
		for _, n := range x[1:] {
			fmt.Fprintf(w, ", ")
			Print(w, Fset, n)
		}
	case StmtList:
		if len(x) == 0 {
			return
		}
		Print(w, Fset, x[0])
		for _, n := range x[1:] {
			fmt.Fprintf(w, "; ")
			Print(w, Fset, n)
		}
	default:
		err := printer.Fprint(w, Fset, node)
		if err != nil && strings.Contains(err.Error(), "go/printer: unsupported node type") {
			// Should never happen, but make it obvious when it does.
			panic(fmt.Errorf("cannot print node %T: %v", node, err))
		}
	}
}
