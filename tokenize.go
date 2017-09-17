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
	tokWildcard
)

type fullToken struct {
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
		default:
			err = fmt.Errorf("%v: %s", pos, msg)
		}
	}
	s.Init(file, []byte(src), onError, scanner.ScanComments)

	var toks []fullToken
	gotDollar := false
	for {
		pos, tok, lit := s.Scan()
		fpos := fset.Position(pos)
		if gotDollar {
			if tok != token.IDENT {
				err = fmt.Errorf("%v: $ must be followed by ident, got %v",
					fpos, tok)
				break
			}
			gotDollar = false
			toks = append(toks, fullToken{tokWildcard, lit})
			continue
		}
		if tok == token.EOF || err != nil {
			break
		}
		if tok == token.ILLEGAL && lit == "$" {
			gotDollar = true
		} else {
			toks = append(toks, fullToken{tok, lit})
		}
	}
	return toks, err
}
