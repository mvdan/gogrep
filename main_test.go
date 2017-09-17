// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import "testing"

func TestGrep(t *testing.T) {
	tests := []struct {
		expr, src string
		wantMatch bool
	}{
		// basic lits
		{"123", "123", true},
		{"false", "true", false},

		// composite lits
		{"[]float64{$x}", "[]float64{3}", true},
		{"[2]bool{$x, 0}", "[2]bool{3, 1}", false},
		{"someStruct{fld: $x}", "someStruct{fld: a, fld2: b}", false},
		{"map[int]int{1: $x}", "map[int]int{1: a}", true},

		// func lits
		{"func($s string) { print($s) }", "func(a string) { print(a) }", true},

		// more types
		{"struct{field $t}", "struct{field int}", true},
		{"$x.(string)", "a.(string)", true},

		// parens
		{"($x)", "(a + b)", true},
		{"($x)", "a + b", false},

		// unary ops
		{"-someConst", "- someConst", true},
		{"*someVar", "* someVar", true},

		// binary ops
		{"$x == $y", "a == b", true},
		{"$x == $y", "123", false},
		{"$x == $y", "a != b", false},
		{"$x - $x", "a - b", false},

		// calls
		{"someFunc($x)", "someFunc(a > b)", true},

		// selector
		{"$x.Field", "a.Field", true},
		{"$x.Field", "a.field", false},
		{"$x.Method()", "a.Method()", true},

		// index
		{"$x[len($x)-1]", "a[len(a)-1]", true},
		{"$x[len($x)-1]", "a[len(b)-1]", false},

		// slicing
		{"$x[:$y]", "a[:1]", true},
		{"$x[3:]", "a[3:5:5]", false},

		// elipsis
		{"append($x, $y...)", "append(a, bs...)", true},
		{"foo($x...)", "foo(a, b)", false},
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
