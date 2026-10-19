package httpapi

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/beads/issueops"
)

func TestBatchApplyNativeCodecPreservesExactWireValues(t *testing.T) {
	request, err := DecodeApplyBatchRequest(strings.NewReader(`{"actor":"fixture","items":[{"kind":"update","update":{"target":{"id":"bd-row"},"expected_version":"9007199254740993","patch":{"metadata":{"set":{"large":9007199254740993,"null":null,"text":"17","empty":"","array":[true,null],"object":{"n":17}}}}}}]}`))
	if err != nil || len(request.Items) != 1 {
		t.Fatalf("wire request refused: %+v, %v", request, err)
	}
	update := request.Items[0].Update
	if *update.ExpectedVersion != 9007199254740993 || string(update.Patch.Metadata.Set["large"]) != "9007199254740993" || string(update.Patch.Metadata.Set["null"]) != "null" || string(update.Patch.Metadata.Set["text"]) != `"17"` {
		t.Fatalf("exact revision or raw metadata changed: %+v", update)
	}
	raw, err := json.Marshal(ApplyBatchResponse(issueops.ApplyBatchResult{Items: []issueops.ItemResult{{Kind: issueops.ItemUpdate, IssueID: "bd-row", Changed: true, RowVersion: 9007199254740993}}}))
	if err != nil || !strings.Contains(string(raw), `"revision":"9007199254740993"`) || strings.Contains(string(raw), `"issue":`) {
		t.Fatalf("lean exact revision response lost: %s, %v", raw, err)
	}
}

func TestBatchApplyNativeCodecRetainsCurrentBoundsAndMembers(t *testing.T) {
	input := `{"actor":"fixture","force_id_prefix":true,"skip_per_edge_cycle_check":true,"items":[{"kind":"create","create":{"id":"other-row","key":"made","title":"x","description":"` + strings.Repeat("a", 8*1024*1024) + `","metadata_refs":{"link":{"key":"made"}}}},{"kind":"dep_add","dep_add":{"source":{"key":"made"},"target":{"id":"bd-other"},"type":"blocks","has_spawner":true,"thread_id":"thread"}}]}`
	request, err := DecodeApplyBatchRequest(strings.NewReader(input))
	if err != nil || !request.ForceIDPrefix || !request.SkipPerEdgeCycleCheck || request.Items[0].Create.MetadataRefs["link"].Key != "made" || !request.Items[1].DepAdd.HasSpawner || request.Items[1].DepAdd.ThreadID != "thread" {
		t.Fatalf("current large body or members regressed: %v", err)
	}
	if _, err := DecodeApplyBatchRequest(strings.NewReader(input + strings.Repeat(" ", 16*1024*1024))); !errors.Is(err, issueops.ErrValidation) {
		t.Fatalf("HTTP-native body bound not enforced: %v", err)
	}
}
