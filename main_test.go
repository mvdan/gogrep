// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"testing"
)

type wantErr string

func tokErr(msg string) wantErr {
	return wantErr("cannot tokenize expr: " + msg)
}

func parseErr(msg string) wantErr {
	return wantErr("cannot parse expr: " + msg)
}

type matches uint

var noMatch = matches(0)

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
		{"123", "123", matches(1)},
		{"false", "true", noMatch},

		// wildcards
		{"$x", "rune", matches(1)},
		{"foo($x, $x)", "foo(1, 2)", noMatch},
		{"foo($_, $_)", "foo(1, 2)", matches(1)},
		{"foo($x, $y, $y)", "foo(1, 2, 2)", matches(1)},

		// many expressions
		{"$x, $y", "1, 2", matches(1)},
		{"$x, $y", "1", noMatch},

		// composite lits
		{"[]float64{$x}", "[]float64{3}", matches(1)},
		{"[2]bool{$x, 0}", "[2]bool{3, 1}", noMatch},
		{"someStruct{fld: $x}", "someStruct{fld: a, fld2: b}", noMatch},
		{"map[int]int{1: $x}", "map[int]int{1: a}", matches(1)},

		// func lits
		{"func($s string) { print($s) }", "func(a string) { print(a) }", matches(1)},
		{"func($x ...$t) {}", "func(a ...int) {}", matches(1)},

		// more types
		{"struct{field $t}", "struct{field int}", matches(1)},
		{"struct{field $t}", "struct{other int}", noMatch},
		{"struct{field $t}", "struct{f1, f2 int}", noMatch},
		{"interface{$x() int}", "interface{i() int}", matches(1)},
		{"chan $x", "chan bool", matches(1)},
		{"<-chan $x", "chan bool", noMatch},
		{"chan $x", "chan<- bool", noMatch},

		// many types
		{"chan $x, interface{}", "chan int, interface{}", matches(1)},
		{"chan $x, interface{}", "chan int", noMatch},

		// parens
		{"($x)", "(a + b)", matches(1)},
		{"($x)", "a + b", noMatch},

		// unary ops
		{"-someConst", "- someConst", matches(1)},
		{"*someVar", "* someVar", matches(1)},

		// binary ops
		{"$x == $y", "a == b", matches(1)},
		{"$x == $y", "123", noMatch},
		{"$x == $y", "a != b", noMatch},
		{"$x - $x", "a - b", noMatch},

		// calls
		{"someFunc($x)", "someFunc(a > b)", matches(1)},

		// selector
		{"$x.Field", "a.Field", matches(1)},
		{"$x.Field", "a.field", noMatch},
		{"$x.Method()", "a.Method()", matches(1)},

		// index
		{"$x[len($x)-1]", "a[len(a)-1]", matches(1)},
		{"$x[len($x)-1]", "a[len(b)-1]", noMatch},

		// slicing
		{"$x[:$y]", "a[:1]", matches(1)},
		{"$x[3:]", "a[3:5:5]", noMatch},

		// type asserts
		{"$x.(string)", "a.(string)", matches(1)},

		// elipsis
		{"append($x, $y...)", "append(a, bs...)", matches(1)},
		{"foo($x...)", "foo(a)", noMatch},
		{"foo($x...)", "foo(a, b)", noMatch},

		// many statements
		{"$x(); $y()", "a(); b()", matches(1)},
		{"$x(); $y()", "a()", noMatch},

		// block
		{"{ $x }", "{ a() }", matches(1)},
		{"{ $x }", "{ a(); b() }", noMatch},
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
	case matches:
		if err != nil {
			terr("unexpected error: %v", err)
			return
		}
		if match && want == 0 {
			terr("got unexpected match")
		} else if !match && want > 0 {
			terr("wanted match, got none")
		}
	default:
		panic(fmt.Sprintf("unexpected anyWant type: %T", anyWant))
	}
}
