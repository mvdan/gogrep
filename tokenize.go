// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"go/scanner"
	"go/token"
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

func tokenize(src string) ([]fullToken, error) {
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
	s.Init(file, []byte(src), onError, scanner.ScanComments)

	var remaining []fullToken
	for {
		pos, tok, lit := s.Scan()
		if err != nil {
			return nil, err
		}
		remaining = append(remaining, fullToken{fset.Position(pos), tok, lit})
		if tok == token.EOF {
			// remaining has a trailing token.EOF
			break
		}
	}
	next := func() fullToken {
		t := remaining[0]
		remaining = remaining[1:]
		return t
	}

	var toks []fullToken
	for t := next(); t.tok != token.EOF; t = next() {
		switch t.lit {
		case "$": // continues below
		case "~":
			toks = append(toks, fullToken{t.pos, tokAggressive, ""})
			continue
		default: // regular Go code
			toks = append(toks, t)
			continue
		}
		wt := fullToken{t.pos, tokWild, ""}
		t = next()
		paren := false
		if paren = t.tok == token.LPAREN; paren {
			t = next()
		}
		if t.tok == token.MUL {
			wt.tok = tokWildAny
			t = next()
		}
		if t.tok != token.IDENT {
			err = fmt.Errorf("%v: $ must be followed by ident, got %v",
				t.pos, t.tok)
			break
		}
		wt.lit = t.lit
		if paren {
			if t = next(); t.tok != token.RPAREN {
				err = fmt.Errorf("%v: expected ) to close $(",
					t.pos)
				break
			}
		}
		toks = append(toks, wt)
	}
	return toks, err
}
