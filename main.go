// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/gogrep"

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
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
		return false, fmt.Errorf("cannot tokenize expr: %v", err)
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
	// trailing newlines can cause issues with commas
	exprSrc := strings.TrimSpace(buf.String())
	astExpr, err := parse(exprSrc)
	if err != nil {
		return false, fmt.Errorf("cannot parse expr: %v", err)
	}
	astSrc, err := parse(src)
	if err != nil {
		return false, fmt.Errorf("cannot parse src: %v", err)
	}
	m := matcher{values: map[string]ast.Node{}}
	return m.node(astExpr, astSrc), nil
}
