// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

const (
	_ token.Token = -iota
	tokWild
	tokWildAny
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
		op := t.lit
		if op == "" {
			op = t.tok.String()
		}
		switch op {
		case "/":
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
		case "type", "asgn", "conv":
			if t = next(); t.tok != token.LPAREN {
				return wt, fmt.Errorf("%v: wanted (", wt.pos)
			}
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
				return wt, fmt.Errorf("%v: could not parse expr %q: %v",
					wt.pos, typeStr, err)
			}
			info.types = append(info.types, typeCheck{
				op, typeExpr})
		case "comp", "addr":
			info.extras = append(info.extras, op)
			if t = next(); t.tok != token.LPAREN {
				return wt, fmt.Errorf("%v: wanted (", wt.pos)
			}
			if t = next(); t.tok != token.RPAREN {
				return wt, fmt.Errorf("%v: wanted )", wt.pos)
			}
			t = next()
		case "is":
			if t = next(); t.tok != token.LPAREN {
				return wt, fmt.Errorf("%v: wanted (", wt.pos)
			}
			switch t = next(); t.lit {
			case "basic", "array", "slice", "struct", "interface",
				"pointer", "func", "map", "chan":
			default:
				return wt, fmt.Errorf("%v: unknown type: %q", wt.pos,
					t.lit)
			}
			info.underlying = t.lit
			if t = next(); t.tok != token.RPAREN {
				return wt, fmt.Errorf("%v: wanted )", wt.pos)
			}
			t = next()
		default:
			break ops
		}
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
		panic(err)
	}
	return n
}
