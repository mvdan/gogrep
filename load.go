// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
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

type loadPkg struct {
	path  string
	nodes []ast.Node
	info  types.Info
}

func (l nodeLoader) untyped(args []string, recurse bool) ([]loadPkg, error) {
	gctx := gotool.Context{BuildContext: *l.ctx}
	paths := gctx.ImportPaths(args)
	var pkgs []loadPkg
	var cur loadPkg
	addFile := func(path string) error {
		f, err := parser.ParseFile(l.fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		cur.nodes = append(cur.nodes, f)
		return nil
	}
	done := map[string]bool{}
	var addPkg func(path string, direct bool) error // to recurse into self
	addPkg = func(path string, direct bool) error {
		if done[path] {
			return nil
		}
		done[path] = true
		if len(cur.nodes) > 0 {
			pkgs = append(pkgs, cur)
			cur = loadPkg{path: path}
		}
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
	if len(cur.nodes) > 0 {
		pkgs = append(pkgs, cur)
	}
	return pkgs, nil
}

func (l nodeLoader) typed(args []string, recurse bool) ([]loadPkg, error) {
	gctx := gotool.Context{BuildContext: *l.ctx}
	paths := gctx.ImportPaths(args)
	conf := loader.Config{Fset: l.fset, Cwd: l.wd, Build: l.ctx}
	if _, err := conf.FromArgs(paths, true); err != nil {
		return nil, err
	}
	var terr error
	conf.TypeChecker.Error = func(err error) {
		if terr == nil {
			terr = err
		}
	}
	prog, err := conf.Load()
	if err != nil {
		if terr != nil {
			return nil, terr
		}
		return nil, err
	}
	var pkgs []loadPkg
	done := map[string]bool{}
	var addPkg func(tpkg *types.Package) // to recurse into self
	addPkg = func(tpkg *types.Package) {
		path := tpkg.Path()
		if done[path] {
			return
		}
		done[path] = true
		pkg := prog.Package(path)
		lpkg := loadPkg{path: path, info: pkg.Info}
		for _, file := range pkg.Files {
			lpkg.nodes = append(lpkg.nodes, file)
		}
		pkgs = append(pkgs, lpkg)
		if !recurse {
			return
		}
		// TODO: differentiate direct imports like in untyped?
		for _, ipkg := range tpkg.Imports() {
			addPkg(ipkg)
		}
	}
	for _, pkg := range prog.InitialPackages() {
		addPkg(pkg.Pkg)
	}
	return pkgs, nil
}
