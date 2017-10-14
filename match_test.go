// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"go/ast"
	"testing"
)

type wantErr string

func tokErr(msg string) wantErr   { return wantErr("cannot tokenize expr: " + msg) }
func parseErr(msg string) wantErr { return wantErr("cannot parse expr: " + msg) }

func TestMatch(t *testing.T) {
	tests := []struct {
		expr, src string
		anyWant   interface{}
	}{
		// expr tokenize errors
		{"$", "", tokErr("1:2: $ must be followed by ident, got EOF")},
		{`"`, "", tokErr("1:1: string literal not terminated")},
		{"", "", parseErr("empty source code")},
		{"\t", "", parseErr("empty source code")},
		{"$(x", "", tokErr("1:4: expected ) to close $(")},
		{"$(x /expr", "", tokErr("1:5: expected / to terminate regex")},
		{"$(x /foo(bar/)", "", tokErr("1:1: error parsing regexp: missing closing ): `^foo(bar$`")},

		// expr parse errors
		{"foo)", "", parseErr("1:4: expected statement, found ')'")},
		{"{", "", parseErr("1:4: expected '}', found 'EOF'")},
		{"$x)", "", parseErr("1:3: expected statement, found ')'")},
		{"$x(", "", parseErr("1:5: expected operand, found '}'")},
		{"$*x)", "", parseErr("1:4: expected statement, found ')'")},
		{"a\n$x)", "", parseErr("2:3: expected statement, found ')'")},

		// basic lits
		{"123", "123", 1},
		{"false", "true", 0},

		// wildcards
		{"$x", "rune", 1},
		{"foo($x, $x)", "foo(1, 2)", 0},
		{"foo($_, $_)", "foo(1, 2)", 1},
		{"foo($x, $y, $y)", "foo(1, 2, 2)", 1},
		{"$(x)", `"foo"`, 1},

		// recursion
		{"$x", "a + b", 3},
		{"$x + $x", "foo(a + a, b + b)", 2},
		{"$x", "var a int", 4},
		{"go foo()", "a(); go foo(); a()", 1},

		// ident regex matches
		{"$(x /foo/)", "bar", 0},
		{"$(x /foo/)", "foo", 1},
		{"$(x /foo/)", "_foo", 0},
		{"$(x /foo/)", "foo_", 0},
		{"$(x /.*foo.*/)", "_foo_", 1},
		{"$(x /.*/) = $_", "a = b", 1},
		{"$(x /.*/) = $_", "a.field = b", 0},

		// many value expressions
		{"$x, $y", "foo(1, 2)", 1},
		{"$x, $y", "1", 0},
		{"$x", "a, b", 3},
		{"b, c", "a, b, c, d", 0},
		{"b, c", "foo(a, b, c, d)", 0},
		{"print($*_, $x)", "print(a, b, c)", 1},

		// any number of expressions
		{"$*x", "a, b", 1},
		{"print($*x)", "print()", 1},
		{"print($*x)", "print(a, b)", 1},
		{"print($*x, $y, $*z)", "print()", 0},
		{"print($*x, $y, $*z)", "print(a)", 1},
		{"print($*x, $y, $*z)", "print(a, b, c)", 1},
		{"{ $*_; return nil }", "{ return nil }", 1},
		{"{ $*_; return nil }", "{ a(); b(); return nil }", 1},
		{"c($*x); c($*x)", "c(); c()", 1},
		{"c($*x); c()", "c(); c()", 1},
		{"c($*x); c($*x)", "c(x); c(y)", 0},
		{"c($*x); c($*x)", "c(x, y); c(z)", 0},
		{"c($*x); c($*x)", "c(x, y); c(x, y)", 1},
		{"c($*x, y); c($*x, y)", "c(x, y); c(x, y)", 1},
		{"c($*x, $*y); c($*x, $*y)", "c(x, y); c(x, y)", 1},

		// composite lits
		{"[]float64{$x}", "[]float64{3}", 1},
		{"[2]bool{$x, 0}", "[2]bool{3, 1}", 0},
		{"someStruct{fld: $x}", "someStruct{fld: a, fld2: b}", 0},
		{"map[int]int{1: $x}", "map[int]int{1: a}", 1},

		// func lits
		{"func($s string) { print($s) }", "func(a string) { print(a) }", 1},
		{"func($x ...$t) {}", "func(a ...int) {}", 1},

		// type exprs
		{"[8]$x", "[8]int", 1},
		{"struct{field $t}", "struct{field int}", 1},
		{"struct{field $t}", "struct{field int}", 1},
		{"struct{field $t}", "struct{other int}", 0},
		{"struct{field $t}", "struct{f1, f2 int}", 0},
		{"interface{$x() int}", "interface{i() int}", 1},
		{"chan $x", "chan bool", 1},
		{"<-chan $x", "chan bool", 0},
		{"chan $x", "chan<- bool", 0},

		// many types (TODO; revisit)
		// {"chan $x, interface{}", "chan int, interface{}", 1},
		// {"chan $x, interface{}", "chan int", 0},
		// {"$x string, $y int", "func(s string, i int) {}", 1},

		// parens
		{"($x)", "(a + b)", 1},
		{"($x)", "a + b", 0},

		// unary ops
		{"-someConst", "- someConst", 1},
		{"*someVar", "* someVar", 1},

		// binary ops
		{"$x == $y", "a == b", 1},
		{"$x == $y", "123", 0},
		{"$x == $y", "a != b", 0},
		{"$x - $x", "a - b", 0},

		// calls
		{"someFunc($x)", "someFunc(a > b)", 1},

		// selector
		{"$x.Field", "a.Field", 1},
		{"$x.Field", "a.field", 0},
		{"$x.Method()", "a.Method()", 1},
		{"a.b", "a.b.c", 1},
		{"b.c", "a.b.c", 0},
		{"$x.c", "a.b.c", 1},
		{"a.$x", "a.b.c", 1},

		// indexes
		{"$x[len($x)-1]", "a[len(a)-1]", 1},
		{"$x[len($x)-1]", "a[len(b)-1]", 0},

		// slicing
		{"$x[:$y]", "a[:1]", 1},
		{"$x[3:]", "a[3:5:5]", 0},

		// type asserts
		{"$x.(string)", "a.(string)", 1},

		// elipsis
		{"append($x, $y...)", "append(a, bs...)", 1},
		{"foo($x...)", "foo(a)", 0},
		{"foo($x...)", "foo(a, b)", 0},

		// many statements
		{"$x(); $y()", "a(); b()", 1},
		{"$x(); $y()", "a()", 0},
		{"$x", "a; b", 3},
		{"b; c", "b", 0},
		{"b; c", "b; c", 1},
		{"b; c", "b; x; c", 0},
		{"b; c", "a; b; c; d", "b; c"},
		{"b; c", "{b; c; d}", 1},
		{"b; c", "{a; b; c}", 1},
		{"b; c", "{b; b; c; c}", "b; c"},
		{"$x++; $x--", "n; a++; b++; b--", "b++; b--"},
		{"$*_; b; $*_", "{a; b; c; d}", "a; b; c; d"},
		{"{$*_; $x}", "{a; b; c}", 1},
		{"{b; c}", "{a; b; c}", 0},

		// mixing lists
		{"$x, $y", "1; 2", 0},
		{"$x; $y", "1, 2", 0},

		// any number of statements
		{"$*x", "a; b", 1},
		{"$*x; b; $*y", "a; b; c", 1},
		{"$*x; b; $*x", "a; b; c", 0},

		// declarations
		{"const $x = $y", "const a = b", 1},
		{"const $x = $y", "const (a = b)", 1},
		{"const $x = $y", "const (a = b\nc = d)", 0},
		{"var $x int", "var a int", 1},
		{"var $x int", "var a int = 3", 0},
		{
			"func $_($x $y) $y { return $x }",
			"func a(i int) int { return i }", 1,
		},

		// entire files
		{"package $_", "package p; var a = 1", 0},
		{"package $_; func Foo() { $*_ }", "package p; func Foo() {}", 1},

		// blocks
		{"{ $x }", "{ a() }", 1},
		{"{ $x }", "{ a(); b() }", 0},

		// assigns
		{"$x = $y", "a = b", 1},
		{"$x := $y", "a, b := c()", 0},

		// if stmts
		{"if $x != nil { $y }", "if p != nil { p.foo() }", 1},
		{"if $x { $y }", "if a { b() } else { c() }", 0},
		{"if $x != nil { $y }", "if a != nil { return a }", 1},

		// for and range stmts
		{"for $x { $y }", "for b { c() }", 1},
		{"for $x := range $y { $z }", "for i := range l { c() }", 1},
		{"for range $y { $z }", "for _, e := range l { e() }", 0},

		// inc/dec stmts
		{"$x++", "a[b]++", 1},
		{"$x--", "a++", 0},

		// returns
		{"return nil, $x", "{ return nil, err }", 1},
		{"return nil, $x", "{ return nil, 0, err }", 0},

		// go stmts
		{"go $x()", "go func() { a() }()", 1},
		{"go func() { $x }()", "go func() { a() }()", 1},
		{"go func() { $x }()", "go a()", 0},

		// defer stmts
		{"defer $x()", "defer func() { a() }()", 1},
		{"defer func() { $x }()", "defer func() { a() }()", 1},
		{"defer func() { $x }()", "defer a()", 0},

		// empty statement
		{";", ";", 1},

		// labeled statement
		{"foo: a", "foo: a", 1},
		{"foo: a", "foo: b", 0},

		// send statement
		{"x <- 1", "x <- 1", 1},
		{"x <- 1", "y <- 1", 0},
		{"x <- 1", "x <- 2", 0},

		// branch statement
		{"break foo", "break foo", 1},
		{"break foo", "break bar", 0},
		{"break foo", "continue foo", 0},
		{"break", "break", 1},
		{"break foo", "break", 0},

		// case clause
		{"switch x {case 4: x}", "switch x {case 4: x}", 1},
		{"switch x {case 4: x}", "switch y {case 4: x}", 0},
		{"switch x {case 4: x}", "switch x {case 5: x}", 0},

		// switch statement
		{"switch x; y {}", "switch x; y {}", 1},
		{"switch x {}", "switch x; y {}", 0},
		{"switch {}", "switch {}", 1},
		{"switch {}", "switch x {}", 0},
		{"switch {}", "switch {case y:}", 0},

		// type switch statement
		{"switch x := y.(z); x {}", "switch x := y.(z); x {}", 1},
		{"switch x := y.(z); x {}", "switch y := y.(z); x {}", 0},
		{"switch x := y.(z); x {}", "switch y := y.(z); x {}", 0},
		// TODO more switch variations.

		// TODO select statement
		// TODO communication clause

		// aggressive mode
		{"for range $x {}", "for _ = range a {}", 0},
		{"~ for range $x {}", "for _ = range a {}", 1},
		{"~ for _ = range $x {}", "for range a {}", 1},
		{"var a int", "var (a, b int; c bool)", 0},
		{"~ var a int", "var (a, b int; c bool)", 1},
		{"{ x; }", "switch { case true: x; }", 0},
		{"~ { x; }", "switch { case true: x; }", 1},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			grepTest(t, tc.expr, tc.src, tc.anyWant)
		})
	}
}

