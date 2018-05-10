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
			[]string{"-x", "var _ = $x", "testdata/two/file1.go", "testdata/two/file2.go"},
			`
				testdata/two/file1.go:3:1: var _ = "file1"
				testdata/two/file2.go:3:1: var _ = "file2"
			`,
		},
		{
			[]string{"-x", "var _ = $x", "-x", "$x", "-a", "type(string)", "testdata/two/file1.go", "testdata/two/file2.go"},
			fmt.Errorf("package p2; expected p1"),
		},
		{
			[]string{"-x", "var _ = $x", "noexist.go"},
			fmt.Errorf("no such file or directory"),
		},
		{
			[]string{"-x", "var _ = $x", "-x", "$x", "-a", "type(string)", "noexist.go"},
			fmt.Errorf("no such file or directory"),
		},
		{
			[]string{"-x", "var _ = $x", "./testdata/two"},
			fmt.Errorf("packages p1 (file1.go) and p2 (file2.go)"),
		},
		{
			[]string{"-x", "var _ = $x", "-x", "$x", "-a", "type(string)", "./testdata/two"},
			fmt.Errorf("packages p1 (file1.go) and p2 (file2.go)"),
		},
		{
			[]string{"-x", "var _ = $x", "p1"},
			`testdata/src/p1/file1.go:3:1: var _ = "file1"`,
		},
		{
			[]string{"-x", "var _ = $x", "-x", "$x", "-a", "type(string)", "-p", "2", "p1"},
			`testdata/src/p1/file1.go:3:1: var _ = "file1"`,
		},
		{
			[]string{"-x", "var _ = $x", "-x", "$x", "-a", "type(int)", "p1"},
			``, // different type
		},
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
			[]string{"-x", "var _ = $x", "-x", "$x", "-a", "type(string)", "-p", "2", "p1/..."},
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
		{
			[]string{"-x", "var _ = $x", "-x", "$x", "-a", "type(string)", "-p", "2", "-r", "p1"},
			`
				testdata/src/p1/file1.go:3:1: var _ = "file1"
				testdata/src/p1/p2/file1.go:3:1: var _ = "file1"
				testdata/src/p1/p2/file2.go:3:1: var _ = "file2"
				testdata/src/p1/testp/file1.go:3:1: var _ = "file1"
			`,
		},
		{
			[]string{"-x", "var _ = $x", "testdata/longstr.go"},
			`
				testdata/longstr.go:3:1: var _ = ` + "`single line`" + `
				testdata/longstr.go:4:1: var _ = "some\nmultiline\nstring"
			`,
		},
		{
			[]string{"-x", "if $_ { $*_ }", "testdata/longstmt.go"},
			`testdata/longstmt.go:4:2: if true { foo(); bar(); }`,
		},
		{
			[]string{"-x", "1, 2, 3, 4, 5", "testdata/exprlist.go"},
			`testdata/exprlist.go:3:13: 1, 2, 3, 4, 5`,
		},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			var buf bytes.Buffer
			m.out = &buf
			err := m.fromArgs(tc.args)
			switch x := tc.want.(type) {
			case error:
				if err == nil {
					t.Fatalf("wanted error %q, got none", x)
				}
				want, got := x.Error(), err.Error()
				if !strings.Contains(got, want) {
					t.Fatalf("wanted error %q, got %q", want, got)
				}
			case string:
				if err != nil {
					t.Fatalf("didn't want error, but got %q", err)
				}
				want := strings.TrimSpace(strings.Replace(x, "\t", "", -1))
				got := strings.TrimSpace(buf.String())
				if want != got {
					t.Fatalf("wanted:\n%s\ngot:\n%s", want, got)
				}
			default:
				t.Fatalf("unknown want type %T", x)
			}
		})
	}
}
