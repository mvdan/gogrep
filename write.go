// Copyright (c) 2018, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"go/ast"
	"go/printer"
	"os"
)

func (m *matcher) cmdWrite(cmd exprCmd, subs []submatch) []submatch {
	seenRoot := make(map[nodePosHash]bool)
	filePaths := make(map[*ast.File]string)
	var next []submatch
	for _, sub := range subs {
		root := m.nodeRoot(sub.node)
		hash := posHash(root)
		if seenRoot[hash] {
			continue // avoid dups
		}
		seenRoot[hash] = true
		file, ok := root.(*ast.File)
		if ok {
			path := m.loader.fset.Position(file.Package).Filename
			if path != "" {
				// write to disk
				filePaths[file] = path
				continue
			}
		}
		// pass it on, to print to stdout
		next = append(next, submatch{node: root})
	}
	for file, path := range filePaths {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			// TODO: return errors instead
			panic(err)
		}
		if err := printConfig.Fprint(f, m.loader.fset, file); err != nil {
			// TODO: return errors instead
			panic(err)
		}
	}
	return next
}

var printConfig = printer.Config{
	Mode:     printer.UseSpaces | printer.TabIndent,
	Tabwidth: 8,
}

func (m *matcher) nodeRoot(node ast.Node) ast.Node {
	list, ok := node.(nodeList)
	if ok {
		node = list.at(0)
	}
	parent := m.parents[node]
	if parent == nil {
		return node
	}
	if _, ok := parent.(nodeList); ok {
		return parent
	}
	return m.nodeRoot(parent)
}
