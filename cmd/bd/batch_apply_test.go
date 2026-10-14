package main

import (
	"context"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/uow"
	"github.com/steveyegge/beads/issueops"
)

func TestBatchApplyCommandRegistered(t *testing.T) {
	cmd, remaining, err := rootCmd.Find([]string{"batch", "apply"})
	if err != nil || cmd == nil || cmd.Name() != "apply" || cmd.Parent() != batchCmd || len(remaining) != 0 {
		t.Fatalf("typed batch command is missing: cmd=%v remaining=%v err=%v", cmd, remaining, err)
	}
}

type typedBatchApplyRecorder struct {
	request issueops.ApplyBatchRequest
	calls   int
}

func (r *typedBatchApplyRecorder) ApplyBatch(_ context.Context, request issueops.ApplyBatchRequest) (issueops.ApplyBatchResult, error) {
	r.calls++
	r.request = request
	return issueops.ApplyBatchResult{Items: []issueops.ItemResult{{Kind: issueops.ItemUpdate, IssueID: "bd-exact", Changed: true, RowVersion: 18}}}, nil
}

type typedBatchProvider struct {
	uow.UnitOfWorkProvider
	applier  issueops.BatchApplier
	accessed int
}

func (p *typedBatchProvider) BatchApplier() (issueops.BatchApplier, error) {
	p.accessed++
	return p.applier, nil
}

func TestTypedBatchApplyUsesOwnProviderAccessor(t *testing.T) {
	recorder := &typedBatchApplyRecorder{}
	provider := &typedBatchProvider{applier: recorder}
	reader := strings.NewReader(`{"actor":"worker","items":[{"kind":"update","update":{"target":{"id":"bd-exact"},"expected_version":"17","patch":{"metadata":{"set":{"nonce":"next"}}}}}]}`)
	result, err := runTypedBatchApply(context.Background(), reader, func() (issueops.BatchApplier, error) { return batchApplierFromProvider(provider) })
	if err != nil || provider.accessed != 1 || recorder.calls != 1 || len(result.Items) != 1 || *recorder.request.Items[0].Update.ExpectedVersion != 17 {
		t.Fatalf("native OWN capability lost: provider=%+v recorder=%+v result=%+v err=%v", provider, recorder, result, err)
	}
}

func TestTypedBatchApplyRefusesBeforeResolvingApplier(t *testing.T) {
	resolved := false
	_, err := runTypedBatchApply(context.Background(), strings.NewReader(`{"actor":"worker","items":[]}`), func() (issueops.BatchApplier, error) { resolved = true; return &typedBatchApplyRecorder{}, nil })
	if err == nil || resolved {
		t.Fatalf("invalid request reached backend: err=%v resolved=%v", err, resolved)
	}
}
