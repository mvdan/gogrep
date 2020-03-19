// +build ignore

package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var globs = []string{
	"go.*",
	// TODO: use 'go list' to include deps automatically
	"mainpkg/*.go",
	"nls/*.go",
	"gsyntax/*.go",
	"internal/load/*.go",
}

type bundledFile struct{ Name, Content string }

var tmpl = template.Must(template.New("").Parse(`
package main

var bundledFiles = [...]struct{
	name, encContent string
}{
{{ range $_, $f := . }} { {{ printf "%q" $f.Name }}, {{ printf "%q" $f.Content }} },
{{ end }}
}
`))

func run() error {
	var files []bundledFile
	for _, glob := range globs {
		matches, err := filepath.Glob(glob)
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no matches for %q", glob)
		}
		for _, match := range matches {
			if strings.HasSuffix(match, "_test.go") {
				continue
			}
			content, err := ioutil.ReadFile(match)
			if err != nil {
				return err
			}
			files = append(files, bundledFile{
				Name:    match,
				Content: base64.RawStdEncoding.EncodeToString(content),
			})
		}
	}
	f, err := os.Create("bundle.go")
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, files); err != nil {
		return err
	}
	return f.Close()
}
