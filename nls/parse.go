// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package nls

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"strconv"
	"strings"

	"mvdan.cc/gogrep/gsyntax"
)

func (g *G) transformSource(expr string) (string, []gsyntax.PosOffset, error) {
	toks, err := g.tokenize([]byte(expr))
	if err != nil {
		return "", nil, fmt.Errorf("cannot tokenize expr: %v", err)
	}
	var offs []gsyntax.PosOffset
	lbuf := lineColBuffer{line: 1, col: 1}
	addOffset := func(length int) {
		lbuf.offs -= length
		offs = append(offs, gsyntax.PosOffset{
			AtLine: lbuf.line,
			AtCol:  lbuf.col,
			Offset: length,
		})
	}
	if len(toks) > 0 && toks[0].tok == tokAggressive {
		toks = toks[1:]
		g.aggressive = true
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
	return strings.TrimSpace(lbuf.String()), offs, nil
}

func (g *G) parseExpr(expr string) (ast.Node, error) {
	exprStr, offs, err := g.transformSource(expr)
	if err != nil {
		return nil, err
	}
	node, _, err := gsyntax.ParseAny(g.Fset, exprStr)
	if err != nil {
		err = gsyntax.SubPosOffsets(err, offs...)
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

func (g *G) tokenize(src []byte) ([]fullToken, error) {
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
		wt, err := g.wildcard(t.pos, next)
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

func (g *G) wildcard(pos token.Position, next func() fullToken) (fullToken, error) {
	wt := fullToken{pos, token.IDENT, wildPrefix}
	t := next()
	var info varInfo
	if t.tok == token.MUL {
		t = next()
		info.any = true
	}
	if t.tok != token.IDENT {
		return wt, fmt.Errorf("%v: $ must be followed by ident, got %v",
			t.pos, t.tok)
	}
	id := len(g.vars)
	wt.lit += strconv.Itoa(id)
	info.name = t.lit
	g.vars = append(g.vars, info)
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
