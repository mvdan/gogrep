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

func grep(expr string, src string) (int, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return 0, fmt.Errorf("cannot tokenize expr: %v", err)
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
	exprStr := strings.TrimSpace(buf.String())
	exprNode, err := parse(exprStr)
	if err != nil {
		return 0, fmt.Errorf("cannot parse expr: %v", err)
	}
	srcNode, err := parse(src)
	if err != nil {
		return 0, fmt.Errorf("cannot parse src: %v", err)
	}
	matches := 0
	match := func(srcNode ast.Node) {
		m := matcher{values: map[string]ast.Node{}}
		if m.node(exprNode, srcNode) {
			matches++
		}
	}
	ast.Inspect(srcNode, func(srcNode ast.Node) bool {
		match(srcNode)
		for _, list := range exprLists(srcNode) {
			match(list)
		}
		return true
	})
	return matches, nil
}
