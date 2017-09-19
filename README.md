# gogrep

[![Build Status](https://travis-ci.org/mvdan/gogrep.svg?branch=master)](https://travis-ci.org/mvdan/gogrep)

Rog and Dan's drunken idea. Work in progress.

	go get -u mvdan.cc/gogrep

Its first argument is a pattern to match. It can be any Go expression or
statement, which may include wildcards. Wildcards are identifiers
preceded by `$`.

	$ gogrep 'if $x != nil { return $x }'
	main.go:47:2: if err != nil { return err; }
	main.go:60:2: if err != nil { return err; }

All wildcards with the same name must match the same syntax node. In
other words, they must be equal in the source code. The `$_` wildcard
doesn't follow this rule, so it can be used to match anything regardless
of how often it is used.

You can also use a `*` before the name to match any number of
expressions or statements, such as:

	$ gogrep 'if err != nil { $*_ }'
	main.go:47:2: if err != nil { return err; }
	main.go:60:2: if err != nil { return err; }
	tokenize.go:42:3: if err != nil { return nil, err; }