func grepStrs(expr, src string) ([]ast.Node, error) {
	m := matcher{}
	exprNode, err := m.compileExpr(expr)
	if err != nil {
		return nil, err
	}
	srcNode, err := parse(src)
	if err != nil {
		return nil, err
	}
	return m.matches(exprNode, srcNode), nil
}

func grepTest(t *testing.T, expr, src string, anyWant interface{}) {
	terr := func(format string, a ...interface{}) {
		t.Errorf("%s | %s: %s", expr, src, fmt.Sprintf(format, a...))
	}
	matches, err := grepStrs(expr, src)
	switch want := anyWant.(type) {
	case wantErr:
		if err == nil {
			terr("wanted error %q, got none", want)
		} else if got := err.Error(); got != string(want) {
			terr("wanted error %q, got %q", want, got)
		}
	case int:
		if err != nil {
			terr("unexpected error: %v", err)
			return
		}
		if len(matches) != want {
			terr("wanted %d matches, got %d", want, len(matches))
		}
	case string:
		if err != nil {
			terr("unexpected error: %v", err)
			return
		}
		if len(matches) != 1 {
			terr("wanted 1 match, got %d", len(matches))
			return
		}
		got := singleLinePrint(matches[0])
		if got != want {
			terr("wanted %q match, got %q", want, got)
		}
	default:
		panic(fmt.Sprintf("unexpected anyWant type: %T", anyWant))
	}
}
