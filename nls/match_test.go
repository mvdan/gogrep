// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package nls_test

import (
	"bytes"
	"fmt"
	"go/token"
	"strings"
	"testing"

	"mvdan.cc/gogrep/gsyntax"
	"mvdan.cc/gogrep/internal/load"
	. "mvdan.cc/gogrep/nls"
)

type wantErr string

func Pipe(funcs ...Function) Function {
	return func(g *G) {
		for _, fn := range funcs {
			fn(g)
		}
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		fn   Function
		want interface{}
	}{

		// expr tokenize errors
		{All(`$`), wantErr(`1:2: $ must be followed by ident, got EOF`)},
		{All(`"`), wantErr(`1:1: string literal not terminated`)},
		{All(""), wantErr(`empty source code`)},
		{All("\t"), wantErr(`empty source code`)},
		{
			Pipe(All(`$x`), Type(`foo`)),
			wantErr(`unknown type: "foo"`),
		},
		{
			Pipe(All(`$x`), Type(`{`)),
			wantErr(`1:1: expected ';', found '{'`),
		},
		{
			Pipe(All(`$x`), Type(`notType + expr`)),
			wantErr(`1:9: expected ';', found '+'`),
		},

		// expr parse errors
		{All(`foo)`), wantErr(`1:4: expected statement, found ')'`)},
		{All(`{`), wantErr(`1:4: expected '}', found 'EOF'`)},
		{All(`$x)`), wantErr(`1:3: expected statement, found ')'`)},
		{All(`$x(`), wantErr(`1:5: expected operand, found '}'`)},
		{All(`$*x)`), wantErr(`1:4: expected statement, found ')'`)},
		{All("a\n$x)"), wantErr(`2:3: expected statement, found ')'`)},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%03d", i), func(t *testing.T) {
			grepTest(t, tc.fn, "nosrc", tc.want)
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		fn    Function
		input string
		want  interface{}
	}{
		// basic lits
		{All(`123`), "123", 1},
		{All(`false`), "true", 0},

		// wildcards
		{All(`$x`), "rune", 1},
		{All(`foo($x, $x)`), "foo(1, 2)", 0},
		{All(`foo($_, $_)`), "foo(1, 2)", 1},
		{All(`foo($x, $y, $y)`), "foo(1, 2, 2)", 1},
		{All(`$x`), `"foo"`, 1},

		// recursion
		{All(`$x`), "a + b", 3},
		{All(`$x + $x`), "foo(a + a, b + b)", 2},
		{All(`$x`), "var a int", 4},
		{All(`go foo()`), "a(); go foo(); a()", 1},

		// ident regex matches
		{
			Pipe(All(`$x`), Regx(`foo`)),
			"bar", 0,
		},
		{
			Pipe(All(`$x`), Regx(`foo`)),
			"foo", 1,
		},
		{
			Pipe(All(`$x`), Regx(`foo`)),
			"_foo", 0,
		},
		{
			Pipe(All(`$x`), Regx(`foo`)),
			"foo_", 0,
		},
		{
			Pipe(All(`$x`), Regx(`.*foo.*`)),
			"_foo_", 1,
		},
		{
			Pipe(All(`$x = $_`), All(`$x`), Regx(`.*`)),
			"a = b", 1,
		},
		{
			Pipe(All(`$x = $_`), All(`$x`), Regx(`.*`)),
			"a.field = b", 0,
		},
		{
			Pipe(All(`$x`), Regx(`.*foo.*`), Regx(`.*bar.*`)),
			"foobar; barfoo; foo; barbar", 2,
		},

		// type equality
		{
			Pipe(All(`$x`), Type(`int`)),
			"var i int", 2, // includes "int" the type
		},
		{
			Pipe(All(`append($x)`), All(`$x`), Type(`[]int`)),
			"var _ = append([]int32{3})", 0,
		},
		{
			Pipe(All(`append($x)`), All(`$x`), Type(`[]int`)),
			"var _ = append([]int{3})", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Type(`[2]int`)),
			"var _ = [...]int{1}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Type(`[2]int`)),
			"var _ = [...]int{1, 2}", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Type(`[2]int`)),
			"var _ = []int{1, 2}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Type(`*int`)),
			"var _ = int(3)", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Type(`*int`)),
			"var _ = new(int)", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Type(`io.Reader`)),
			`import "io"; var _ = io.Writer(nil)`, 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Type(`io.Reader`)),
			`import "io"; var _ = io.Reader(nil)`, 1,
		},
		{
			Pipe(All(`$x`), Type(`int`)),
			`type I int; func (i I) p() { print(i) }`, 1,
		},
		{
			Pipe(All(`$x`), Type(`*I`)),
			`type I int; var i *I`, 2,
		},
		// TODO
		// {
		//	Pipe(All(`$x`), Type(`chan int`)),
		// 	`ch := make(chan int)`, 2,
		// },

		// type assignability
		{
			Pipe(All(`const _ = $x`), All(`$x`), Type(`int`)),
			"const _ = 3", 0,
		},
		{
			Pipe(All(`var $x $_`), All(`$x`), Type(`io.Reader`)),
			`import "os"; var f *os.File`, 0,
		},
		{
			Pipe(All(`var $x $_`), All(`$x`), Asgn(`io.Reader`)),
			`import "os"; var f *os.File`, 1,
		},
		{
			Pipe(All(`var $x $_`), All(`$x`), Asgn(`io.Writer`)),
			`import "io"; var r io.Reader`, 0,
		},
		{
			Pipe(All(`var $_ $_ = $x`), All(`$x`), Asgn(`*url.URL`)),
			`var _ interface{} = 0`, 0,
		},
		{
			Pipe(All(`var $_ $_ = $x`), All(`$x`), Asgn(`*url.URL`)),
			`var _ interface{} = nil`, 1,
		},

		// type conversions
		{
			Pipe(All(`const _ = $x`), All(`$x`), Type(`int`)),
			"const _ = 3", 0,
		},
		{
			Pipe(All(`const _ = $x`), All(`$x`), Conv(`int`)),
			"const _ = 3", 1,
		},
		{
			Pipe(All(`const _ = $x`), All(`$x`), Conv(`int32`)),
			"const _ = 3", 1,
		},
		{
			Pipe(All(`const _ = $x`), All(`$x`), Conv(`[]byte`)),
			"const _ = 3", 0,
		},
		{
			Pipe(All(`var $x $_`), All(`$x`), Type(`int`)),
			"type I int; var i I", 0,
		},
		{
			Pipe(All(`var $x $_`), All(`$x`), Conv(`int`)),
			"type I int; var i I", 1,
		},

		// comparable types
		{
			Pipe(All(`var _ = $x`), All(`$x`), Comp),
			"var _ = []byte{0}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Comp),
			"var _ = [...]byte{0}", 1,
		},

		// addressable expressions
		{
			Pipe(All(`var _ = $x`), All(`$x`), Addr),
			"var _ = []byte{0}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Addr),
			"var s struct { i int }; var _ = s.i", 1,
		},

		// underlying types
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Basic)),
			"var _ = []byte{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Basic)),
			"var _ = 3", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Basic)),
			`import "io"; var _ = io.SeekEnd`, 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Array)),
			"var _ = []byte{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Array)),
			"var _ = [...]byte{}", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Slice)),
			"var _ = []byte{}", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Slice)),
			"var _ = [...]byte{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Struct)),
			"var _ = []byte{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Struct)),
			"var _ = struct{}{}", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Interface)),
			"var _ = struct{}{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Interface)),
			"var _ = interface{}(nil)", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Pointer)),
			"var _ = []byte{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Pointer)),
			"var _ = new(byte)", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Func)),
			"var _ = []byte{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Func)),
			"var _ = func() {}", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Map)),
			"var _ = []byte{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Map)),
			"var _ = map[int]int{}", 1,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Chan)),
			"var _ = []byte{}", 0,
		},
		{
			Pipe(All(`var _ = $x`), All(`$x`), Kind(Chan)),
			"var _ = make(chan int)", 1,
		},

		// many value expressions
		{All(`$x, $y`), "foo(1, 2)", 1},
		{All(`$x, $y`), "1", 0},
		{All(`$x`), "a, b", 3},
		// unlike statements, expressions don't automatically
		// imply partial matches
		{All(`b, c`), "a, b, c, d", 0},
		{All(`b, c`), "foo(a, b, c, d)", 0},
		{All(`print($*_, $x)`), "print(a, b, c)", 1},

		// any number of expressions
		{All(`$*x`), "a, b", "a, b"},
		{All(`print($*x)`), "print()", 1},
		{All(`print($*x)`), "print(a, b)", 1},
		{All(`print($*x, $y, $*z)`), "print()", 0},
		{All(`print($*x, $y, $*z)`), "print(a)", 1},
		{All(`print($*x, $y, $*z)`), "print(a, b, c)", 1},
		{All(`{ $*_; return nil }`), "{ return nil }", 1},
		{All(`{ $*_; return nil }`), "{ a(); b(); return nil }", 1},
		{All(`c($*x); c($*x)`), "c(); c()", 1},
		{All(`c($*x); c()`), "c(); c()", 1},
		{All(`c($*x); c($*x)`), "c(x); c(y)", 0},
		{All(`c($*x); c($*x)`), "c(x, y); c(z)", 0},
		{All(`c($*x); c($*x)`), "c(x, y); c(x, y)", 1},
		{All(`c($*x, y); c($*x, y)`), "c(x, y); c(x, y)", 1},
		{All(`c($*x, $*y); c($*x, $*y)`), "c(x, y); c(x, y)", 1},

		// composite lits
		{All(`[]float64{$x}`), "[]float64{3}", 1},
		{All(`[2]bool{$x, 0}`), "[2]bool{3, 1}", 0},
		{All(`someStruct{fld: $x}`), "someStruct{fld: a, fld2: b}", 0},
		{All(`map[int]int{1: $x}`), "map[int]int{1: a}", 1},

		// func lits
		{All(`func($s string) { print($s) }`), "func(a string) { print(a) }", 1},
		{All(`func($x ...$t) {}`), "func(a ...int) {}", 1},

		// type exprs
		{All(`[8]$x`), "[8]int", 1},
		{All(`struct{field $t}`), "struct{field int}", 1},
		{All(`struct{field $t}`), "struct{field int}", 1},
		{All(`struct{field $t}`), "struct{other int}", 0},
		{All(`struct{field $t}`), "struct{f1, f2 int}", 0},
		{All(`interface{$x() int}`), "interface{i() int}", 1},
		{All(`chan $x`), "chan bool", 1},
		{All(`<-chan $x`), "chan bool", 0},
		{All(`chan $x`), "chan<- bool", 0},

		// many types (TODO; revisit)
		// {Pipe(All(`chan $x, interface{}`)), "chan int, interface{}", 1},
		// {Pipe(All(`chan $x, interface{}`)), "chan int", 0},
		// {Pipe(All(`$x string, $y int`)), "func(s string, i int) {}", 1},

		// parens
		{All(`($x)`), "(a + b)", 1},
		{All(`($x)`), "a + b", 0},

		// unary ops
		{All(`-someConst`), "- someConst", 1},
		{All(`*someVar`), "* someVar", 1},

		// binary ops
		{All(`$x == $y`), "a == b", 1},
		{All(`$x == $y`), "123", 0},
		{All(`$x == $y`), "a != b", 0},
		{All(`$x - $x`), "a - b", 0},

		// calls
		{All(`someFunc($x)`), "someFunc(a > b)", 1},

		// selector
		{All(`$x.Field`), "a.Field", 1},
		{All(`$x.Field`), "a.field", 0},
		{All(`$x.Method()`), "a.Method()", 1},
		{All(`a.b`), "a.b.c", 1},
		{All(`b.c`), "a.b.c", 0},
		{All(`$x.c`), "a.b.c", 1},
		{All(`a.$x`), "a.b.c", 1},

		// indexes
		{All(`$x[len($x)-1]`), "a[len(a)-1]", 1},
		{All(`$x[len($x)-1]`), "a[len(b)-1]", 0},

		// slicing
		{All(`$x[:$y]`), "a[:1]", 1},
		{All(`$x[3:]`), "a[3:5:5]", 0},

		// type asserts
		{All(`$x.(string)`), "a.(string)", 1},

		// elipsis
		{All(`append($x, $y...)`), "append(a, bs...)", 1},
		{All(`foo($x...)`), "foo(a)", 0},
		{All(`foo($x...)`), "foo(a, b)", 0},

		// forcing node to be a statement
		{All(`append($*_);`), "f(); x = append(x, a)", 0},
		{All(`append($*_);`), "f(); append(x, a)", 1},

		// many statements
		{All(`$x(); $y()`), "a(); b()", 1},
		{All(`$x(); $y()`), "a()", 0},
		{All(`$x`), "a; b", 3},
		{All(`b; c`), "b", 0},
		{All(`b; c`), "b; c", 1},
		{All(`b; c`), "b; x; c", 0},
		{All(`b; c`), "a; b; c; d", "b; c"},
		{All(`b; c`), "{b; c; d}", 1},
		{All(`b; c`), "{a; b; c}", 1},
		{All(`b; c`), "{b; b; c; c}", "b; c"},
		{All(`$x++; $x--`), "n; a++; b++; b--", "b++; b--"},
		{All(`$*_; b; $*_`), "{a; b; c; d}", "a; b; c; d"},
		{All(`{$*_; $x}`), "{a; b; c}", 1},
		{All(`{b; c}`), "{a; b; c}", 0},
		{All(`$x := $_; $x = $_`), "a := n; b := n; b = g", "b := n; b = g"},
		{All(`$x := $_; $*_; $x = $_`), "a := n; b := n; b = g", "b := n; b = g"},

		// mixing lists
		{All(`$x, $y`), "1; 2", 0},
		{All(`$x; $y`), "1, 2", 0},

		// any number of statements
		{All(`$*x`), "a; b", "a; b"},
		{All(`$*x; b; $*y`), "a; b; c", 1},
		{All(`$*x; b; $*x`), "a; b; c", 0},

		// const/var declarations
		{All(`const $x = $y`), "const a = b", 1},
		{All(`const $x = $y`), "const (a = b)", 1},
		{All(`const $x = $y`), "const (a = b\nc = d)", 0},
		{All(`var $x int`), "var a int", 1},
		{All(`var $x int`), "var a int = 3", 0},

		// func declarations
		{
			All(`func $_($x $y) $y { return $x }`),
			"func a(i int) int { return i }", 1,
		},
		{All(`func $x(i int)`), "func a(i int)", 1},
		{All(`func $x(i int) {}`), "func a(i int)", 0},
		{
			All(`func $_() $*_ { $*_ }`),
			"func f() {}", 1,
		},
		{
			All(`func $_() $*_ { $*_ }`),
			"func f() (int, error) { return 3, nil }", 1,
		},

		// type declarations
		{All(`struct{}`), "type T struct{}", 1},
		{All(`type $x struct{}`), "type T struct{}", 1},
		{All(`struct{$_ int}`), "type T struct{n int}", 1},
		{All(`struct{$_ int}`), "var V struct{n int}", 1},
		{All(`struct{$_}`), "type T struct{n int}", 1},
		{All(`struct{$*_}`), "type T struct{n int}", 1},
		{
			All(`struct{$*_; Foo $t; $*_}`),
			"type T struct{Foo string; a int; B}", 1,
		},

		// value specs
		{All(`$_ int`), "var a int", 1},
		{All(`$_ int`), "var a bool", 0},
		// TODO: consider these
		{All(`$_ int`), "var a int = 3", 0},
		{All(`$_ int`), "var a, b int", 0},
		{All(`$_ int`), "func(i int) { println(i) }", 0},

		// entire files
		{All(`package $_`), "package p; var a = 1", 0},
		{All(`package $_; func Foo() { $*_ }`), "package p; func Foo() {}", 1},

		// blocks
		{All(`{ $x }`), "{ a() }", 1},
		{All(`{ $x }`), "{ a(); b() }", 0},
		{All(`{}`), "func f() {}", 1},

		// assigns
		{All(`$x = $y`), "a = b", 1},
		{All(`$x := $y`), "a, b := c()", 0},

		// if stmts
		{All(`if $x != nil { $y }`), "if p != nil { p.foo() }", 1},
		{All(`if $x { $y }`), "if a { b() } else { c() }", 0},
		{All(`if $x != nil { $y }`), "if a != nil { return a }", 1},

		// for and range stmts
		{All(`for $x { $y }`), "for b { c() }", 1},
		{All(`for $x := range $y { $z }`), "for i := range l { c() }", 1},
		{All(`for $x := range $y { $z }`), "for i = range l { c() }", 0},
		{All(`for $x = range $y { $z }`), "for i := range l { c() }", 0},
		{All(`for range $y { $z }`), "for _, e := range l { e() }", 0},

		// $*_ matching stmt+expr combos (ifs)
		{All(`if $*x {}`), "if a {}", 1},
		{All(`if $*x {}`), "if a(); b {}", 1},
		{All(`if $*x {}; if $*x {}`), "if a(); b {}; if a(); b {}", 1},
		{All(`if $*x {}; if $*x {}`), "if a(); b {}; if b {}", 0},
		{All(`if $*_ {} else {}`), "if a(); b {}", 0},
		{All(`if $*_ {} else {}`), "if a(); b {} else {}", 1},
		{All(`if a(); $*_ {}`), "if b {}", 0},

		// $*_ matching stmt+expr combos (fors)
		{All(`for $*x {}`), "for {}", 1},
		{All(`for $*x {}`), "for a {}", 1},
		{All(`for $*x {}`), "for i(); a; p() {}", 1},
		{All(`for $*x {}; for $*x {}`), "for i(); a; p() {}; for i(); a; p() {}", 1},
		{All(`for $*x {}; for $*x {}`), "for i(); a; p() {}; for i(); b; p() {}", 0},
		{All(`for a(); $*_; {}`), "for b {}", 0},
		{All(`for ; $*_; c() {}`), "for b {}", 0},

		// $*_ matching stmt+expr combos (switches)
		{All(`switch $*x {}`), "switch a {}", 1},
		{All(`switch $*x {}`), "switch a(); b {}", 1},
		{All(`switch $*x {}; switch $*x {}`), "switch a(); b {}; switch a(); b {}", 1},
		{All(`switch $*x {}; switch $*x {}`), "switch a(); b {}; switch b {}", 0},
		{All(`switch a(); $*_ {}`), "for b {}", 0},

		// $*_ matching stmt+expr combos (node type mixing)
		{All(`if $*x {}; for $*x {}`), "if a(); b {}; for a(); b; {}", 1},
		{All(`if $*x {}; for $*x {}`), "if a(); b {}; for a(); b; c() {}", 0},

		// for $*_ {} matching a range for
		{All(`for $_ {}`), "for range x {}", 0},
		{All(`for $*_ {}`), "for range x {}", 1},
		{All(`for $*_ {}`), "for _, v := range x {}", 1},

		// $*_ matching optional statements (ifs)
		{All(`if $*_; b {}`), "if b {}", 1},
		{All(`if $*_; b {}`), "if a := f(); b {}", 1},
		// TODO: should these match?
		//{Pipe(All(`if a(); $*x { f($*x) }`)), "if a(); b { f(b) }", 1},
		//{Pipe(All(`if a(); $*x { f($*x) }`)), "if a(); b { f(b, c) }", 0},
		//{Pipe(All(`if $*_; $*_ {}`)), "if a(); b {}", 1},

		// $*_ matching optional statements (fors)
		{All(`for $*x; b; $*x {}`), "for b {}", 1},
		{All(`for $*x; b; $*x {}`), "for a(); b; a() {}", 1},
		{All(`for $*x; b; $*x {}`), "for a(); b; c() {}", 0},

		// $*_ matching optional statements (switches)
		{All(`switch $*_; b {}`), "switch b := f(); b {}", 1},
		{All(`switch $*_; b {}`), "switch b := f(); c {}", 0},

		// inc/dec stmts
		{All(`$x++`), "a[b]++", 1},
		{All(`$x--`), "a++", 0},

		// returns
		{All(`return nil, $x`), "{ return nil, err }", 1},
		{All(`return nil, $x`), "{ return nil, 0, err }", 0},

		// go stmts
		{All(`go $x()`), "go func() { a() }()", 1},
		{All(`go func() { $x }()`), "go func() { a() }()", 1},
		{All(`go func() { $x }()`), "go a()", 0},

		// defer stmts
		{All(`defer $x()`), "defer func() { a() }()", 1},
		{All(`defer func() { $x }()`), "defer func() { a() }()", 1},
		{All(`defer func() { $x }()`), "defer a()", 0},

		// empty statement
		{All(`;`), ";", 1},

		// labeled statement
		{All(`foo: a`), "foo: a", 1},
		{All(`foo: a`), "foo: b", 0},

		// send statement
		{All(`x <- 1`), "x <- 1", 1},
		{All(`x <- 1`), "y <- 1", 0},
		{All(`x <- 1`), "x <- 2", 0},

		// branch statement
		{All(`break foo`), "break foo", 1},
		{All(`break foo`), "break bar", 0},
		{All(`break foo`), "continue foo", 0},
		{All(`break`), "break", 1},
		{All(`break foo`), "break", 0},

		// case clause
		{All(`switch x {case 4: x}`), "switch x {case 4: x}", 1},
		{All(`switch x {case 4: x}`), "switch y {case 4: x}", 0},
		{All(`switch x {case 4: x}`), "switch x {case 5: x}", 0},
		{All(`switch {$_}`), "switch {case 5: x}", 1},
		{All(`switch x {$_}`), "switch x {case 5: x}", 1},
		{All(`switch x {$*_}`), "switch x {case 5: x}", 1},
		{All(`switch x {$*_}`), "switch x {}", 1},
		{All(`switch x {$*_}`), "switch x {case 1: a; case 2: b}", 1},
		{All(`switch {$a; $a}`), "switch {case true: a; case true: a}", 1},
		{All(`switch {$a; $a}`), "switch {case true: a; case true: b}", 0},

		// switch statement
		{All(`switch x; y {}`), "switch x; y {}", 1},
		{All(`switch x {}`), "switch x; y {}", 0},
		{All(`switch {}`), "switch {}", 1},
		{All(`switch {}`), "switch x {}", 0},
		{All(`switch {}`), "switch {case y:}", 0},
		{All(`switch $_ {}`), "switch x {}", 1},
		{All(`switch $_ {}`), "switch x; y {}", 0},
		{All(`switch $_; $_ {}`), "switch x {}", 0},
		{All(`switch $_; $_ {}`), "switch x; y {}", 1},
		{All(`switch { $*_; case $*_: $*a }`), "switch { case x: y() }", 0},

		// type switch statement
		{All(`switch x := y.(z); x {}`), "switch x := y.(z); x {}", 1},
		{All(`switch x := y.(z); x {}`), "switch y := y.(z); x {}", 0},
		{All(`switch x := y.(z); x {}`), "switch y := y.(z); x {}", 0},
		// TODO more switch variations.

		// TODO select statement
		// TODO communication clause
		{All(`select {$*_}`), "select {case <-x: a}", 1},
		{All(`select {$*_}`), "select {}", 1},
		{All(`select {$a; $a}`), "select {case <-x: a; case <-x: a}", 1},
		{All(`select {$a; $a}`), "select {case <-x: a; case <-x: b}", 0},
		{All(`select {case x := <-y: f(x)}`), "select {case x := <-y: f(x)}", 1},

		// aggressive mode
		{All(`for range $x {}`), "for _ = range a {}", 0},
		{All(`~ for range $x {}`), "for _ = range a {}", 1},
		{All(`~ for _ = range $x {}`), "for range a {}", 1},
		{All(`a int`), "var (a, b int; c bool)", 0},
		{All(`~ a int`), "var (a, b uint; c bool)", 0},
		{All(`~ a int`), "var (a, b int; c bool)", 1},
		{All(`~ a int`), "var (a, b int; c bool)", 1},
		{All(`{ x; }`), "switch { case true: x; }", 0},
		{All(`~ { x; }`), "switch { case true: x; }", 1},
		{All(`a = b`), "a = b; a := b", 1},
		{All(`a := b`), "a = b; a := b", 1},
		{All(`~ a = b`), "a = b; a := b; var a = b", 3},
		{All(`~ a := b`), "a = b; a := b; var a = b", 3},

		// many cmds
		{
			All(`break`),
			"switch { case x: break }; for { y(); break; break }",
			3,
		},
		{
			Pipe(All(`for { $*_ }`), All(`break`)),
			"switch { case x: break }; for { y(); break; break }",
			2,
		},
		{
			Pipe(All(`for { $*_ }`), Incl(`break`)),
			"break; for {}; for { if x { break } else { break } }",
			1,
		},
		{
			Pipe(All(`for { $*_ }`), Excl(`break`)),
			"break; for {}; for { x() }; for { break }",
			2,
		},
		{
			Pipe(All(`for { $*sts }`), All(`$*sts`)),
			"for { a(); b() }",
			"a(); b()",
		},
		{
			Pipe(All(`for { $*sts }`), All(`$*sts`)),
			"for { if x { a(); b() } }",
			"if x { a(); b(); }",
		},
		{
			Pipe(All(`foo`), Suggest(`bar`)),
			`foo(); println("foo"); println(foo, foobar)`,
			`bar(); println("foo"); println(bar, foobar)`,
		},
		{
			Pipe(All(`$f()`), Suggest(`$f(nil)`)),
			`foo(); bar(); baz(x)`,
			`foo(nil); bar(nil); baz(x)`,
		},
		{
			Pipe(All(`foo($*_)`), Suggest(`foo()`)),
			`foo(); foo(a, b); bar(x)`,
			`foo(); foo(); bar(x)`,
		},
		{
			Pipe(All(`a, b`), Suggest(`c, d`)),
			`foo(); foo(a, b); bar(a, b)`,
			`foo(); foo(c, d); bar(c, d)`,
		},
		{
			Pipe(All(`a(); b()`), Suggest(`c(); d()`)),
			`{ a(); b(); c(); }; { a(); a(); b(); }`,
			`{ c(); d(); c(); }; { a(); c(); d(); }`,
		},
		{
			Pipe(All(`a()`), Suggest(`c()`)),
			`{ a(); b(); a(); }`,
			`{ c(); b(); c(); }`,
		},
		{
			Pipe(All(`go func() { $f() }()`), Suggest(`go $f()`)),
			`{ go func() { f.Close() }(); }`,
			`{ go f.Close(); }`,
		},
		{
			Pipe(All(`foo`), Suggest(`bar`)),
			`package p; var foo int`,
			`package p; var bar int`,
		},
		{
			Pipe(All(`foo($*a)`), Suggest(`bar($*a)`)),
			`{ foo(); }`,
			`{ bar(); }`,
		},
		{
			Pipe(All(`foo($*a)`), Suggest(`bar($*a)`)),
			`{ foo(0); }`,
			`{ bar(0); }`,
		},
		{
			Pipe(All(`a(); b()`), Suggest(`x = a()`)),
			`{ a(); b(); }`,
			`{ x = a(); }`,
		},
		{
			Pipe(All(`a(); b()`), Suggest(`a()`)),
			`{ a(); b(); }`,
			`{ a(); }`,
		},
		{
			Pipe(All(`a, b`), Suggest(`c`)),
			`foo(a, b)`,
			`foo(c)`,
		},
		{
			Pipe(All(`b = a()`), Suggest(`c()`)),
			`if b = a(); b { }`,
			`if c(); b { }`,
		},
		// {
		// 	Pipe(All(`foo()", "-p", "1`)),
		// 	`{ if foo() { bar(); }; etc(); }`,
		// 	`if foo() { bar(); }`,
		// },
		{
			Pipe(All(`f($*a)`), Suggest(`f2(x, $a)`)),
			`f(c, d)`,
			`f2(x, c, d)`,
		},
		{
			Pipe(All(`err = f(); if err != nil { $*then }`), Suggest(`if err := f(); err != nil { $then }`)),
			`{ err = f(); if err != nil { handle(err); }; }`,
			`{ if err := f(); err != nil { handle(err); }; }`,
		},
		{
			Pipe(All(`List{$e}`), Suggest(`$e`)),
			`List{foo()}`,
			`foo()`,
		},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%03d", i), func(t *testing.T) {
			grepTest(t, tc.fn, tc.input, tc.want)
		})
	}
}

