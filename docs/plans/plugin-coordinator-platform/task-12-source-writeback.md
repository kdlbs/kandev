---
id: "12-source-writeback"
title: "Linked Jira and Linear issue writeback"
status: complete
wave: 12
depends_on: ["01-exact-host-foundation", "06-workspace-observations", "07-task-commands"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-008
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-008.1
  - AC-PLUGINS-MANAGED-COORDINATION-008.2
  - AC-PLUGINS-MANAGED-COORDINATION-008.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 12: Linked Jira and Linear issue writeback

## Summary

Add optional linked-issue comments and transitions using native integration credentials. Retain uncertain outcomes for reconciliation.

## In scope

- Implement capability/readback/comment/transition adapters that resolve the exact task source link within its workspace.
- Persist prepared intent and provider result; use provider idempotency or identity lookup where available and expose uncertain outcomes otherwise.
- Cover stale source links, missing credentials, rate limits, post-send timeout, retry, and both providers using recorded/fake provider responses.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- A plugin can comment or transition an existing linked Jira/Linear issue without receiving credentials.
- A timeout after provider acceptance cannot cause an automatic duplicate write.
- Receipts identify the source subject and actor, and distinguish unsupported, denied, limited, and uncertain outcomes.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/plugins ./internal/jira ./internal/linear -run 'TestSourceWritebackReceipts|TestPluginWriteback' -count=1)
```

Evidence to create:

- `apps/backend/internal/plugins/host_source_writeback_test.go`: `TestSourceWritebackReceipts`.
- `apps/backend/internal/jira/service_plugin_writeback_test.go`: `TestPluginWriteback`.
- `apps/backend/internal/linear/service_plugin_writeback_test.go`: `TestPluginWriteback`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/plugins/`
- `apps/backend/internal/jira/service.go`
- `apps/backend/internal/linear/service.go`
- `apps/backend/pkg/pluginsdk/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 06](task-06-workspace-observations.md)
- [Task 07](task-07-task-commands.md)

## Risks

Providers do not all offer exactly-once comments. Unknown outcomes remain operator-visible until resolved; never mask them as an effect-free retryable error.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

WO12 is complete. Added exact Jira and Linear linked-issue capability reads,
comments, and transitions using workspace-owned credentials. The Host persists
actor/source identity, version fences, payload digest, provider result, and
durable receipts. A send reservation makes ambiguous outcomes `UNCERTAIN`; replay
returns the saved receipt without another provider request. Added provider and
Host gRPC adapters, SDK status mapping, and updated manifest, authoring, and
gRPC contract docs.

Validation passed:

- Required race command:
  `TMPDIR=/root/.cache/kandev-tmp go test -race ./internal/plugins ./internal/jira ./internal/linear -run 'TestSourceWritebackReceipts|TestPluginWriteback' -count=1`.
- `go test ./internal/plugins -count=1` and
  `go test ./internal/backendapp -run '^TestPluginSourceIssueController' -count=1`.
- `go test -list` confirmed `TestSourceWritebackReceipts` and both
  `TestPluginWriteback` tests are present.
- The Host/SDK exact writeback gRPC round-trip, durable store receipt tests, and
  `make proto` passed during the implementation checks.
- Public docs validation passed: 62 validator tests and 47 published pages.
- `git diff --check` and a trailing-whitespace scan of the new writeback files
  passed.

The focused package lint command still reports existing complexity and string
findings in adjacent Exact Host and workspace-admin code. It reports no findings
in the Jira/Linear source adapter or writeback receipt implementation.
