//go:build cgo

package main

import "testing"

func TestProxiedServerShowExactStoreParity(t *testing.T) {
	isolateBeadsDirForTest(t)
	requireSharedProxiedServer(t)
	bd := buildEmbeddedBD(t)
	for _, env := range newCrossModeEnvs(t, bd, "xrc", "xrp") {
		testShowExactStoreContract(t, env)
	}
}
