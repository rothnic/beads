package main

import (
	"strings"
	"testing"
)

func TestBatchJSONParsesPinnedCreateAndVersionGuard(t *testing.T) {
	request, err := parseBatchJSON(strings.NewReader(`{"schema_version":"1","request":{"items":[{"kind":"create","create":{"id":"bd-record","title":"cited record","issue_type":"task","status":"closed","description":"quotes \" and newlines\n","metadata":{"flag":true,"count":9007199254740993},"labels":["evidence"]}},{"kind":"update","update":{"target":{"id":"bd-state"},"expected_version":"9007199254740993","patch":{"metadata":{"set":{"epoch":"new"}}}}}]}}`), "fixture-actor")
	if err != nil || len(request.Items) != 2 {
		t.Fatalf("JSON plan: %+v, %v", request, err)
	}
	if request.Actor != "fixture-actor" || request.Items[0].Create.Issue.ID != "bd-record" || *request.Items[1].Update.ExpectedVersion != 9007199254740993 {
		t.Fatal("pinned identity or guard was discarded")
	}
}

func TestBatchJSONRejectsUnknownAndAmbiguousFieldsBeforeWrites(t *testing.T) {
	for _, input := range []string{
		`{"schema_version":"1","request":{"items":[{"kind":"create","create":null}]}}`,
		`{"schema_version":"1","request":{"items":[{"kind":"unknown"}]}}`,
		`{"schema_version":"1","request":{"items":[{"kind":"create","create":{"id":"bd-x","title":"x","row_version":42}}]}}`,
		`{"schema_version":"1","request":{"actor":"human","items":[]}}`,
		`{"schema_version":"1","request":{}} {}`,
	} {
		if _, err := parseBatchJSON(strings.NewReader(input), "fixture-actor"); err == nil {
			t.Fatalf("invalid plan accepted: %s", input)
		}
	}
}
