// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"testing"
)

func TestGrep(t *testing.T) {
	tests := []struct {
		expr, src string
		wantMatch bool
	}{
		// basic lits
		{"123", "123", true},
		{"false", "true", false},

		// wildcards
		{"$x", "rune", true},
		{"foo($x, $x)", "foo(1, 2)", false},
		{"foo($_, $_)", "foo(1, 2)", true},
		{"foo($x, $y, $y)", "foo(1, 2, 2)", true},

		// many expressions
		{"$x, $y", "1, 2", true},
		{"$x, $y", "1", false},

		// composite lits
		{"[]float64{$x}", "[]float64{3}", true},
		{"[2]bool{$x, 0}", "[2]bool{3, 1}", false},
		{"someStruct{fld: $x}", "someStruct{fld: a, fld2: b}", false},
		{"map[int]int{1: $x}", "map[int]int{1: a}", true},

		// func lits
		{"func($s string) { print($s) }", "func(a string) { print(a) }", true},
		{"func($x ...$t) {}", "func(a ...int) {}", true},

		// more types
		{"struct{field $t}", "struct{field int}", true},
		{"interface{$x() int}", "interface{i() int}", true},
		{"chan $x", "chan bool", true},
		{"<-chan $x", "chan bool", false},
		{"chan $x", "chan<- bool", false},

		// many types
		{"chan $x, interface{}", "chan int, interface{}", true},
		{"chan $x, interface{}", "chan int", false},

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

		// type asserts
		{"$x.(string)", "a.(string)", true},

		// elipsis
		{"append($x, $y...)", "append(a, bs...)", true},
		{"foo($x...)", "foo(a)", false},
		{"foo($x...)", "foo(a, b)", false},

		// many statements
		{"$x(); $y()", "a(); b(); c()", true},
		{"$x(); $y()", "a()", false},

		// block
		{"{ $x }", "{ a() }", true},
		{"{ $x }", "{ a(); b() }", false},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			grepTest(t, tc.expr, tc.src, tc.wantMatch)
		})
	}
}

func grepTest(t *testing.T, expr, src string, wantMatch bool) {
	match, err := grep(expr, src)
	if err != nil {
		t.Errorf("%s | %s: unexpected error: %v", expr, src, err)
		return
	}
	if match && !wantMatch {
		t.Errorf("%s | %s: got unexpected match", expr, src)
	} else if !match && wantMatch {
		t.Errorf("%s | %s: wanted match, got none", expr, src)
	}
}
