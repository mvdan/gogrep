// +build gogrep

package builtin

import "mvdan.cc/gogrep/nls"

func excludeParallel(g *nls.G) {
	// g.With(nls.All(`$*body`))
	// g.With(nls.All(`$*body`), nls.All(`{ $*_; }`), nls.Replace(`{}`))
	g.Excluding(`t.Parallel()`)
}

func NonParallelTests(g *nls.G) {
	// TODO: include the case where a sub-test calls Parallel, but the
	// parent test does not.
	g.All(`func $f(t *testing.T) { $*body }`)
	excludeParallel(g)
	g.Report("$f is not parallel")
}

func NonParallelSubtests(g *nls.G) {
	// TODO: include the case where a sub-test calls Parallel, but the
	// parent test does not.
	g.All(`t.Run($n, func(t *testing.T) { $*body })`)
	excludeParallel(g)
	g.Report("subtest $n is not parallel")
}
