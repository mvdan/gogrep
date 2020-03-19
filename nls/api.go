// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

// Package nls provides support for querying Go code. It is intended to be used
// with the "gogrep" tool, which executes top-level query functions of the form:
//
//     func Name(*nls.G)
//
// G keeps track of the current matches in the input source, as well as
// supporting types to obtain position information and type information.
// When a query function is executed, G's initial matches are simply the input
// source code, such as the files in any input packages.
//
// Searching
//
// Most of the methods on G allow navigating and changing the current set of
// matches. Some allow finding sub-matches, like All, while others allow
// filtering out the current matches, such as Including.
//
// Syntax
//
// A pattern is a piece of Go code which may include dollar expressions. It can
// be a number of statements, a number of expressions, a declaration, or an
// entire file.
//
// A dollar expression consist of '$' and a name. Dollar expressions with the
// same name match the same node, excluding "_". For example:
//
//     g.All("$x.$_ = $x") // assignment of self to a field in self
//
// If '*' is before the name, it will match any number of nodes. Example:
//
//     g.All("fmt.Fprintf(os.Stdout, $*_)") // all Fprintf calls on stdout
//
// Results
//
// By default, the matches that remain when reaching the end of a query function
// are printed out to the user, including the position information and the
// syntax node. The user can choose to use a custom message via Report, or
// replace those matches in-place with other code via Suggest.
package nls

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"regexp"
	"strings"

	"mvdan.cc/gogrep/gsyntax"
)

type G struct {
	Tests bool

	Fset *token.FileSet

	current []Match
	Scope   *types.Scope
	*types.Info

	parents map[ast.Node]ast.Node

	aggressive bool

	// information about variables (wildcards), by id (which is an
	// integer starting at 0)
	vars []varInfo

	// node values recorded by name, excluding "_" (used only by the
	// actual matching phase)
	values map[string]ast.Node

	stdImporter types.Importer
}

type Match struct {
	Node   ast.Node
	Values map[string]ast.Node
}

// Expr is short for
//
//     expr, _ := m.Node.(ast.Expr).
func (m Match) Expr() ast.Expr {
	expr, _ := m.Node.(ast.Expr)
	return expr
}

type runError error

func (g *G) Fatal(err error) {
	panic(runError(err))
}

type varInfo struct {
	name string
	any  bool
}

type Function func(*G)

func (g *G) Run(fn Function, nodes ...ast.Node) (_ []ast.Node, err error) {
	defer func() {
		if e, _ := recover().(runError); e != nil {
			err = e
		}
	}()
	g.parents = make(map[ast.Node]ast.Node)
	g.fillParents(nodes...)
	g.current = make([]Match, len(nodes))
	for i, node := range nodes {
		g.current[i].Node = node
		g.current[i].Values = make(map[string]ast.Node)
	}

	fn(g)
	result := make([]ast.Node, len(g.current))
	for i, m := range g.current {
		result[i] = m.Node
	}
	return result, nil
}

func (g *G) fillParents(nodes ...ast.Node) {
	stack := make([]ast.Node, 1, 32)
	for _, node := range nodes {
		gsyntax.Inspect(node, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			if _, ok := node.(gsyntax.NodeList); !ok {
				g.parents[node] = stack[len(stack)-1]
			}
			stack = append(stack, node)
			return true
		})
	}
}

func (g *G) info(id int) varInfo {
	if id < 0 {
		return varInfo{}
	}
	return g.vars[id]
}
func With(fns ...Function) Function { return func(g *G) { g.With(fns...) } }

func All(expr string) Function     { return func(g *G) { g.All(expr) } }
func Incl(expr string) Function    { return func(g *G) { g.Including(expr) } }
func Excl(expr string) Function    { return func(g *G) { g.Excluding(expr) } }
func Replace(expr string) Function { return func(g *G) { g.Replace(expr) } }
func Suggest(expr string) Function { return func(g *G) { g.Suggest(expr) } }

func Regx(expr string) Function    { return func(g *G) { g.Regexp(expr) } }
func Kind(which KindType) Function { return func(g *G) { g.Kind(which) } }
func Type(expr string) Function    { return func(g *G) { g.Type(expr) } }
func Asgn(expr string) Function    { return func(g *G) { g.Assignable(expr) } }
func Conv(expr string) Function    { return func(g *G) { g.Convertible(expr) } }
func Comp(g *G)                    { g.Comparable() }
func Addr(g *G)                    { g.Addressable() }

func (g *G) matchFilter(fn func(Match) bool) {
	var next []Match
	for _, m := range g.current {
		if fn(m) {
			next = append(next, m)
		}
	}
	g.current = next
}

// TODO: are Map/Filter useful at all?

func (g *G) _Map(fn func(Match) Match) {
	for i, m := range g.current {
		g.current[i] = fn(m)
	}
}

func (g *G) filter(fn func(*G) bool) {
	var next []Match
	for i, m := range g.current {
		g2 := *g
		g2.current = g.current[i : i+1]
		if fn(&g2) {
			next = append(next, m)
		}
	}
	g.current = next
}

