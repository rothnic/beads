package main

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/issueops"
)

const exactStoreReadJSONCapability = "issues.getExact.cli.json.v1"

// runShowExactStore stays on the selected provider's reader. Ordinary show may
// resolve a partial ID or route a missing ID to another store; neither is safe
// when its revision will guard a later write to this store. Reads are independent,
// but the output is complete or an error: no successful partial array is emitted.
func runShowExactStore(cmd *cobra.Command, ctx context.Context, args []string) error {
	if !jsonOutput {
		return HandleErrorRespectJSON("--exact-store requires --json")
	}
	for _, name := range []string{"current", "thread", "refs", "children", "watch", "as-of", "short", "long", "local-time", "comments-tail"} {
		if cmd.Flags().Changed(name) {
			return HandleErrorRespectJSON("--exact-store cannot be combined with --%s", name)
		}
	}
	idFlags, _ := cmd.Flags().GetStringArray("id")
	ids := append(append([]string(nil), args...), idFlags...)
	if len(ids) == 0 {
		return HandleErrorRespectJSON("--exact-store requires at least one explicit issue ID")
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return HandleErrorRespectJSON("--exact-store requires nonempty exact issue IDs")
		}
	}
	includeDependents, _ := cmd.Flags().GetBool("include-dependents")
	includeComments, _ := cmd.Flags().GetBool("include-comments")
	briefDeps, _ := cmd.Flags().GetBool("brief-deps")
	rd, err := openIssueReader()
	if err != nil {
		return HandleErrorRespectJSON("open selected-store reader: %v", err)
	}
	details := make([]*issueops.IssueDetails, 0, len(ids))
	for _, id := range ids {
		row, err := rd.Get(ctx, showGetRequest(id, includeDependents, includeComments, briefDeps))
		if err != nil {
			return HandleErrorRespectJSON("reading exact issue %q in selected store: %v", id, err)
		}
		details = append(details, row)
	}
	return outputJSON(details)
}
