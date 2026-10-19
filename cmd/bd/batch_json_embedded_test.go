//go:build cgo

package main

import (
	"os"
	"testing"
)

func TestEmbeddedBatchJSONGuardedPublication(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded JSON batch tests")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "jbc")
	testBatchJSONGuardedPublication(t, crossModeEnv{mode: "classic", bd: bd, dir: dir, env: bdEnv(dir)})
}
