// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import "testing"

func TestGrep(t *testing.T) {
	tests := []struct {
		expr, src string
		wantMatch bool
	}{
		{"123", "123", true},
		{"false", "true", false},

		{"[]float64{$x}", "[]float64{3}", true},
		{"someStruct{fld: $x}", "someStruct{fld: a, fld2: b}", false},

		{"-someConst", "- someConst", true},
		{"*someVar", "* someVar", true},

		{"$x == $y", "a == b", true},
		{"$x == $y", "123", false},
		{"$x == $y", "a != b", false},
		{"$x - $x", "a - b", false},

		{"someFunc($x)", "someFunc(a > b)", true},
	}
	for _, tc := range tests {
		match, err := grep(tc.expr, tc.src)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		if match && !tc.wantMatch {
			t.Errorf("%s | %s: got unexpected match", tc.expr, tc.src)
		} else if !match && tc.wantMatch {
			t.Errorf("%s | %s: wanted match, got none", tc.expr, tc.src)
		}
	}
}