type wantMultiline string

func TestMatchMultiline(t *testing.T) {
	tests := []struct {
		fn   Function
		src  string
		want string
	}{
		{
			Pipe(All(`List{$e}`), Suggest(`$e`)),
			"return List{\n\tfoo(),\n}",
			"return foo()",
		},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%03d", i), func(t *testing.T) {
			grepTest(t, tc.fn, tc.src, wantMultiline(tc.want))
		})
	}
}

func grepTest(t *testing.T, fn Function, input string, want interface{}) {
	g := &G{Fset: token.NewFileSet()}
	node, err := load.Input(g, input)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := g.Run(fn, node)
	if want, ok := want.(wantErr); ok {
		if err == nil {
			t.Fatalf("wanted error %q, got none", want)
		} else if got := err.Error(); !strings.Contains(got, string(want)) {
			t.Fatalf("wanted error %q, got %q", want, got)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want, ok := want.(int); ok {
		if len(matches) != want {
			t.Fatalf("wanted %d matches, got %d", want, len(matches))
		}
		return
	}
	if len(matches) != 1 {
		t.Fatalf("wanted 1 match, got %d", len(matches))
	}
	var got, wantStr string
	switch want := want.(type) {
	case string:
		wantStr = want
		got = gsyntax.PrintCompact(matches[0])
	case wantMultiline:
		wantStr = string(want)
		var buf bytes.Buffer
		gsyntax.Print(&buf, g.Fset, matches[0])
		got = buf.String()
	default:
		panic(fmt.Sprintf("unexpected want type: %T", want))
	}
	if got != wantStr {
		t.Fatalf("wanted %q, got %q", wantStr, got)
	}
}
