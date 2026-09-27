---
id: "03-managed-lifetime"
title: "Retained managed conversation lifecycle"
status: done
wave: 3
depends_on: ["01-exact-host-foundation"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-COORDINATION-002
acceptance_criteria:
  - AC-PLUGINS-COORDINATION-002.1
  - AC-PLUGINS-COORDINATION-002.2
  - AC-PLUGINS-COORDINATION-002.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 03: Retained managed conversation lifecycle

## Summary

Add opt-in managed conversation identity and retention through exact APIs. Keep the existing v1 lifetime unchanged.

## In scope

- Implement ensure/get/list/delete and explicit pause, revisioned launch configuration, host-owned retention, and read-only transcript access after uninstall.
- Wire disable/crash/upgrade/restart into normal lifecycle stop and reconciliation. Add idle-only profile/executor updates and explicit scratch execution.
- Protect pending input storage hooks for the next work order; no dispatcher is advertised until restricted runtime and durable input are ready.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Ensure is idempotent by installation/workspace/instance key and conflicting updates cannot change a running turn.
- Disable and upgrade preserve history; uninstall revokes execution and reinstall cannot acquire old resources.
- Existing v1 lifecycle regression tests pass unchanged.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/plugins ./internal/task/service -run 'TestManagedConversationLifetime|TestManagedConversationIdentity|TestAgentConversation' -count=1)
(cd apps/backend && go test ./pkg/pluginsdk/...)
```

Evidence to create:

- `apps/backend/internal/plugins/host_managed_conversations_test.go`: `TestManagedConversationLifetime`.
- `apps/backend/internal/task/service/managed_conversations_test.go`: `TestManagedConversationIdentity`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/plugins/host_agent_conversations.go`
- `apps/backend/internal/plugins/service_lifecycle.go`
- `apps/backend/internal/task/service/agent_conversations.go`
- `apps/backend/internal/task/repository/`
- `apps/backend/pkg/pluginsdk/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)

## Risks

The current disable/uninstall cleanup deletes v1 managed conversations. Select the new lifetime explicitly instead of weakening existing cleanup ownership.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Implemented the opt-in installation/workspace/instance-key lifetime, revisioned
idle-only launch updates, operation-identity recovery, pause, delete, and retained
transcripts. Disable closes Host admission, pauses retained conversations, and
stops active runs through the orchestrator; uninstall detaches transcripts after
revoking the installation. Legacy v1 cleanup remains separate.

Validation passed:

- `go test ./internal/plugins ./internal/task/service ./pkg/pluginsdk/...`
- `go test -race ./internal/plugins ./internal/task/service -run 'TestManagedConversationLifetime|TestManagedConversationIdentity|TestAgentConversation|TestManagedConversationPauseStopsActiveExecution|TestManagedConversationEnsureRepairsAcceptedOperation' -count=1`
- `go test ./internal/backendapp -run '^$'`
- `make -C apps/backend build`
- `golangci-lint run ./internal/task/service ./internal/plugins ./internal/backendapp --timeout=5m` (0 issues)
- Public-doc validation (62 tests; 47 published pages) and `git diff --check`

The multi-target build completed. Its expected macOS codesign notice reported
that `codesign`/`rcodesign` is unavailable in this environment; the Go binaries
were produced successfully.