func (g *G) With(fns ...Function) {
	g.filter(func(g *G) bool {
		for _, fn := range fns {
			fn(g)
		}
		return len(g.current) > 0
	})
}

// All replaces the current set of matches with the set of sub-matches which
// match the given pattern.
func (g *G) All(pattern string) {
	var next []Match
	seen := map[nodePosHash]bool{}

	exprNode, err := g.parseExpr(pattern)
	if err != nil {
		g.Fatal(err)
	}

	// The values context for each new Match must be a new copy from its
	// parent Match. If we don't do this copy, all the Matches would share
	// the same map and have side effects.
	var startValues map[string]ast.Node

	match := func(exprNode, node ast.Node) {
		if node == nil {
			return
		}
		g.values = valsCopy(startValues)
		found := g.topNode(exprNode, node)
		if found == nil {
			return
		}
		hash := posHash(found)
		if !seen[hash] {
			next = append(next, Match{
				Node:   found,
				Values: g.values,
			})
			seen[hash] = true
		}
	}
	for _, m := range g.current {
		startValues = valsCopy(m.Values)
		g.walkWithLists(exprNode, m.Node, match)
	}
	g.current = next
}

func (g *G) including(pattern string, wantAny bool) {
	exprNode, err := g.parseExpr(pattern)
	if err != nil {
		g.Fatal(err)
	}

	any := false
	match := func(exprNode, node ast.Node) {
		if node == nil {
			return
		}
		found := g.topNode(exprNode, node)
		if found != nil {
			any = true
		}
	}
	g.matchFilter(func(m Match) bool {
		any = false
		g.values = m.Values
		g.walkWithLists(exprNode, m.Node, match)
		return any == wantAny
	})
}

// Including filters the current set of matches by discarding those that do not
// contain any sub-match for pattern.
func (g *G) Including(pattern string) {
	g.including(pattern, true)
}

// Including filters the current set of matches by discarding those that contain
// any sub-match for pattern.
func (g *G) Excluding(pattern string) {
	g.including(pattern, false)
}

func (g *G) Regexp(expr string) {
	if !strings.HasPrefix(expr, "^") {
		expr = "^" + expr
	}
	if !strings.HasSuffix(expr, "$") {
		expr = expr + "$"
	}
	rx := regexp.MustCompile(expr)

	g.matchFilter(func(m Match) bool {
		// TODO: stringify any node?
		str := nodeString(m.Node)
		return str != "" && rx.MatchString(str)
	})
}

type KindType uint8

const (
	_ KindType = iota
	Basic
	Array
	Slice
	Struct
	Interface
	Pointer
	Func
	Map
	Chan
)

func (g *G) Kind(which KindType) {
	g.typFilter(func(typ types.Type) bool {
		u := typ.Underlying()
		uok := true
		switch which {
		case Basic:
			_, uok = u.(*types.Basic)
		case Array:
			_, uok = u.(*types.Array)
		case Slice:
			_, uok = u.(*types.Slice)
		case Struct:
			_, uok = u.(*types.Struct)
		case Interface:
			_, uok = u.(*types.Interface)
		case Pointer:
			_, uok = u.(*types.Pointer)
		case Func:
			_, uok = u.(*types.Signature)
		case Map:
			_, uok = u.(*types.Map)
		case Chan:
			_, uok = u.(*types.Chan)
		}
		return uok
	})
}

func (g *G) Type(expr string) {
	want := g.resolveTypeStr(expr)
	g.typFilter(func(typ types.Type) bool {
		return types.Identical(typ, want)
	})
}

func (g *G) Assignable(expr string) {
	want := g.resolveTypeStr(expr)
	g.typFilter(func(typ types.Type) bool {
		return types.AssignableTo(typ, want)
	})
}

func (g *G) Convertible(expr string) {
	want := g.resolveTypeStr(expr)
	g.typFilter(func(typ types.Type) bool {
		return types.ConvertibleTo(typ, want)
	})
}

func (g *G) Comparable() {
	g.typFilter(types.Comparable)
}

func (g *G) Addressable() {
	g.matchFilter(func(m Match) bool {
		return g.Types[m.Expr()].Addressable()
	})
}

func (g *G) typFilter(fn func(types.Type) bool) {
	g.matchFilter(func(m Match) bool {
		return fn(g.TypeOf(m.Expr()))
	})
}

func nodeString(node ast.Node) string {
	switch node := node.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.ExprStmt:
		return nodeString(node.X)
	default:
		return ""
	}
}

func (g *G) Report(message string) {
	wd, err := os.Getwd()
	if err != nil {
		g.Fatal(err)
	}
	for _, m := range g.current {
		message := os.Expand(message, func(name string) string {
			node, ok := m.Values[name]
			if !ok {
				return "$!{unknown: " + name + "}"
			}
			return gsyntax.PrintCompact(node)
		})
		fpos := g.Fset.Position(m.Node.Pos())
		if strings.HasPrefix(fpos.Filename, wd) {
			fpos.Filename = fpos.Filename[len(wd)+1:]
		}
		fmt.Printf("%v: %s\n", fpos, message)
	}
	g.current = nil
}
