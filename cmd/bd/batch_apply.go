package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/httpapi"
	"github.com/steveyegge/beads/internal/storage/uow"
	"github.com/steveyegge/beads/issueops"
)

const nativeBatchApplyCapability = "issues.batchApply.cli.v1"

func newBatchApplyCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "apply --request <file|->",
		Short: "Apply a revision-guarded JSON batch atomically",
		Long: `Apply related issue changes together in the current workspace.

Read each issue with 'bd show --json' and copy its revision string into an
update's expected_version field. Optional status and assignee guards can
further restrict the update. A failed guard or normal close-policy check
rolls back the entire request. Workspace hooks apply to committed changes.

Supply a JSON object with actor and items members. The result is JSON with
exact revision strings. Use '-' to read the request from stdin. The separate
line-based 'bd batch' format continues to work as before.`,
		Example: `  bd show bd-42 --json
  bd batch apply --request batch.json --json
  bd batch apply --request - --json < batch.json`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			path, err := cmd.Flags().GetString("request")
			if err != nil {
				return err
			}
			if path == "" {
				return fmt.Errorf("--request is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			CheckReadonly("batch apply")
			path, _ := cmd.Flags().GetString("request")
			var reader io.Reader = cmd.InOrStdin()
			if path != "-" {
				file, err := os.Open(path) // #nosec G304 -- user-selected read-only JSON request; decoder bounds input to 4 MiB
				if err != nil {
					return HandleErrorRespectJSON("opening batch request: %v", err)
				}
				defer file.Close() //nolint:errcheck // read-only bounded request
				reader = file
			}
			result, err := runTypedBatchApply(cmd.Context(), reader, nativeBatchApplier)
			if err != nil {
				return typedBatchApplyError(err)
			}
			return outputJSON(httpapi.ApplyBatchResponse(result))
		},
	}
	command.Flags().String("request", "", "JSON request file, or '-' for stdin")
	return command
}

func runTypedBatchApply(ctx context.Context, reader io.Reader, resolve func() (issueops.BatchApplier, error)) (issueops.ApplyBatchResult, error) {
	request, err := httpapi.DecodeApplyBatchRequest(reader)
	if err != nil {
		return issueops.ApplyBatchResult{}, err
	}
	applier, err := resolve()
	if err != nil {
		return issueops.ApplyBatchResult{}, err
	}
	// Explicit native commit point, retaining the normal notifying provider.
	return applier.ApplyBatch(explicitCommitPointContext(ctx), request)
}

func nativeBatchApplier() (issueops.BatchApplier, error) {
	if usesProxiedServer() {
		return batchApplierFromProvider(uowProvider)
	}
	if store == nil {
		return nil, fmt.Errorf("native store is not initialized")
	}
	return store.BatchApplier()
}

func batchApplierFromProvider(provider uow.UnitOfWorkProvider) (issueops.BatchApplier, error) {
	source, ok := provider.(uow.BatchApplierSource)
	if !ok {
		return nil, fmt.Errorf("native provider does not support batch apply")
	}
	return source.BatchApplier()
}

func typedBatchApplyError(err error) error {
	if !jsonOutput {
		return HandleError("applying batch: %v", err)
	}
	code := "batch_apply_failed"
	switch {
	case errors.Is(err, issueops.ErrVersionMismatch):
		code = "version_mismatch"
	case errors.Is(err, issueops.ErrStatusMismatch):
		code = "status_mismatch"
	case errors.Is(err, issueops.ErrAssigneeMismatch):
		code = "assignee_mismatch"
	case errors.Is(err, issueops.ErrCloseBlocked):
		code = "close_blocked"
	case errors.Is(err, issueops.ErrValidation):
		code = "validation_failed"
	}
	return outputJSONError(err, code)
}

func init() { batchCmd.AddCommand(newBatchApplyCommand()) }
