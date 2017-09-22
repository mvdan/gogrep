// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	const expr = "const _ = $x"
	tests := []struct {
		args []string
		want interface{}
	}{
		{
			[]string{"testdata/file1.go", "testdata/file2.go"},
			`
				testdata/file1.go:3:1: const _ = "file1"
				testdata/file2.go:3:1: const _ = "file2"
			`,
		},
		{
			[]string{"noexist.go"},
			fmt.Errorf("open noexist.go: no such file or directory"),
		},
	}
	for _, tc := range tests {
		var buf bytes.Buffer
		err := grepArgs(&buf, expr, tc.args)
		switch x := tc.want.(type) {
		case error:
			if err == nil {
				t.Errorf("wanted error %v, got none", x)
			} else if want, got := x.Error(), err.Error(); want != got {
				t.Errorf("wanted error %q, got %q", want, got)
			}
		case string:
			if err != nil {
				t.Errorf("didn't want error, but got %v", err)
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
