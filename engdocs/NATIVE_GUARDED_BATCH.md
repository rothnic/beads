# Guarded JSON batches

Use `bd batch apply` when related issue edits must commit together, and only
while the issues still match the state you read. The command uses the current
workspace and its normal close policy and mutation hooks.

This source-build interface is available when `bd version --json` advertises
`issues.batchApply.cli.v1` in its `capabilities` array. The published user
documentation follows its [release pin](../docs/cli-docs.pin).

## Prepare a request

Read the issues and copy their `revision` strings into the request. Treat
these tokens as opaque strings; do not convert them through JSON numbers.

```bash
bd version --json
bd show bd-42 bd-43 --json
```

Save the following as `batch.json`, replacing the IDs, revisions and expected
status with the values you read:

```json
{
  "actor": "automation",
  "items": [
    {
      "kind": "update",
      "update": {
        "target": {"id": "bd-42"},
        "expected_version": "17",
        "expected_status": "in_progress",
        "patch": {"metadata": {"set": {"result.accepted": true}}}
      }
    },
    {
      "kind": "update",
      "update": {
        "target": {"id": "bd-43"},
        "expected_version": "8",
        "patch": {"metadata": {"set": {"input.issue": "bd-42"}}}
      }
    }
  ]
}
```

Apply the file, or pass the same object through stdin:

```bash
bd batch apply --request batch.json --json
bd batch apply --request - --json < batch.json
```

## Behavior and results

- **Guards:** revision, status and assignee checks run within the transaction.
  A failed check rolls back every item, including earlier edits.
- **Metadata:** `set` changes named keys and `unset` removes named keys.
  Untouched values retain their JSON types, including objects and arrays.
- **Closing:** a `close` item uses the normal close policy. A blocked close
  refuses the whole batch unless the request explicitly uses the supported
  force option. Integrations that rely on ordinary admission must omit it.
- **Results:** the JSON response contains `keys` and an `items` array in
  request order. Each item reports its operation, issue ID, whether it
  changed, and its revision as a string. Normal JSON envelope settings apply.

On a guard refusal, read the issues again and decide whether to construct a
new request. If a process or connection fails without a usable response,
the commit outcome is unknown: inspect current state before attempting
another write.

The request format is shared with the
[batch API schema](../internal/httpapi/spec/openapi.v0.yaml). The existing
line-based `bd batch` script format remains a separate interface.
