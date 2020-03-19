// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"golang.org/x/tools/go/packages"
	"mvdan.cc/gogrep/internal/load"
)

var usage = func() {
	fmt.Fprint(os.Stderr, `usage: gogrep query [flags] [packages]

gogrep runs queries on Go source code.

The query can be inline source, a Go file, or a pattern of Go packages. Note that the 'gogrep' build tag is used when loading queries via package patterns.

    gogrep 'return $x' ./...
    gogrep returns_grep.go ./...
    gogrep ./grep/... ./...

The flags include:

    -tests  query test packages too

The remaining arguments are package patterns to query. See 'go help packages'.

To see the documentation for the query language, see https://godoc.org/mvdan.cc/gogrep/nls.
`)
}

func main() { os.Exit(main1()) }

func main1() int {
	if err := mainerr(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func mainerr() error {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
	}
	dir, err := ioutil.TempDir("", "gogrep")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	binPath := filepath.Join(dir, "gogrep-main")
	mainData := mainTmplData{}

	if strings.ContainsAny(args[0], " |(){};") {
		// Add the source files for the entry point and the command line
		// function.
		calls := strings.Split(args[0], "; ")
		if err := tmplFile(dir, "cmdline.go", funcTmpl, calls); err != nil {
			return err
		}
		mainData.Funcs = []string{"CommandLine"}
		if err := tmplFile(dir, "gogrep_main.go", mainTmpl, mainData); err != nil {
			return err
		}
		mainData.Files = append(mainData.Files, "cmdline.go", "gogrep_main.go")

		bundlePath := "./bundled-gogrep"
		if path := sourceDir(); path != "" {
			bundlePath = path
		} else {
			for _, file := range bundledFiles {
				name := filepath.Join(dir, "bundled-gogrep", file.name)
				if err := os.MkdirAll(filepath.Dir(name), 0777); err != nil {
					return err
				}
				f, err := os.Create(name)
				if err != nil {
					return err
				}
				defer f.Close()
				content, err := base64.RawStdEncoding.DecodeString(file.encContent)
				if err != nil {
					return err
				}
				if _, err := f.Write(content); err != nil {
					return err
				}
				if err := f.Close(); err != nil {
					return err
				}
			}
		}
		// Add a go.mod with a replace to the gogrep source code.
		if err := tmplFile(dir, "go.mod", modTmpl, bundlePath); err != nil {
			return err
		}
		mainData.Dir = dir // use our go.mod above
	} else {
		fset := token.NewFileSet()
		cfg := &packages.Config{
			Mode:       packages.NeedName | packages.NeedFiles | packages.NeedSyntax,
			BuildFlags: []string{"-tags=gogrep"},
			Fset:       fset,
		}
		grepPkgs, err := packages.Load(cfg, args[0])
		if err != nil {
			return fmt.Errorf("error loading gogrep funcs: %v", err)
		}
		if err := load.PkgsErr(grepPkgs); err != nil {
			return err
		}
		if len(grepPkgs) == 0 {
			return fmt.Errorf("first argument did not match any packages")
		}
		for _, pkg := range grepPkgs {
			imported := false
			for _, file := range pkg.Syntax {
				fileCopied := false
				for _, decl := range file.Decls {
					fd, ok := decl.(*ast.FuncDecl)
					if !ok {
						continue
					}
					if !grepFunc(fd) {
						continue
					}
					name := fd.Name.Name
					if pkg.PkgPath != "command-line-arguments" {
						// Packages we can import.
						if !imported {
							mainData.Imports = append(mainData.Imports, pkg.PkgPath)
							imported = true
						}
						name = path.Base(pkg.PkgPath) + "." + name
					} else if !fileCopied {
						// An ad-hoc package; build with it directly.
						file.Name.Name = "main"
						name := fset.Position(file.Pos()).Filename
						f, err := os.Create(filepath.Join(dir, filepath.Base(name)))
						if err != nil {
							return err
						}
						if err := printer.Fprint(f, fset, file); err != nil {
							return err
						}
						if err := f.Close(); err != nil {
							return err
						}
						mainData.Files = append(mainData.Files, f.Name())
						fileCopied = true
					}
					mainData.Funcs = append(mainData.Funcs, name)
				}
			}
		}
		if err := tmplFile(dir, "gogrep_main.go", mainTmpl, mainData); err != nil {
			return err
		}
		mainData.Files = append(mainData.Files, filepath.Join(dir, "gogrep_main.go"))
	}

	// Build the binary.
	{
		goArgs := []string{"build", "-tags=gogrep", "-o=" + binPath}
		cmd := exec.Command("go", append(goArgs, mainData.Files...)...)
		cmd.Dir = mainData.Dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	// Run the built program.
	cmd := exec.Command(binPath, args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func sourceDir() string {
	if path := os.Getenv("GOGREP_SOURCE"); path != "" {
		if path == "off" {
			return "" // force using bundled source
		}
		return path // we assume it's absolute
	}
	_, file, _, _ := runtime.Caller(0)
	if !filepath.IsAbs(file) {
		return "" // e.g. used -trimpath
	}
	if _, err := os.Stat(file); err != nil {
		return "" // source is now moved or deleted
	}
	return filepath.Dir(file)
}

func grepFunc(fd *ast.FuncDecl) bool {
	if !token.IsExported(fd.Name.Name) {
		return false
	}
	if fd.Recv != nil || fd.Type.Results != nil || fd.Type.Params == nil {
		return false
	}
	if len(fd.Type.Params.List) != 1 {
		return false
	}
	param := fd.Type.Params.List[0]
	if len(param.Names) > 1 {
		return false
	}
	star, ok := param.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch typ := star.X.(type) {
	case *ast.Ident:
		return typ.Name == "G"
	case *ast.SelectorExpr:
		id, ok := typ.X.(*ast.Ident)
		return ok && id.Name == "nls" && typ.Sel.Name == "G"
	}
	return false
}

func tmplFile(dir, name string, tmpl *template.Template, data interface{}) error {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tmpl.Execute(f, data); err != nil {
		return err
	}
	return f.Close()
}

//go:generate go run gen_bundle.go
//go:generate gofmt -s -w bundle.go

var modTmpl = template.Must(template.New("").Parse(`
module _m

go 1.14

require mvdan.cc/gogrep v0.0.0-00010101000000-000000000000

replace mvdan.cc/gogrep => {{ . }}
`))

var funcTmpl = template.Must(template.New("").Parse(`
package main

import . "mvdan.cc/gogrep/nls"

func CommandLine(g *G) {
{{ range $_, $call := . }}	{{ $call }}(g)
{{ end }}
}
`))

type mainTmplData struct {
	Imports []string // to add as Go imports
	Dir     string   // where to run the build from
	Files   []string // files to build directly; for ad-hoc packages
	Funcs   []string // qualified if needed, e.g. pkg.FooFunc
}

var mainTmpl = template.Must(template.New("").Parse(`
// +build ignore

package main

import (
	"os"

	"mvdan.cc/gogrep/mainpkg"
	"mvdan.cc/gogrep/nls"

{{ range $_, $path := .Imports }}	{{ printf "%q" $path }}
{{ end }}
)

var grepFuncs = []nls.Function{
{{ range $_, $fn := .Funcs }}	{{ $fn }},
{{ end }}
}

func main() { os.Exit(mainpkg.Run(grepFuncs)) }
`))
