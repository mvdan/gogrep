// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

func (m *matcher) parseExpr(expr string) (node ast.Node, err error) {
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
	if node, err = parseDetectingNode(exprStr); err != nil {
		err = subPosOffsets(err, offs...)
		return nil, fmt.Errorf("cannot parse expr: %v", err)
	}
	return node, nil
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

var tmplDecl = template.Must(template.New("").Parse(`` +
	`package p; {{ . }}`))

var tmplExprs = template.Must(template.New("").Parse(`` +
	`package p; var _ = []interface{}{ {{ . }}, }`))

var tmplStmts = template.Must(template.New("").Parse(`` +
	`package p; func _() { {{ . }} }`))

var tmplType = template.Must(template.New("").Parse(`` +
	`package p; var _ {{ . }}`))

var tmplValSpec = template.Must(template.New("").Parse(`` +
	`package p; var {{ . }}`))

func execTmpl(tmpl *template.Template, src string) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, src); err != nil {
		panic(err)
	}
	return buf.String()
}

func noBadNodes(node ast.Node) bool {
	any := false
	ast.Inspect(node, func(n ast.Node) bool {
		if any {
			return false
		}
		switch n.(type) {
		case *ast.BadExpr, *ast.BadDecl:
			any = true
		}
		return true
	})
	return !any
}

func parseDetectingNode(src string) (ast.Node, error) {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	scan := scanner.Scanner{}
	scan.Init(file, []byte(src), nil, 0)
	if _, tok, _ := scan.Scan(); tok == token.EOF {
		return nil, fmt.Errorf("empty source code")
	}
	var mainErr error

	// first try as a whole file
	if f, err := parser.ParseFile(fset, "", src, 0); err == nil && noBadNodes(f) {
		return f, nil
	}

	// then as a declaration
	asDecl := execTmpl(tmplDecl, src)
	if f, err := parser.ParseFile(fset, "", asDecl, 0); err == nil {
		if dc := f.Decls[0]; noBadNodes(dc) {
			return dc, nil
		}
	}

	// then as value expressions
	asExprs := execTmpl(tmplExprs, src)
	if f, err := parser.ParseFile(fset, "", asExprs, 0); err == nil {
		vs := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
		if cl := vs.Values[0].(*ast.CompositeLit); noBadNodes(cl) {
			if len(cl.Elts) == 1 {
				return cl.Elts[0], nil
			}
			return exprList(cl.Elts), nil
		}
	}

	// then try as statements
	asStmts := execTmpl(tmplStmts, src)
	if f, err := parser.ParseFile(fset, "", asStmts, 0); err == nil {
		if bl := f.Decls[0].(*ast.FuncDecl).Body; noBadNodes(bl) {
			if len(bl.List) == 1 {
				return bl.List[0], nil
			}
			return stmtList(bl.List), nil
		}
	} else {
		// Statements is what covers most cases, so it will give
		// the best overall error message. Show positions
		// relative to where the user's code is put in the
		// template.
		mainErr = subPosOffsets(err, posOffset{1, 1, 22})
	}

	// type expressions not yet picked up, for e.g. chans and interfaces
	asType := execTmpl(tmplType, src)
	if f, err := parser.ParseFile(fset, "", asType, 0); err == nil {
		vs := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
		if typ := vs.Type; noBadNodes(typ) {
			return typ, nil
		}
	}

	// value specs
	asValSpec := execTmpl(tmplValSpec, src)
	if f, err := parser.ParseFile(fset, "", asValSpec, 0); err == nil {
		vs := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
		if noBadNodes(vs) {
			return vs, nil
		}
	}
	return nil, mainErr
}

type posOffset struct {
	atLine, atCol int
	offset        int
}

func subPosOffsets(err error, offs ...posOffset) error {
	list, ok := err.(scanner.ErrorList)
	if !ok {
		return err
	}
	for i, err := range list {
		for _, off := range offs {
			if err.Pos.Line != off.atLine {
				continue
			}
			if err.Pos.Column < off.atCol {
				continue
			}
			err.Pos.Column -= off.offset
		}
		list[i] = err
	}
	return list
}

const (
	_ token.Token = -iota
	tokAggressive
)

type fullToken struct {
	pos token.Position
	tok token.Token
	lit string
}

type caseStatus uint

const (
	caseNone caseStatus = iota
	caseNeedBlock
	caseHere
)

