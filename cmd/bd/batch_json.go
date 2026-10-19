package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/steveyegge/beads/internal/httpapi"
	"github.com/steveyegge/beads/internal/storage/uow"
	"github.com/steveyegge/beads/issueops"
)

// JSON input projects directly onto the existing guarded BatchApplier role.
// It adds a CLI transport, not another interpretation of batch semantics.
const nativeBatchJSONCapability = "issues.batchApply.cli.json.v1"

func parseBatchJSON(reader io.Reader, actorName string) (issueops.ApplyBatchRequest, error) {
	refuse := func(detail string) (issueops.ApplyBatchRequest, error) {
		return issueops.ApplyBatchRequest{}, fmt.Errorf("%s: %w", detail, issueops.ErrValidation)
	}
	const maxBytes = 8 * 1024 * 1024
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return issueops.ApplyBatchRequest{}, err
	}
	if len(raw) > maxBytes {
		return refuse(fmt.Sprintf("JSON batch exceeds %d bytes", maxBytes))
	}
	var plan map[string]json.RawMessage
	if err := json.Unmarshal(raw, &plan); err != nil || plan == nil {
		return refuse("JSON batch requires one object")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := checkBatchJSONMembers(decoder); err != nil {
		return refuse(err.Error())
	}
	if _, err := decoder.Token(); err != io.EOF {
		return refuse("JSON batch requires one object")
	}
	for name := range plan {
		if name != "schema_version" && name != "request" {
			return refuse(fmt.Sprintf("unknown JSON batch field %q", name))
		}
	}
	var schema string
	if err := json.Unmarshal(plan["schema_version"], &schema); err != nil || schema != "1" {
		return refuse("unsupported JSON batch schema")
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(plan["request"], &members); err != nil || members == nil {
		return refuse("JSON batch request must be an object")
	}
	if _, present := members["actor"]; present {
		return refuse("batch actor comes from the CLI actor, not the JSON request")
	}
	actor, err := json.Marshal(actorName)
	if err != nil {
		return issueops.ApplyBatchRequest{}, err
	}
	members["actor"] = actor
	wire, err := json.Marshal(members)
	if err != nil {
		return issueops.ApplyBatchRequest{}, err
	}
	return httpapi.DecodeApplyBatchRequest(bytes.NewReader(wire))
}

// Check original object names before using the decoded envelope fields.
// Token values are inspected only; the request keeps its original raw JSON.
func checkBatchJSONMembers(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := make(map[string]bool)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("JSON batch object member must be a string")
			}
			if seen[name] {
				return fmt.Errorf("duplicate JSON member %q", name)
			}
			seen[name] = true
			if err := checkBatchJSONMembers(decoder); err != nil {
				return err
			}
		}
	case json.Delim('['):
		for decoder.More() {
			if err := checkBatchJSONMembers(decoder); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	_, err = decoder.Token()
	return err
}

func runBatchJSON(ctx context.Context, request issueops.ApplyBatchRequest, proxied bool) (issueops.ApplyBatchResult, error) {
	var applier issueops.BatchApplier
	var err error
	if proxied {
		source, ok := uowProvider.(uow.BatchApplierSource)
		if !ok {
			return issueops.ApplyBatchResult{}, fmt.Errorf("native provider does not support guarded JSON batches")
		}
		applier, err = source.BatchApplier()
	} else {
		if store == nil {
			return issueops.ApplyBatchResult{}, fmt.Errorf("provider does not support guarded JSON batches")
		}
		applier, err = store.BatchApplier()
	}
	if err != nil {
		return issueops.ApplyBatchResult{}, err
	}
	return applier.ApplyBatch(explicitCommitPointContext(ctx), request)
}

func batchJSONError(err error) error {
	code := "batch_apply_failed"
	guard := false
	switch {
	case errors.Is(err, issueops.ErrVersionMismatch):
		code, guard = "version_mismatch", true
	case errors.Is(err, issueops.ErrStatusMismatch):
		code, guard = "status_mismatch", true
	case errors.Is(err, issueops.ErrAssigneeMismatch):
		code, guard = "assignee_mismatch", true
	case errors.Is(err, issueops.ErrCloseBlocked):
		code = "close_blocked"
	case errors.Is(err, issueops.ErrValidation):
		code = "validation_failed"
	}
	var reported error
	if jsonOutput {
		reported = outputJSONError(err, code)
	} else {
		reported = HandleError("applying JSON batch: %v", err)
	}
	if guard {
		return &exitError{Code: ExitGuardMismatch}
	}
	return reported
}
