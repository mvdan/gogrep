// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/gogrep"

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"log"
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
			buf.WriteString(t.tok.String())
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
