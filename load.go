// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"go/token"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type nodeLoader struct {
	wd   string
	fset *token.FileSet
}

func (l nodeLoader) typed(args []string, recurse bool) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode:  packages.LoadSyntax,
		Dir:   l.wd,
		Fset:  l.fset,
		Tests: true,
	}
	if recurse {
		// we'll need the syntax trees for the dependencies too
		cfg.Mode = packages.LoadAllSyntax
	}
	pkgs, err := packages.Load(cfg, args...)
	if err != nil {
		return nil, err
	}

	// Make a sorted list of the packages, including transitive dependencies
	// if recurse is true.
	byPath := make(map[string]*packages.Package)
	var addDeps func(*packages.Package)
	addDeps = func(pkg *packages.Package) {
		if strings.HasSuffix(pkg.PkgPath, ".test") {
			// don't add recursive test deps
			return
		}
		for _, imp := range pkg.Imports {
			if _, ok := byPath[imp.PkgPath]; ok {
				continue // seen; avoid recursive call
			}
			byPath[imp.PkgPath] = imp
			addDeps(imp)
		}
	}
	for _, pkg := range pkgs {
		byPath[pkg.PkgPath] = pkg
		if recurse {
			// add all dependencies once
			addDeps(pkg)
		}
	}
	pkgs = pkgs[:0]
	for _, pkg := range byPath {
		pkgs = append(pkgs, pkg)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].PkgPath < pkgs[j].PkgPath
	})
	return pkgs, nil
}
