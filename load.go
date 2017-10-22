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

type nodeLoader struct {
	wd   string
	ctx  *build.Context
	fset *token.FileSet
}

func (l nodeLoader) load(args []string, recurse, typed bool) ([]ast.Node, error) {
	if typed {
		return l.typed(args, recurse)
	}
	return l.untyped(args, recurse)
}

func (l nodeLoader) untyped(args []string, recurse bool) ([]ast.Node, error) {
	gctx := gotool.Context{BuildContext: *l.ctx}
	paths := gctx.ImportPaths(args)
	var nodes []ast.Node
	addFile := func(path string) error {
		f, err := parser.ParseFile(l.fset, path, nil, 0)
		if err != nil {
			return err
		}
		nodes = append(nodes, f)
		return nil
	}
	done := map[string]bool{}
	var addPkg func(path string, direct bool) error // to recurse into self
	addPkg = func(path string, direct bool) error {
		if done[path] {
			return nil
		}
		done[path] = true
		pkg, err := l.ctx.Import(path, l.wd, 0)
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
		if !recurse {
			return nil
		}
		imports := pkg.Imports
		if direct {
			imports = append(imports, pkg.TestImports...)
			imports = append(imports, pkg.XTestImports...)
		}
		for _, path := range imports {
			if err := addPkg(path, false); err != nil {
				return err
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
		if err := addPkg(path, true); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func (l nodeLoader) typed(args []string, recurse bool) ([]ast.Node, error) {
	gctx := gotool.Context{BuildContext: *l.ctx}
	paths := gctx.ImportPaths(args)
	conf := loader.Config{Fset: l.fset, Cwd: l.wd, Build: l.ctx}
	if _, err := conf.FromArgs(paths, true); err != nil {
		return nil, err
	}
	prog, err := conf.Load()
	if err != nil {
		return nil, err
	}
	var nodes []ast.Node
	for _, pkg := range prog.InitialPackages() {
		for _, file := range pkg.Files {
			nodes = append(nodes, file)
		}
	}
	return nodes, nil
}
