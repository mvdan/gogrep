// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/kisielk/gotool"
	"golang.org/x/tools/go/loader"
)

func loadUntyped(wd string, ctx *build.Context, fset *token.FileSet, args []string, recurse bool) ([]ast.Node, error) {
	gctx := gotool.Context{BuildContext: *ctx}
	paths := gctx.ImportPaths(args)
	var nodes []ast.Node
	addFile := func(path string) error {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		nodes = append(nodes, f)
		return nil
	}
	done := map[string]bool{}
	var addPkg func(path string) error // to recurse into self
	addPkg = func(path string) error {
		if done[path] {
			return nil
		}
		done[path] = true
		pkg, err := ctx.Import(path, wd, 0)
		if err != nil {
			return err
		}
		for _, names := range [...][]string{
			pkg.GoFiles, pkg.CgoFiles, pkg.IgnoredGoFiles,
			pkg.TestGoFiles, pkg.XTestGoFiles,
		} {
			for _, name := range names {
				path := filepath.Join(pkg.Dir, name)
				if err := addFile(path); err != nil {
					return err
				}
			}
		}
		if recurse {
			for _, path := range pkg.Imports {
				if err := addPkg(path); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, path := range paths {
		if strings.HasSuffix(path, ".go") {
			if err := addFile(path); err != nil {
				return nil, err
			}
			continue
		}
		if err := addPkg(path); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func loadTyped(wd string, ctx *build.Context, fset *token.FileSet, args []string) (*loader.Program, error) {
	gctx := gotool.Context{BuildContext: *ctx}
	paths := gctx.ImportPaths(args)
	conf := loader.Config{Fset: fset, Cwd: wd, Build: ctx}
	if _, err := conf.FromArgs(paths, true); err != nil {
		return nil, err
	}
	return conf.Load()
}
