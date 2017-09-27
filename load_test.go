// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"bytes"
	"fmt"
	"go/build"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	const expr = "const _ = $x"
	ctx := build.Default
	ctx.GOPATH = "testdata"
	tests := []struct {
		args    []string
		recurse bool
		want    interface{}
	}{
		{
			[]string{"testdata/file1.go", "testdata/file2.go"}, false,
			`
				testdata/file1.go:3:1: const _ = "file1"
				testdata/file2.go:3:1: const _ = "file2"
			`,
		},
		{
			[]string{"noexist.go"}, false,
			fmt.Errorf("no such file or directory"),
		},
		{
			[]string{"./testdata"}, false,
			fmt.Errorf("packages p1 (file1.go) and p2 (file2.go)"),
		},
		{
			[]string{"p1"}, false,
			`
				testdata/src/p1/file1.go:3:1: const _ = "file1"
			`,
		},
		{
			[]string{"p1/..."}, false,
			`
				testdata/src/p1/file1.go:3:1: const _ = "file1"
				testdata/src/p1/p2/file1.go:3:1: const _ = "file1"
				testdata/src/p1/p2/file2.go:3:1: const _ = "file2"
				testdata/src/p1/p3/testp/file1.go:3:1: const _ = "file1"
				testdata/src/p1/testp/file1.go:3:1: const _ = "file1"
			`,
		},
		{
			[]string{"p1"}, true,
			`
				testdata/src/p1/file1.go:3:1: const _ = "file1"
				testdata/src/p1/p2/file1.go:3:1: const _ = "file1"
				testdata/src/p1/p2/file2.go:3:1: const _ = "file2"
				testdata/src/p1/testp/file1.go:3:1: const _ = "file1"
			`,
		},
	}
	for _, tc := range tests {
		var buf bytes.Buffer
		err := grepArgs(&buf, &ctx, expr, tc.args, tc.recurse)
		switch x := tc.want.(type) {
		case error:
			if err == nil {
				t.Errorf("wanted error %q, got none", x)
				continue
			}
			want, got := x.Error(), err.Error()
			if !strings.Contains(got, want) {
				t.Errorf("wanted error %q, got %q", want, got)
			}
		case string:
			if err != nil {
				t.Errorf("didn't want error, but got %q", err)
				break
			}
			want := strings.TrimSpace(strings.Replace(x, "\t", "", -1))
			got := strings.TrimSpace(buf.String())
			if want != got {
				t.Errorf("wanted:\n%s\ngot:\n%s", want, got)
			}
		default:
			t.Errorf("unknown want type %T", x)
		}
	}
}
