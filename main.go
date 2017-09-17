// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/gogrep"

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"log"
	"strings"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		log.Fatal("needs two args")
	}
	match, err := grep(args[0], args[1])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(match)
}

func grep(expr string, src string) (bool, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	for _, t := range toks {
		var s string
		switch {
		case t.tok == tokWildcard:
			s = wildName(t.lit)
		case t.lit != "":
			s = t.lit
		default:
			buf.WriteString(token.Token(t.tok).String())
		}
		buf.WriteString(s)
		buf.WriteByte(' ') // for e.g. consecutive idents
	}
	astExpr, err := parser.ParseExpr(buf.String())
	if err != nil {
		return false, err
	}
	astSrc, err := parser.ParseExpr(src)
	if err != nil {
		return false, err
	}
	m := matcher{values: map[string]ast.Node{}}
	return m.node(astExpr, astSrc), nil
}

type matcher struct{
	values map[string]ast.Node
}

func (m *matcher) node(expr, node ast.Node) bool {
	if expr == nil || node == nil {
		return expr == node
	}
	switch x := expr.(type) {
	case *ast.Ident:
		if !isWildName(x.Name) {
			y, ok := node.(*ast.Ident)
			return ok && x.Name == y.Name
		}
		name := fromWildName(x.Name)
		prev, ok := m.values[name]
		if !ok {
			m.values[name] = node
			return true
		}
		return m.node(prev, node)

	case *ast.BasicLit:
		// TODO: also try with resolved constants?
		y, ok := node.(*ast.BasicLit)
		return ok && x.Kind == y.Kind && x.Value == y.Value
	case *ast.CompositeLit:
		y, ok := node.(*ast.CompositeLit)
		return ok && m.node(x.Type, y.Type) && m.exprs(x.Elts, y.Elts)

	case *ast.ArrayType:
		y, ok := node.(*ast.ArrayType)
		return ok && m.node(x.Len, y.Len) && m.node(x.Elt, y.Elt)

	case *ast.UnaryExpr:
		y, ok := node.(*ast.UnaryExpr)
		return ok && x.Op == y.Op && m.node(x.X, y.X)
	case *ast.BinaryExpr:
		y, ok := node.(*ast.BinaryExpr)
		return ok && x.Op == y.Op && m.node(x.X, y.X) && m.node(x.Y, y.Y)
	case *ast.CallExpr:
		y, ok := node.(*ast.CallExpr)
		return ok && m.node(x.Fun, y.Fun) && m.exprs(x.Args, y.Args)
	case *ast.KeyValueExpr:
		y, ok := node.(*ast.KeyValueExpr)
		return ok && m.node(x.Key, y.Key) && m.node(x.Value, y.Value)
	case *ast.StarExpr:
		y, ok := node.(*ast.StarExpr)
		return ok && m.node(x.X, y.X)

	default:
		panic(fmt.Sprintf("unexpected node: %T", x))
	}
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

// exprToken exists to add extra possible tokens on top of the ones
// recognized by vanilla Go.
type exprToken token.Token

const (
	_ exprToken = -iota
	tokWildcard
)

type fullToken struct {
	tok exprToken
	lit string
}

func tokenize(src string) ([]fullToken, error) {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var err error
	onError := func(pos token.Position, msg string) {
		switch msg { // allow some extra chars
		case `illegal character U+0024 '$'`:
		default:
			err = fmt.Errorf("%v: %s", pos, msg)
		}
	}
	s.Init(file, []byte(src), onError, scanner.ScanComments)

	var toks []fullToken
	gotDollar := false
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF || err != nil {
			break
		}
		fpos := fset.Position(pos)
		if gotDollar {
			if tok != token.IDENT {
				err = fmt.Errorf("%v: $ must be followed by ident, got %v",
					fpos, tok)
				break
			}
			gotDollar = false
			toks = append(toks, fullToken{tokWildcard, lit})
		} else if tok == token.ILLEGAL && lit == "$" {
			gotDollar = true
		} else {
			toks = append(toks, fullToken{exprToken(tok), lit})
		}
	}
	return toks, err
}
