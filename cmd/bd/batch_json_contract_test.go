package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	storageissueops "github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/storage/uow"
	"github.com/steveyegge/beads/issueops"
)

func TestBatchJSONRejectsAmbiguousMembers(t *testing.T) {
	previousContext, previousProvider := cmdCtx, uowProvider
	previousReadonly, previousProxied, previousJSON, previousActor := readonlyMode, proxiedServerMode, jsonOutput, actor
	t.Cleanup(func() {
		cmdCtx, uowProvider = previousContext, previousProvider
		readonlyMode, proxiedServerMode, jsonOutput, actor = previousReadonly, previousProxied, previousJSON, previousActor
	})
	isolateBeadsDirForTest(t)
	t.Chdir(t.TempDir())
	t.Setenv("BD_JSON_ENVELOPE", "0")
	cmdCtx, readonlyMode, proxiedServerMode, actor = nil, false, true, "fixture-actor"

	// Invoke the actual command handler with a recording provider, without opening a store.
	dispatch := func(t *testing.T, input string, refuse bool) (*batchJSONFixtureProvider, *batchJSONFixtureApplier, error, string) {
		t.Helper()
		applier := &batchJSONFixtureApplier{}
		provider := &batchJSONFixtureProvider{applier: applier}
		uowProvider, jsonOutput = provider, refuse
		cmd := &cobra.Command{Use: "batch"}
		cmd.Flags().String("input-format", "json", "")
		cmd.Flags().String("file", "", "")
		cmd.Flags().Bool("dry-run", false, "")
		cmd.Flags().String("message", "", "")
		cmd.SetIn(strings.NewReader(input))
		cmd.SetOut(&bytes.Buffer{})
		var err error
		stderr := captureStderr(t, func() { err = batchCmd.RunE(cmd, nil) })
		return provider, applier, err, stderr
	}

	const first = `{"kind":"create","create":{"id":"bd-first","title":"first"}}`
	const second = `{"kind":"create","create":{"id":"bd-second","title":"second"}}`
	for _, tc := range []struct{ name, input string }{
		{"schema_case_alias", `{"SCHEMA_VERSION":"1","request":{"items":[` + first + `]}}`},
		{"request_case_alias", `{"schema_version":"1","REQUEST":{"items":[` + first + `]}}`},
		{"duplicate_schema", `{"schema_version":"unsupported","schema_version":"1","request":{"items":[` + first + `]}}`},
		{"duplicate_request", `{"schema_version":"1","request":{"items":[` + first + `]},"request":{"items":[` + second + `]}}`},
		{"duplicate_items", `{"schema_version":"1","request":{"items":[` + first + `],"items":[` + second + `]}}`},
		{"escaped_duplicate_items", `{"schema_version":"1","request":{"it\u0065ms":[` + first + `],"items":[` + second + `]}}`},
		{"duplicate_nested_title", `{"schema_version":"1","request":{"items":[{"kind":"create","create":{"id":"bd-first","title":"discarded","title":"replacement"}}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseBatchJSON(strings.NewReader(tc.input), "fixture-actor"); !errors.Is(err, issueops.ErrValidation) {
				t.Errorf("ambiguous input needs typed validation refusal: %v", err)
			}
			provider, applier, err, stderr := dispatch(t, tc.input, true)
			var exit *exitError
			if !errors.As(err, &exit) || exit.Code != 1 {
				t.Errorf("command must exit 1: %v", err)
			}
			var reported struct {
				Code string `json:"code"`
			}
			if json.Unmarshal([]byte(stderr), &reported) != nil || reported.Code != "validation_failed" {
				t.Errorf("command needs typed validation JSON: %q", stderr)
			}
			if provider.called != 0 || applier.called != 0 {
				t.Errorf("ambiguous input reached writer: provider=%d applier=%d request=%+v", provider.called, applier.called, applier.request)
			}
		})
	}

	t.Run("valid_distinct_creates", func(t *testing.T) {
		input := `{"schema_version":"1","request":{"items":[` + first + `,` + second + `]}}`
		provider, applier, err, stderr := dispatch(t, input, false)
		if err != nil || stderr != "" || provider.called != 1 || applier.called != 1 || len(applier.request.Items) != 2 {
			t.Fatalf("valid control did not reach writer once: provider=%d applier=%d err=%v stderr=%q", provider.called, applier.called, err, stderr)
		}
		if applier.request.Items[0].Create.Issue.ID != "bd-first" || applier.request.Items[1].Create.Issue.ID != "bd-second" {
			t.Fatal("distinct creates were discarded or reordered")
		}
	})
	t.Run("valid_metadata_and_exact_revision", func(t *testing.T) {
		input := `{"schema_version":"1","request":{"items":[{"kind":"create","create":{"id":"bd-first","title":"first","metadata":{"Items":null,"items":[{"same":true},{"same":false}],"number":9007199254740993,"text":"line\n\"quote"}}},{"kind":"update","update":{"target":{"id":"bd-existing"},"expected_version":"9007199254740993","patch":{"title":"second"}}}]}}`
		request, err := parseBatchJSON(strings.NewReader(input), "fixture-actor")
		if err != nil || len(request.Items) != 2 {
			t.Fatalf("valid metadata or revision refused: %v", err)
		}
		if request.Actor != "fixture-actor" || request.Items[1].Update.ExpectedVersion == nil || *request.Items[1].Update.ExpectedVersion != 9007199254740993 {
			t.Fatal("actor or exact revision was changed")
		}
		var metadata map[string]json.RawMessage
		if err := json.Unmarshal(request.Items[0].Create.Issue.Metadata, &metadata); err != nil {
			t.Fatal(err)
		}
		if string(metadata["number"]) != "9007199254740993" || string(metadata["Items"]) != "null" || string(metadata["items"]) != `[{"same":true},{"same":false}]` || string(metadata["text"]) != `"line\n\"quote"` {
			t.Fatalf("metadata lost precision, legal case-sensitive keys, or values: %s", request.Items[0].Create.Issue.Metadata)
		}
	})
}

func TestBatchJSONRejectsLossyOrUnownedWireFields(t *testing.T) {
	for _, input := range []string{
		`{"schema_version":"1","request":{"items":[{"kind":"create","create":{"title":"x","metadata":` + strings.Repeat("[", 10001) + `0` + strings.Repeat("]", 10001) + `}}]}}`,
		`{"schema_version":"1","request":null}`,
		`{"schema_version":"1","request":{"Actor":"human","items":[]}}`,
		`{"schema_version":"1","request":{"Items":[]}}`,
		`{"schema_version":"1","request":{"items":[{"kind":"update","update":{"target":{"id":"bd-x"},"expected_version":9007199254740993,"patch":{"title":"x"}}}]}}`,
		`{"schema_version":"1","request":{"items":[{"kind":"create","create":{"title":"x","created_by":"someone"}}]}}`,
		`{"schema_version":"1","request":{"items":[{"kind":"create","create":{"title":"x","created_at":"2026-01-01T00:00:00Z"}}]}}`,
	} {
		if _, err := parseBatchJSON(strings.NewReader(input), "fixture-actor"); !errors.Is(err, issueops.ErrValidation) {
			t.Fatalf("invalid plan needs typed validation refusal: %s: %v", input, err)
		}
	}
	input := `{"schema_version":"1","request":` + strings.Repeat(" ", 8*1024*1024) + `{}}`
	if _, err := parseBatchJSON(strings.NewReader(input), "fixture"); !errors.Is(err, issueops.ErrValidation) {
		t.Fatalf("oversized envelope accepted: %v", err)
	}
}

type batchJSONFixtureApplier struct {
	request  issueops.ApplyBatchRequest
	deferred bool
	called   int
}

func (a *batchJSONFixtureApplier) ApplyBatch(ctx context.Context, request issueops.ApplyBatchRequest) (issueops.ApplyBatchResult, error) {
	a.request, a.deferred = request, storageissueops.VersionCommitDeferred(ctx)
	a.called++
	return issueops.ApplyBatchResult{}, nil
}

type batchJSONFixtureProvider struct {
	uow.UnitOfWorkProvider
	applier issueops.BatchApplier
	called  int
}

func (p *batchJSONFixtureProvider) BatchApplier() (issueops.BatchApplier, error) {
	p.called++
	return p.applier, nil
}

func TestBatchJSONRetainsProviderRoleAndExplicitCommitPoint(t *testing.T) {
	previous := uowProvider
	t.Cleanup(func() { uowProvider = previous })
	applier := &batchJSONFixtureApplier{}
	provider := &batchJSONFixtureProvider{applier: applier}
	uowProvider = provider
	_, err := runBatchJSON(storageissueops.WithDeferredVersionCommit(t.Context()), issueops.ApplyBatchRequest{Actor: "fixture"}, true)
	if err != nil || provider.called != 1 || applier.called != 1 || applier.deferred || applier.request.Actor != "fixture" {
		t.Fatalf("owning role or commit context lost: provider=%+v applier=%+v err=%v", provider, applier, err)
	}
	uowProvider = struct{ uow.UnitOfWorkProvider }{}
	if _, err := runBatchJSON(t.Context(), issueops.ApplyBatchRequest{}, true); err == nil {
		t.Fatal("provider lacking its native BatchApplier role accepted")
	}
}
