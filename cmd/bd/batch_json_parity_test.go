//go:build cgo

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestProxiedServerBatchJSONGuardedPublicationParity(t *testing.T) {
	requireSharedProxiedServer(t)
	bd := buildEmbeddedBD(t)
	for _, env := range newCrossModeEnvs(t, bd, "jbc", "jbp") {
		testBatchJSONGuardedPublication(t, env)
	}
}

func TestProxiedServerBatchJSONManagedGuardedPublication(t *testing.T) {
	requireProxiedServerEnv(t)
	bd := buildEmbeddedBD(t)
	project := bdProxiedInit(t, bd, "jbm")
	testBatchJSONGuardedPublication(t, crossModeEnv{mode: "managed-proxy", bd: bd, dir: project.dir, env: bdProxiedEnv(project.dir)})
}

func testBatchJSONGuardedPublication(t *testing.T, env crossModeEnv) {
	t.Helper()
	existing := env.create(t, "JSON batch state")
	prefix := existing
	for index, char := range existing {
		if char == '-' {
			prefix = existing[:index]
			break
		}
	}
	pinned := prefix + "-json-pinned"
	plan := fmt.Sprintf(`{"schema_version":"1","request":{"items":[{"kind":"create","create":{"id":%q,"title":"JSON evidence","issue_type":"task","status":"closed","description":"opaque 9007199254740993 \n evidence","metadata":{"bool":true,"number":17}}},{"kind":"update","update":{"target":{"id":%q},"expected_version":"42","patch":{"title":"changed"}}}]}}`, pinned, existing)
	stdout, stderr, code := env.runStdin(t, plan, "batch", "--input-format=json", "--json")
	if code != ExitGuardMismatch || stdout != "" || !strings.Contains(stderr, `"code": "version_mismatch"`) {
		t.Fatalf("[%s] stale guard accepted: %s %s", env.mode, stdout, stderr)
	}
	_, _, code = env.run(t, "show", pinned, "--json")
	if code == 0 {
		t.Fatalf("[%s] partial create survived failed guard", env.mode)
	}
	plan = fmt.Sprintf(`{"schema_version":"1","request":{"items":[{"kind":"create","create":{"id":%q,"title":"JSON evidence","issue_type":"task","status":"closed","description":"opaque 9007199254740993 \n evidence","metadata":{"bool":true,"number":17}}}]}}`, pinned)
	stdout, stderr, code = env.runStdin(t, plan, "batch", "--input-format=json", "--json")
	if code != 0 {
		t.Fatalf("[%s] publication: %d %s %s", env.mode, code, stdout, stderr)
	}
	row := env.show(t, pinned)
	if row.Status != types.StatusClosed {
		t.Fatalf("[%s] published open row", env.mode)
	}
	if row.Description != "opaque 9007199254740993 \n evidence" {
		t.Fatalf("[%s] opaque evidence changed: %q", env.mode, row.Description)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if string(metadata["bool"]) != "true" || string(metadata["number"]) != "17" {
		t.Fatalf("[%s] typed metadata changed: %s", env.mode, row.Metadata)
	}
	testBatchJSONRevisionRoundTrip(t, env, pinned)
}

func testBatchJSONRevisionRoundTrip(t *testing.T, env crossModeEnv, id string) {
	t.Helper()
	var rows []struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal([]byte(env.mustRun(t, "show", id, "--json")), &rows); err != nil || len(rows) != 1 || rows[0].Revision == "" {
		t.Fatalf("[%s] initial revision unavailable: %+v %v", env.mode, rows, err)
	}
	revision := rows[0].Revision
	for _, title := range []string{"first exact revision", "second exact revision"} {
		plan := fmt.Sprintf(`{"schema_version":"1","request":{"items":[{"kind":"update","update":{"target":{"id":%q},"expected_version":%q,"patch":{"title":%q}}}]}}`, id, revision, title)
		stdout, stderr, code := env.runStdin(t, plan, "batch", "--input-format=json", "--json")
		var result struct {
			Items []struct {
				Revision string `json:"revision"`
				Changed  bool   `json:"changed"`
			} `json:"items"`
		}
		if code != 0 || json.Unmarshal([]byte(stdout), &result) != nil || len(result.Items) != 1 || !result.Items[0].Changed || result.Items[0].Revision == "" {
			t.Fatalf("[%s] exact revision chaining failed: %d %s %s", env.mode, code, stdout, stderr)
		}
		if _, err := types.ParseRevisionToken(result.Items[0].Revision); err != nil {
			t.Fatalf("[%s] result revision is not an exact native token: %v", env.mode, err)
		}
		revision = result.Items[0].Revision
		if got := env.show(t, id); got.Title != title {
			t.Fatalf("[%s] matching revision did not write title: %q", env.mode, got.Title)
		}
	}
	before := env.show(t, id)
	lateID := id + "-late"
	plan := fmt.Sprintf(`{"schema_version":"1","request":{"items":[{"kind":"create","create":{"id":%q,"title":"must roll back","issue_type":"task"}},{"kind":"update","update":{"target":{"id":%q},"expected_version":%q,"expected_status":"open","patch":{"title":"must not land"}}}]}}`, lateID, id, revision)
	stdout, stderr, code := env.runStdin(t, plan, "batch", "--input-format=json", "--json")
	if code != ExitGuardMismatch || stdout != "" || !strings.Contains(stderr, `"code": "status_mismatch"`) {
		t.Fatalf("[%s] late guard failed to refuse whole plan: %d %s %s", env.mode, code, stdout, stderr)
	}
	if _, _, code := env.run(t, "show", lateID, "--json"); code == 0 {
		t.Fatalf("[%s] early create survived a late native guard", env.mode)
	}
	if after := env.show(t, id); after.Title != before.Title || string(after.Metadata) != string(before.Metadata) {
		t.Fatalf("[%s] failed plan changed the existing row", env.mode)
	}
}
