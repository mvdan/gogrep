// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package load

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"

	"mvdan.cc/gogrep/gsyntax"
	"mvdan.cc/gogrep/nls"
)

func Input(g *nls.G, src string) (ast.Node, error) {
	node, file, err := gsyntax.ParseAny(g.Fset, src)
	if err != nil {
		return nil, err
	}

	// Type-checking is attempted on a best-effort basis.
	g.Info = &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	pkg := types.NewPackage("", "")
	config := &types.Config{
		Importer: importer.Default(),
		Error:    func(error) {}, // don't stop at the first error
	}
	check := types.NewChecker(config, g.Fset, pkg, g.Info)
	_ = check.Files([]*ast.File{file})
	g.Scope = pkg.Scope()
	return node, nil
}

func PkgsErr(pkgs []*packages.Package) error {
	jointErr := ""
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, err := range pkg.Errors {
			if jointErr != "" {
				jointErr += "\t"
			}
			jointErr += err.Error() + "\n"
		}
	})
	if jointErr != "" {
		return fmt.Errorf("%s", jointErr)
	}
	return nil
}

func Args(g *nls.G, args ...string) ([]*packages.Package, error) {
	mode := packages.NeedName | packages.NeedImports | packages.NeedSyntax |
		packages.NeedTypes | packages.NeedTypesInfo
	cfg := &packages.Config{
		Mode:  mode,
		Fset:  g.Fset,
		Tests: g.Tests,
	}
	pkgs, err := packages.Load(cfg, args...)
	if err != nil {
		return nil, err
	}
	if err := PkgsErr(pkgs); err != nil {
		return nil, err
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].PkgPath < pkgs[j].PkgPath
	})
	return pkgs, nil
}