func (m *matcher) tokenize(src []byte) ([]fullToken, error) {
	m.typed = false
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var err error
	onError := func(pos token.Position, msg string) {
		switch msg { // allow certain extra chars
		case `illegal character U+0024 '$'`:
		case `illegal character U+007E '~'`:
		default:
			err = fmt.Errorf("%v: %s", pos, msg)
		}
	}

	// we will modify the input source under the scanner's nose to
	// enable some features such as regexes.
	s.Init(file, src, onError, scanner.ScanComments)

	next := func() fullToken {
		pos, tok, lit := s.Scan()
		return fullToken{fset.Position(pos), tok, lit}
	}

	caseStat := caseNone

	var toks []fullToken
	for t := next(); t.tok != token.EOF; t = next() {
		switch t.lit {
		case "$": // continues below
		case "~":
			toks = append(toks, fullToken{t.pos, tokAggressive, ""})
			continue
		case "switch", "select", "case":
			if t.lit == "case" {
				caseStat = caseNone
			} else {
				caseStat = caseNeedBlock
			}
			fallthrough
		default: // regular Go code
			if t.tok == token.LBRACE && caseStat == caseNeedBlock {
				caseStat = caseHere
			}
			toks = append(toks, t)
			continue
		}
		wt, err := m.wildcard(t.pos, next, src)
		if err != nil {
			return nil, err
		}
		if caseStat == caseHere {
			toks = append(toks, fullToken{wt.pos, token.IDENT, "case"})
		}
		toks = append(toks, wt)
		if caseStat == caseHere {
			toks = append(toks, fullToken{wt.pos, token.COLON, ""})
			toks = append(toks, fullToken{wt.pos, token.IDENT, "gogrep_body"})
		}
	}
	return toks, err
}

func (m *matcher) wildcard(pos token.Position, next func() fullToken, src []byte) (fullToken, error) {
	wt := fullToken{pos, token.IDENT, wildPrefix}
	t := next()
	paren := false
	if paren = t.tok == token.LPAREN; paren {
		t = next()
	}
	var info varInfo
	if t.tok == token.MUL {
		t = next()
		info.any = true
	}
	if t.tok != token.IDENT {
		return wt, fmt.Errorf("%v: $ must be followed by ident, got %v",
			t.pos, t.tok)
	}
	id := len(m.vars)
	wt.lit += strconv.Itoa(id)
	info.name = t.lit
	if !paren {
		m.vars = append(m.vars, info)
		return wt, nil
	}
	t = next()
ops:
	for {
		switch t.tok {
		case token.EOF, token.SEMICOLON, token.RPAREN:
			break ops
		case token.QUO:
			start := t.pos.Offset + 1
			rxStr := string(src[start:])
			end := strings.Index(rxStr, "/")
			if end < 0 {
				return wt, fmt.Errorf("%v: expected / to terminate regex",
					t.pos)
			}
			rxStr = rxStr[:end]
			for i := start; i < start+end; i++ {
				src[i] = ' '
			}
			t = next() // skip opening /
			if t.tok != token.QUO {
				// skip any following token, as
				// go/scanner retains one char
				// for its next token.
				t = next()
			}
			t = next() // skip closing /
			if !strings.HasPrefix(rxStr, "^") {
				rxStr = "^" + rxStr
			}
			if !strings.HasSuffix(rxStr, "$") {
				rxStr = rxStr + "$"
			}
			rx, err := regexp.Compile(rxStr)
			if err != nil {
				return wt, fmt.Errorf("%v: %v", wt.pos, err)
			}
			info.nameRxs = append(info.nameRxs, rx)
			continue
		}

		op := t.lit
		if t = next(); t.tok != token.LPAREN {
			return wt, fmt.Errorf("%v: wanted (", wt.pos)
		}
		switch op {
		case "type", "asgn", "conv":
			t = next()
			start := t.pos.Offset
			for open := 1; open > 0; t = next() {
				switch t.tok {
				case token.LPAREN:
					open++
				case token.RPAREN:
					open--
				case token.EOF:
					return wt, fmt.Errorf("%v: expected ) to close (", wt.pos)
				}
			}
			end := t.pos.Offset - 1
			typeStr := strings.TrimSpace(string(src[start:end]))
			typeExpr, err := parser.ParseExpr(typeStr)
			if err != nil {
				return wt, err
			}
			info.types = append(info.types, typeCheck{
				op, typeExpr})
			continue
		case "comp", "addr":
			info.extras = append(info.extras, op)
		case "is":
			switch t = next(); t.lit {
			case "basic", "array", "slice", "struct", "interface",
				"pointer", "func", "map", "chan":
			default:
				return wt, fmt.Errorf("%v: unknown type: %q", wt.pos,
					t.lit)
			}
			info.underlying = t.lit
		default:
			return wt, fmt.Errorf("%v: unknown op %q", wt.pos, op)
		}
		if t = next(); t.tok != token.RPAREN {
			return wt, fmt.Errorf("%v: wanted )", wt.pos)
		}
		t = next()
	}
	if t.tok != token.RPAREN {
		return wt, fmt.Errorf("%v: expected ) to close $(",
			t.pos)
	}
	if info.needExpr() {
		m.typed = true
	}
	m.vars = append(m.vars, info)
	return wt, nil
}

// using a prefix is good enough for now
const wildPrefix = "gogrep_"

func isWildName(name string) bool {
	return strings.HasPrefix(name, wildPrefix)
}

func fromWildName(s string) int {
	if !isWildName(s) {
		return -1
	}
	n, err := strconv.Atoi(s[len(wildPrefix):])
	if err != nil {
		return -1
	}
	return n
}
