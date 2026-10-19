# JSON batch input

Use `bd batch --input-format=json --file request.json --json` to submit related
issue changes in one native transaction. Omit `--file` to read stdin. The
default input format remains the line grammar shown by `bd batch --help`.

## Request

The envelope contains `schema_version: "1"` and a `request` object. The request
uses the same lowercase fields as the
[batchApply request schema](../internal/httpapi/spec/openapi.v0.yaml#L10078).
For example:

```json
{
  "schema_version": "1",
  "request": {
    "provenance": "publish a related record",
    "items": [
      {
        "kind": "create",
        "create": {
          "id": "bd-evidence",
          "title": "Retained evidence",
          "issue_type": "task",
          "status": "closed",
          "description": "Opaque source body"
        }
      }
    ]
  }
}
```

- **Actor:** Omit `request.actor`. The CLI supplies its configured actor as
  audit provenance; an actor string is not authenticated identity.
- **Commit message:** Use `request.provenance`. JSON mode refuses `--message`
  and `--dry-run`.
- **Bounds:** Supply one JSON object of at most 8 MiB and 1,000 items. Unknown
  wire fields and unsupported schema versions are refused before applying it.
- **JSON members:** Use exact lowercase envelope and wire field names. Duplicate
  keys in any object, including metadata, are refused before writing.
- **Metadata:** Native JSON storage semantics still apply, including
  [numeric normalization](../issueops/metadatacas.go#L50). Use strings for
  coordination tokens.

## Revision guards

Copy an issue's `revision` string from `bd show --exact-store --json -- <id>` into
`update.expected_version` or `close.expected_version`. Keep the string intact:
converting it through a floating-point number can lose digits. Optional update
guards also accept `expected_status` and `expected_assignee`.

The result contains `keys` and ordered `items`, including `issue_id`, `changed`
and an exact `revision` string. A caller can use that returned string as the
next request's guard. It does not include hydrated issue bodies.

- **Mismatch:** The entire batch rolls back, including earlier items, and the
  command exits 13. With `--json`, stderr carries `version_mismatch`,
  `status_mismatch` or `assignee_mismatch` in its `code` field.
- **Other batch refusals:** The batch handler exits 1 with a typed JSON error
  on stderr when `--json` is set. Normal CLI startup checks still apply.
- **Coverage:** A revision is the native equality token. It is not a complete
  content version, timestamp or Git commit; consumers must validate any
  additional state their own operation requires.

## Execution and compatibility

Direct and proxied workspaces use their normal `BatchApplier` role. Committed
changes retain native completion hooks and batch commit behavior. Close items
use the selected backend's native close policy; the direct-store external
blocker gap is tracked separately in
[issue #7290](https://github.com/gastownhall/beads/issues/7290) and
[PR #7394](https://github.com/gastownhall/beads/pull/7394).

Check `bd version --json` for `issues.batchApply.cli.json.v1` before using this
entry point. That token identifies this command grammar and wire contract;
HTTP capability tokens describe a different entry point. A client requiring
this contract must refuse an older binary rather than retry individual writes.

This CLI follows [PR #7169](https://github.com/gastownhall/beads/pull/7169)'s JSON
batch entry point. The parity tests are registered in the embedded and proxied
Bazel lanes. The published generated command reference remains pinned to the
latest released tag until the next release-pin update.

## Exact selected-store reads

`bd show --exact-store --json -- <id>...` reads exact IDs through the selected
store's existing `issueops.Reader`, including its wisp fallback. It does not
resolve prefixes, follow issue routes or use contributor routing to find a
missing row. The same mode uses the proxied provider's reader in a proxied
workspace. Workspace selection still follows the normal CLI startup rules.

The result is a detail array in input order, retaining duplicate IDs, raw
metadata JSON values and decimal `revision` strings. These are independent
reads, not a multi-row transaction snapshot. Any missing row or read failure
fails the command without a partial success array. This mode does not update
the last-touched issue.

Require `issues.getExact.cli.json.v1` in the store-free `bd version --json`
capability list before using this mode. JSON is required; explicit IDs (or
`--id`) are required. Current-issue, thread, references, children, watch,
historical and text-rendering modes cannot be combined with it. Ordinary
`bd show` keeps its existing routing and output behavior.
