# gogrep

[![Build Status](https://travis-ci.org/mvdan/gogrep.svg?branch=master)](https://travis-ci.org/mvdan/gogrep)

	go get -u mvdan.cc/gogrep

Search for Go code using syntax trees. Work in progress.

	gogrep 'if $x != nil { return $x, $*_ }'

### Instrucitons

	usage: gogrep commands [packages]

A command is of the form "-A pattern", where -A is one of:

       -x  find all nodes matching a pattern
       -g  discard nodes not matching a pattern
       -v  discard nodes matching a pattern

If -A is ommitted for a single command, -x will be assumed.

A pattern is a piece of Go code which may include wildcards. It can be:

       a statement (many if split by semicolonss)
       an expression (many if split by commas)
       a type expression
       a top-level declaration (var, func, const)
       an entire file

Wildcards consist of `$` and a name. All wildcards with the same name
within an expression must match the same node, excluding "_". Example:

       $x.$_ = $x // assignment of self to a field in self

If `*` is before the name, it will match any number of nodes. Example:

       fmt.Fprintf(os.Stdout, $*_) // all Fprintfs on stdout

Regexes can also be used to match certain identifier names only. The
`.*` pattern can be used to math identifiers only. Example:

       fmt.$(_ /Fprint.*/)(os.Stdout, $*_) // all Fprint* on stdout

The nodes resulting from applying the commands will be printed line by
line to standard output.
