# gogrep

Rog and Dan's drunken idea. Work in progress.

	go get -u mvdan.cc/gogrep

Its first argument is a pattern to match. It can be any Go expression or
statement, which may include wildcards. Wildcards are identifiers
preceded by `$`.

	$ gogrep 'if $x != nil { return $x }'
	main.go:37:2: if err != nil { return err; }
	main.go:50:2: if err != nil { return err; }
