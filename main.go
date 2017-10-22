// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/gogrep"

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/printer"
	"go/token"
	"go/types"
	"io"
	"os"
	"regexp"
	"strings"
)

var usage = func() {
	fmt.Fprint(os.Stderr, `usage: gogrep commands [packages]

Options:

  -r   match all dependencies recursively too

A command is of the form "-A pattern", where -A is one of:

  -x   find all nodes matching a pattern
  -g   discard nodes not matching a pattern
  -v   discard nodes matching a pattern

If -A is ommitted for a single command, -x will be assumed.

A pattern is a piece of Go code which may include wildcards. It can be:

       a statement (many if split by semicolonss)
       an expression (many if split by commas)
       a type expression
       a top-level declaration (var, func, const)
       an entire file

Wildcards consist of '$' and a name. All wildcards with the same name
within an expression must match the same node, excluding "_". Example:

       $x.$_ = $x // assignment of self to a field in self

If '*' is before the name, it will match any number of nodes. Example:

       fmt.Fprintf(os.Stdout, $*_) // all Fprintfs on stdout

Regexes can also be used to match certain identifier names only. The
'.*' pattern can be used to match all identifiers. Example:

       fmt.$(_ /Fprint.*/)(os.Stdout, $*_) // all Fprint* on stdout

The nodes resulting from applying the commands will be printed line by
line to standard output.
`)
}

