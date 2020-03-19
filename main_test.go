// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(g *testing.M) {
	os.Exit(testscript.RunMain(g, map[string]func() int{
		"gogrep": main1,
	}))
}

var update = flag.Bool("u", false, "update testscript output files")

func TestScripts(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "scripts"),
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "MOD_DIR="+wd)

			// GitHub Actions doesn't define %LocalAppData% on
			// Windows, which breaks $GOCACHE. Set it ourselves.
			if runtime.GOOS == "windows" {
				env.Vars = append(env.Vars, fmt.Sprintf(`LOCALAPPDATA=%s\appdata`, env.WorkDir))
			}

			for _, name := range [...]string{
				"HOME",
				"USERPROFILE", // $HOME for windows
				"GOCACHE",
			} {
				if value := os.Getenv(name); value != "" {
					env.Vars = append(env.Vars, name+"="+value)
				}
			}
			return nil
		},
		UpdateScripts: *update,
	})
}
