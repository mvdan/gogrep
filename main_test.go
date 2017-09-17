// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"testing"
)

type wantErr string

func tokErr(msg string) wantErr {
	return wantErr("cannot tokenize expr: "+msg)
}

func parseErr(msg string) wantErr {
	return wantErr("cannot parse expr: "+msg)
}

func TestGrep(t *testing.T) {
	tests := []struct {
		expr, src string
		anyWant   interface{}
	}{
		// expr tokenize errors
		{"$", "", tokErr("1:2: $ must be followed by ident, got EOF")},
		{`"`, "", tokErr("1:1: string literal not terminated")},

		// expr parse errors
		{"{", "", parseErr("6:2: expected '}', found 'EOF'")},

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
			grepTest(t, tc.expr, tc.src, tc.anyWant)
		})
	}
}

func grepTest(t *testing.T, expr, src string, anyWant interface{}) {
	terr := func(format string, a ...interface{}) {
		t.Errorf("%s | %s: %s", expr, src, fmt.Sprintf(format, a...))
	}
	match, err := grep(expr, src)
	switch want := anyWant.(type) {
	case wantErr:
		if err == nil {
			terr("wanted error %q, got none", want)
		} else if got := err.Error(); got != string(want) {
			terr("wanted error %q, got %q", got, want)
		}
	case bool:
		if err != nil {
			terr("unexpected error: %v", err)
			return
		}
		if match && !want {
			terr("got unexpected match")
		} else if !match && want {
			terr("wanted match, got none")
		}
	}
}
