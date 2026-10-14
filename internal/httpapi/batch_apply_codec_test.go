package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/beads/issueops"
)

func TestNativeBatchApplyCodecKeepsRawMetadataAndExactRevision(t *testing.T) {
	raw := `{"actor":"worker","items":[{"kind":"update","update":{"target":{"id":"bd-exact"},"expected_version":"-9223372036854775808","expected_status":"in_progress","expected_assignee":"worker","patch":{"metadata":{"set":{"large":9007199254740993123456789,"bool":true,"null":null,"object":{"x":1},"array":[1,false]}}}}}]}`
	request, err := DecodeApplyBatchRequest(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	item := request.Items[0].Update
	if request.Actor != "worker" || item.Target.ID != "bd-exact" || *item.ExpectedVersion != -9223372036854775808 || string(*item.ExpectedStatus) != "in_progress" || *item.ExpectedAssignee != "worker" {
		t.Fatalf("guards changed: %+v", request)
	}
	for key, want := range map[string]string{"large": "9007199254740993123456789", "bool": "true", "null": "null", "object": `{"x":1}`, "array": "[1,false]"} {
		if string(item.Patch.Metadata.Set[key]) != want {
			t.Fatalf("%s lost its JSON type/precision: %s", key, item.Patch.Metadata.Set[key])
		}
	}
	encoded, err := json.Marshal(ApplyBatchResponse(issueops.ApplyBatchResult{Items: []issueops.ItemResult{{Kind: issueops.ItemUpdate, IssueID: "bd-exact", Changed: true, RowVersion: -9223372036854775808}}}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"revision":"-9223372036854775808"`)) {
		t.Fatalf("numeric/rounded wire revision: %s", encoded)
	}
}

func TestNativeBatchApplyCodecRefusesInvalidWholeRequest(t *testing.T) {
	valid := `{"actor":"worker","items":[{"kind":"update","update":{"target":{"id":"bd-exact"},"expected_version":"17","patch":{"metadata":{"set":{"nonce":"next"}}}}}]}`
	for name, raw := range map[string]string{
		"trailing":           valid + `{}`,
		"unknown":            strings.Replace(valid, `"actor":"worker"`, `"actor":"worker","typo":true`, 1),
		"numeric-revision":   strings.Replace(valid, `"expected_version":"17"`, `"expected_version":17`, 1),
		"wrong-item-payload": strings.Replace(valid, `"kind":"update"`, `"kind":"close"`, 1),
		"null-actor":         strings.Replace(valid, `"actor":"worker"`, `"actor":null`, 1),
		"empty-items":        `{"actor":"worker","items":[]}`,
		"null-body":          `null`,
		"oversized":          strings.Repeat(" ", maxApplyBatchBodyBytes) + valid,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplyBatchRequest(strings.NewReader(raw)); !errors.Is(err, issueops.ErrValidation) {
				t.Fatalf("invalid request accepted/untyped: %v", err)
			}
		})
	}
}