func main() {
	m := matcher{
		out: os.Stdout,
		ctx: &build.Default,
	}
	err := m.fromArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type matcher struct {
	out io.Writer
	ctx *build.Context

	recursive         bool
	typed, aggressive bool

	// information about variables (wildcards), by id (which is an
	// integer starting at 0)
	vars []varInfo

	// node values recorded by name, excluding "_" (used only by the
	// actual matching phase)
	values map[string]ast.Node

	types.Info
}

type varInfo struct {
	name    string
	any     bool
	nameRxs []*regexp.Regexp
	types   []ast.Expr
}

func (m *matcher) info(id int) varInfo {
	if id < 0 {
		return varInfo{}
	}
	return m.vars[id]
}

type exprCmd struct {
	name string
	src  string
	node ast.Node
}

type orderedFlag struct {
	name string
	cmds *[]exprCmd
}

func (o *orderedFlag) String() string { return "" }
func (o *orderedFlag) Set(val string) error {
	*o.cmds = append(*o.cmds, exprCmd{name: o.name, src: val})
	return nil
}

func (m *matcher) fromArgs(args []string) error {
	cmds, paths, err := m.parseCmds(args)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	loader := nodeLoader{wd, m.ctx, fset}
	nodes, err := loader.load(paths, m.recursive, m.typed)
	if err != nil {
		return err
	}
	for _, n := range m.matches(cmds, nodes) {
		fpos := loader.fset.Position(n.Pos())
		if strings.HasPrefix(fpos.Filename, wd) {
			fpos.Filename = fpos.Filename[len(wd)+1:]
		}
		fmt.Fprintf(m.out, "%v: %s\n", fpos, singleLinePrint(n))
	}
	return nil
}

func (m *matcher) parseCmds(args []string) ([]exprCmd, []string, error) {
	flagSet := flag.NewFlagSet("gogrep", flag.ExitOnError)
	flagSet.Usage = usage
	flagSet.BoolVar(&m.recursive, "r", false, "match all dependencies recursively too")

	var cmds []exprCmd
	flagSet.Var(&orderedFlag{
		name: "x",
		cmds: &cmds,
	}, "x", "range over the matches")
	flagSet.Var(&orderedFlag{
		name: "g",
		cmds: &cmds,
	}, "g", "discard if there are no matches")
	flagSet.Var(&orderedFlag{
		name: "v",
		cmds: &cmds,
	}, "v", "discard if there are any matches")
	flagSet.Parse(args)
	paths := flagSet.Args()

	if len(cmds) == 0 && len(paths) > 0 {
		cmds = append(cmds, exprCmd{name: "x", src: paths[0]})
		paths = paths[1:]
	}
	if len(cmds) < 1 {
		return nil, nil, fmt.Errorf("need at least one command")
	}
	for i, cmd := range cmds {
		node, err := m.compileExpr(cmd.src)
		if err != nil {
			return nil, nil, err
		}
		cmds[i].node = node
	}
	return cmds, paths, nil
}

type bufferJoinLines struct {
	bytes.Buffer
	last string
}

var rxNeedSemicolon = regexp.MustCompile(`([])}a-zA-Z0-9"'` + "`" + `]|\+\+|--)$`)

func (b *bufferJoinLines) Write(p []byte) (n int, err error) {
	if string(p) == "\n" {
		if rxNeedSemicolon.MatchString(b.last) {
			b.Buffer.WriteByte(';')
		}
		b.Buffer.WriteByte(' ')
		return 1, nil
	}
	p = bytes.Trim(p, "\t")
	n, err = b.Buffer.Write(p)
	b.last = string(p)
	return
}

func singleLinePrint(node ast.Node) string {
	var buf bufferJoinLines
	printNode(&buf, token.NewFileSet(), node)
	return buf.String()
}

func printNode(w io.Writer, fset *token.FileSet, node ast.Node) {
	switch x := node.(type) {
	case exprList:
		if len(x) == 0 {
			return
		}
		printNode(w, fset, x[0])
		for _, n := range x[1:] {
			fmt.Fprintf(w, ", ")
			printNode(w, fset, n)
		}
	case stmtList:
		if len(x) == 0 {
			return
		}
		printNode(w, fset, x[0])
		for _, n := range x[1:] {
			fmt.Fprintf(w, "; ")
			printNode(w, fset, n)
		}
	default:
		err := printer.Fprint(w, fset, node)
		if err != nil && strings.Contains(err.Error(), "go/printer: unsupported node type") {
			// Should never happen, but make it obvious when it does.
			panic(fmt.Errorf("cannot print node: %v\n", node, err))
		}
	}
}

type lineColBuffer struct {
	bytes.Buffer
	line, col, offs int
}

func (l *lineColBuffer) WriteString(s string) (n int, err error) {
	for _, r := range s {
		if r == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.offs++
	}
	return l.Buffer.WriteString(s)
}

func (m *matcher) compileCmds(cmds []exprCmd) error {
	return nil
}

func (m *matcher) compileExpr(expr string) (node ast.Node, err error) {
	toks, err := m.tokenize([]byte(expr))
	if err != nil {
		return nil, fmt.Errorf("cannot tokenize expr: %v", err)
	}
	var offs []posOffset
	lbuf := lineColBuffer{line: 1, col: 1}
	addOffset := func(length int) {
		lbuf.offs -= length
		offs = append(offs, posOffset{
			atLine: lbuf.line,
			atCol:  lbuf.col,
			offset: length,
		})
	}
	if len(toks) > 0 && toks[0].tok == tokAggressive {
		toks = toks[1:]
		m.aggressive = true
	}
	lastLit := false
	for _, t := range toks {
		if lbuf.offs >= t.pos.Offset && lastLit && t.lit != "" {
			lbuf.WriteString(" ")
		}
		for lbuf.offs < t.pos.Offset {
			lbuf.WriteString(" ")
		}
		if t.lit == "" {
			lbuf.WriteString(t.tok.String())
			lastLit = false
			continue
		}
		if isWildName(t.lit) {
			// to correct the position offsets for the extra
			// info attached to ident name strings
			addOffset(len(wildPrefix) - 1)
		}
		lbuf.WriteString(t.lit)
		lastLit = strings.TrimSpace(t.lit) != ""
	}
	// trailing newlines can cause issues with commas
	exprStr := strings.TrimSpace(lbuf.String())
	if node, err = parse(exprStr); err != nil {
		err = subPosOffsets(err, offs...)
		return nil, fmt.Errorf("cannot parse expr: %v", err)
	}
	return node, nil
}
