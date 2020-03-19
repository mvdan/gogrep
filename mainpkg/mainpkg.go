// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package mainpkg

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"io/ioutil"
	"os"
	"strings"

	"mvdan.cc/gogrep/gsyntax"
	"mvdan.cc/gogrep/internal/load"
	"mvdan.cc/gogrep/nls"
)

var tests = flag.Bool("tests", false, "search test packages too")

func Run(funcs []nls.Function) int {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
	}

	fset := token.NewFileSet()
	g := &nls.G{
		Fset:  fset,
		Tests: *tests,
	}
	results, err := run(g, funcs, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, n := range results {
		fpos := fset.Position(n.Pos())
		if strings.HasPrefix(fpos.Filename, wd) {
			fpos.Filename = fpos.Filename[len(wd)+1:]
		}
		fmt.Printf("%v: %s\n", fpos, gsyntax.PrintCompact(n))
	}
	return 0
}

func run(g *nls.G, funcs []nls.Function, args []string) ([]ast.Node, error) {
	if len(args) == 0 {
		input, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("error reading input: %v", err)
		}
		node, err := load.Input(g, string(input))
		if err != nil {
			return nil, fmt.Errorf("error loading input: %v", err)
		}
		var results []ast.Node
		for _, fn := range funcs {
			result, err := g.Run(fn, node)
			if err != nil {
				return nil, err
			}
			results = append(results, result...)
		}
		return results, nil
	}
	pkgs, err := load.Args(g, args...)
	if err != nil {
		return nil, fmt.Errorf("error loading packages: %v", err)
	}
	var results []ast.Node
	for _, pkg := range pkgs {
		g.Info = pkg.TypesInfo
		nodes := make([]ast.Node, len(pkg.Syntax))
		for i, f := range pkg.Syntax {
			nodes[i] = f
		}
		for _, fn := range funcs {
			result, err := g.Run(fn, nodes...)
			if err != nil {
				return nil, err
			}
			results = append(results, result...)
		}
	}
	return results, nil
}
