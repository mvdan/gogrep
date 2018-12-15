// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

func (m *matcher) load(wd string, args ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode:  packages.LoadSyntax,
		Dir:   wd,
		Fset:  m.fset,
		Tests: m.tests,
	}
	if m.recursive {
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
		if m.recursive {
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
