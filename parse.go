// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"go/ast"
	"go/parser"
)

func parse(src string) (ast.Node, error) {
	return parser.ParseExpr(src)
}
