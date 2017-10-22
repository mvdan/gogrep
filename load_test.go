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
	ctx := build.Default
	ctx.GOPATH = "testdata"
	m := matcher{ctx: &ctx}
	tests := []struct {
		args []string
		want interface{}
	}{
		{
			[]string{"-x", "var _ = $x", "testdata/file1.go", "testdata/file2.go"},
			`
				testdata/file1.go:3:1: var _ = "file1"
				testdata/file2.go:3:1: var _ = "file2"
			`,
		},
		{
			[]string{"-x", "var _ = $(x type(string))", "testdata/file1.go", "testdata/file2.go"},
			fmt.Errorf("package p2; expected p1"),
		},
		{
			[]string{"-x", "var _ = $x", "noexist.go"},
			fmt.Errorf("no such file or directory"),
		},
		{
			[]string{"-x", "var _ = $(x type(string))", "noexist.go"},
			fmt.Errorf("no such file or directory"),
		},
		{
			[]string{"-x", "var _ = $x", "./testdata"},
			fmt.Errorf("packages p1 (file1.go) and p2 (file2.go)"),
		},
		{
			[]string{"-x", "var _ = $(x type(string))", "./testdata"},
			fmt.Errorf("packages p1 (file1.go) and p2 (file2.go)"),
		},
		{
			[]string{"-x", "var _ = $x", "p1"},
			`
				testdata/src/p1/file1.go:3:1: var _ = "file1"
			`,
		},
		{
			[]string{"-x", "var _ = $(x type(string))", "p1"},
			`
				testdata/src/p1/file1.go:3:1: var _ = "file1"
			`,
		},
		// TODO: add typed variants for these
		{
			[]string{"-x", "var _ = $x", "p1/..."},
			`
				testdata/src/p1/file1.go:3:1: var _ = "file1"
				testdata/src/p1/p2/file1.go:3:1: var _ = "file1"
				testdata/src/p1/p2/file2.go:3:1: var _ = "file2"
				testdata/src/p1/p3/testp/file1.go:3:1: var _ = "file1"
				testdata/src/p1/testp/file1.go:3:1: var _ = "file1"
			`,
		},
		{
			[]string{"-x", "var _ = $x", "-r", "p1"},
			`
				testdata/src/p1/file1.go:3:1: var _ = "file1"
				testdata/src/p1/p2/file1.go:3:1: var _ = "file1"
				testdata/src/p1/p2/file2.go:3:1: var _ = "file2"
				testdata/src/p1/testp/file1.go:3:1: var _ = "file1"
			`,
		},
	}
	for _, tc := range tests {
		var buf bytes.Buffer
		m.out = &buf
		err := m.fromArgs(tc.args)
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
