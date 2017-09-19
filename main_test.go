// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"go/ast"
	"testing"
)

type wantErr string

func parseErr(msg string) wantErr {
	return wantErr("cannot parse expr: " + msg)
}

func TestGrep(t *testing.T) {
	tests := []struct {
		expr, src string
		anyWant   interface{}
	}{
		// expr tokenize errors
		{"$", "", parseErr("1:2: $ must be followed by ident, got EOF")},
		{`"`, "", parseErr("1:1: string literal not terminated")},

		// expr parse errors
		{"foo)", "", parseErr("1:5: expected statement, found ')' (and 1 more errors)")},
		{"{", "", parseErr("2:2: expected '}', found 'EOF'")},

		// basic lits
		{"123", "123", 1},
		{"false", "true", 0},

		// wildcards
		{"$x", "rune", 1},
		{"foo($x, $x)", "foo(1, 2)", 0},
		{"foo($_, $_)", "foo(1, 2)", 1},
		{"foo($x, $y, $y)", "foo(1, 2, 2)", 1},

		// recursion
		{"$x", "a + b", 3},
		{"$x + $x", "foo(a + a, b + b)", 2},
		{"$x", "var a int", 5},
		{"go foo()", "a(); go foo(); a()", 1},

		// many value expressions
		{"$x, $y", "foo(1, 2)", 1},
		{"$x, $y", "1", 0},

		// any number of expressions (TODO: remember the lists)
		{"print($*x)", "print()", 1},
		{"print($*x)", "print(a, b)", 1},
		{"print($*x, $y, $*z)", "print()", 0},
		{"print($*x, $y, $*z)", "print(a)", 1},
		{"print($*x, $y, $*z)", "print(a, b, c)", 1},
		{"{ $*_; return nil }", "{ return nil }", 1},
		{"{ $*_; return nil }", "{ a(); b(); return nil }", 1},

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
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			grepTest(t, tc.expr, tc.src, tc.anyWant)
		})
	}
}

func grepStrs(expr, src string) ([]ast.Node, error) {
	exprNode, _, err := compileExpr(expr)
	if err != nil {
		return nil, err
	}
	srcNode, err := parse(src)
	if err != nil {
		return nil, err
	}
	return search(exprNode, srcNode), nil
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
	default:
		panic(fmt.Sprintf("unexpected anyWant type: %T", anyWant))
	}
}
