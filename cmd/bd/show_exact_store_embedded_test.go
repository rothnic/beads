//go:build cgo

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestEmbeddedShowExactStoreRejectsRoutedForeignID(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded exact-store tests")
	}
	isolateBeadsDirForTest(t)
	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "xsl")
	foreignDir := filepath.Join(dir, "foreign")
	if err := os.MkdirAll(foreignDir, 0o750); err != nil {
		t.Fatal(err)
	}
	initGitRepoAt(t, foreignDir)
	runBDInit(t, bd, foreignDir, "--prefix", "xsf")
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(`{"prefix":"xsf-","path":"foreign"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	local := crossModeEnv{mode: "classic", bd: bd, dir: dir, env: bdEnv(dir)}
	foreign := crossModeEnv{mode: "foreign", bd: bd, dir: foreignDir, env: bdEnv(foreignDir)}
	localID := local.create(t, "local exact-store row")
	foreignID := foreign.create(t, "foreign row must stay foreign")
	before := foreign.mustRun(t, "show", "--json", "--", foreignID)

	// No-credit control: the foreign ID must really be reachable through the
	// existing ordinary-show route. A nonexistent or disabled route proves nothing.
	ordinary := local.mustRun(t, "show", "--json", "--", foreignID)
	if rows := exactStoreTestRows(t, ordinary); len(rows) != 1 || rows[0].ID != foreignID {
		t.Fatalf("ordinary route did not reach foreign store: %s", ordinary)
	}
	for _, envelope := range []string{"0", "1"} {
		refusalEnv := local
		refusalEnv.env = append(envWithout(local.env, "BD_JSON_ENVELOPE"), "BD_JSON_ENVELOPE="+envelope)
		for _, ids := range [][]string{{foreignID}, {localID, foreignID}} {
			args := append([]string{"show", "--exact-store", "--json", "--"}, ids...)
			stdout, stderr, code := refusalEnv.run(t, args...)
			if code == 0 {
				t.Fatalf("foreign exact read returned success/partial data: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			if message := jsonErrorMessage(t, stdout); !strings.Contains(message, foreignID) {
				t.Fatalf("foreign exact read returned the wrong error: envelope=%s stdout=%s stderr=%s", envelope, stdout, stderr)
			}
		}
	}
	if after := foreign.mustRun(t, "show", "--json", "--", foreignID); after != before {
		t.Fatal("read attempts changed the foreign issue")
	}
	testShowExactStoreContract(t, local)
}

func exactStoreTestRows(t *testing.T, body string) []types.IssueDetails {
	t.Helper()
	var rows []types.IssueDetails
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("exact-store detail array: %v: %s", err, body)
	}
	return rows
}

// Shared with the real proxied-provider leg; this does not substitute a reader
// fake for either transport. Revision and metadata are compared with the same
// stored row so the oracle does not assume revisions are monotonic.
func testShowExactStoreContract(t *testing.T, env crossModeEnv) {
	t.Helper()
	metadata := `{"large":9007199254740993123456789,"bool":true,"object":{"x":1},"array":[1,false],"null":null}`
	first := env.create(t, "exact raw metadata", "--metadata", metadata)
	second := env.create(t, "exact ordering")
	stored := exactStoreTestRows(t, env.mustRun(t, "show", "--json", "--", first))[0]
	rows := exactStoreTestRows(t, env.mustRun(t, "show", "--exact-store", "--json", "--", second, first, second))
	if len(rows) != 3 || rows[0].ID != second || rows[1].ID != first || rows[2].ID != second {
		t.Fatalf("[%s] input order/duplicates changed: %+v", env.mode, rows)
	}
	if rows[1].Revision == "" || rows[1].Revision != stored.Revision {
		t.Fatalf("[%s] native revision changed: %q vs %q", env.mode, rows[1].Revision, stored.Revision)
	}
	if _, err := types.ParseRevisionToken(rows[1].Revision); err != nil {
		t.Fatalf("[%s] invalid native revision: %v", env.mode, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rows[1].Metadata, &raw); err != nil {
		t.Fatal(err)
	}
	var storedRaw map[string]json.RawMessage
	if err := json.Unmarshal(stored.Metadata, &storedRaw); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"large": string(storedRaw["large"]), "bool": "true", "object": `{"x":1}`, "array": `[1,false]`, "null": "null"} {
		// Ignore backend JSON whitespace without converting any value through float64.
		if strings.Join(strings.Fields(string(raw[key])), "") != strings.Join(strings.Fields(want), "") {
			t.Fatalf("[%s] raw %s changed: %s vs %s", env.mode, key, raw[key], want)
		}
	}

	// Reader.Get's issue-to-wisp fallback remains inside this same provider.
	wisp := env.create(t, "exact wisp", "--ephemeral")
	wispRows := exactStoreTestRows(t, env.mustRun(t, "show", "--exact-store", "--json", "--", wisp))
	if len(wispRows) != 1 || wispRows[0].ID != wisp || wispRows[0].Revision == "" {
		t.Fatalf("[%s] exact same-store wisp unavailable: %+v", env.mode, wispRows)
	}

	// Preserve an existing ID containing a flag-looking segment, too.
	prefix, _, _ := strings.Cut(first, "-")
	literal := prefix + "--exact-store"
	if got := env.create(t, "literal exact ID", "--id", literal); got != literal {
		t.Fatalf("[%s] fixture did not retain literal ID: %q", env.mode, got)
	}
	literalRows := exactStoreTestRows(t, env.mustRun(t, "show", "--exact-store", "--json", "--id="+literal))
	if len(literalRows) != 1 || literalRows[0].ID != literal {
		t.Fatalf("[%s] existing literal ID changed: %+v", env.mode, literalRows)
	}
	for _, envelope := range []string{"0", "1"} {
		refusalEnv := env
		refusalEnv.env = append(envWithout(env.env, "BD_JSON_ENVELOPE"), "BD_JSON_ENVELOPE="+envelope)
		missing := first + "-missing"
		partialOut, partialErr, partialCode := refusalEnv.run(t, "show", "--exact-store", "--json", "--", second, missing)
		if partialCode == 0 {
			t.Fatalf("[%s] missing row emitted partial success: %d %s %s", env.mode, partialCode, partialOut, partialErr)
		}
		if message := jsonErrorMessage(t, partialOut); !strings.Contains(message, missing) {
			t.Fatalf("[%s] missing row returned the wrong error: %s %s", env.mode, partialOut, partialErr)
		}

		// An unambiguous prefix still fails; exact mode never resolves partial IDs.
		partial := first[:len(first)-1]
		stdout, stderr, code := refusalEnv.run(t, "show", "--exact-store", "--json", "--", partial)
		if code == 0 {
			t.Fatalf("[%s] partial ID accepted: %d %s %s", env.mode, code, stdout, stderr)
		}
		if message := jsonErrorMessage(t, stdout); !strings.Contains(message, partial) {
			t.Fatalf("[%s] partial ID returned the wrong error: %s %s", env.mode, stdout, stderr)
		}
		// A flag-looking ID reaches the native reader literally after --, and --id
		// supplies the same escape hatch. Neither may activate a rendering mode.
		for _, args := range [][]string{
			{"show", "--exact-store", "--json", "--", "--short"},
			{"show", "--exact-store", "--json", "--id=--short"},
		} {
			stdout, stderr, code = refusalEnv.run(t, args...)
			if code == 0 {
				t.Fatalf("[%s] hostile ID became a flag: %d %s %s", env.mode, code, stdout, stderr)
			}
			if message := jsonErrorMessage(t, stdout); !strings.Contains(message, `reading exact issue`) || !strings.Contains(message, "--short") || strings.Contains(message, "cannot be combined") {
				t.Fatalf("[%s] hostile ID returned the wrong error: %s %s", env.mode, stdout, stderr)
			}
		}
		// Real command validation is exercised too; the full incompatible-mode table
		// is a store-free unit test so each refusal does not open another database.
		stdout, stderr, code = refusalEnv.run(t, "show", "--exact-store", "--json", "--current")
		if code == 0 {
			t.Fatalf("[%s] incompatible current mode accepted: %d %s %s", env.mode, code, stdout, stderr)
		}
		if message := jsonErrorMessage(t, stdout); !strings.Contains(message, "--current") {
			t.Fatalf("[%s] incompatible current mode returned the wrong error: %s %s", env.mode, stdout, stderr)
		}
	}
}
