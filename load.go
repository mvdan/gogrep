// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/loader"
)

func loadPaths(fset *token.FileSet, paths []string) []*ast.File {
	// TODO: implement
	return nil
}

func loadTyped(fset *token.FileSet, paths []string) (*loader.Program, error) {
	conf := loader.Config{Fset: fset}
	if _, err := conf.FromArgs(paths, true); err != nil {
		return nil, err
	}
	return conf.Load()
}
