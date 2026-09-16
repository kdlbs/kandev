---
created: 2026-09-16
status: draft
requirements:
  - REQ-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001
  - REQ-UI-EMPTY-TURN-NOTICE-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/integrations/system-design/mcp-tool-schema-portability.md
  - ../../specs/agents/system-design/session-recovery-failures.md
legacy_specs: []
---

# Implementation plan: Portable MCP tool schemas

## Overview

Restore Auggie sessions in GitHub- and GitLab-backed tasks and prevent the class
of regression. PR #3671 added `manage_task_change_request_kandev` (top-level
`oneOf`) and `update_task_change_request_automation_kandev` (top-level `allOf`).
The Augment API rejects any tool definition with a root `oneOf`/`allOf`/`anyOf`,
so every `session/prompt` returns `400 Bad Request` and no turn runs. Kandev's
own compile and argument validation pass, so nothing caught it before release.

This package covers both preventing the failure and surfacing it well when a
recoverable failure of this class occurs. Task 01 rewrites the two schemas to the
portable subset and moves their cross-field constraints into the handlers,
preserving the exact rejection behavior. Task 02 enforces the portable subset at
the shared compile seam so a root combinator fails closed at registration and a
built-in regression fails the registration test. Task 03 removes the duplicate
"finished without producing any output" notice that this exact failure produced,
by attaching the recovery entry to the turn that failed rather than opening a
second synthetic turn. Task 04 surfaces the sanitized provider detail (for
example `Tool schema does not support oneOf, allOf, or anyOf at the top level`)
in the existing collapsed technical-details disclosure, which was empty for
post-start recoverable failures.

## Scope

### In scope

- Rewrite `taskChangeRequestToolSchema` and `taskChangeRequestAutomationToolSchema`
  to drop root `oneOf`/`allOf` and the target `oneOf`.
- Enforce the surviving operation/target/prompt-exclusivity constraints in the
  change-request handlers before any backend side effect.
- Reject a root `oneOf`/`allOf`/`anyOf` in `toolschema.Compile` and
  `compileToolArgumentSchema`, covering plugin and built-in tools.
- Update pinning tests that assert the old schema shape; extend the
  all-modes registration test to prove no root combinator survives.
- Suppress the duplicate empty-turn notice on a turn that ends in a recoverable
  agent failure, and stop the failure from opening a second turn whose only
  content is the recovery status message.
- Populate the recovery entry's `error_output` metadata with the sanitized
  failure detail for post-start recoverable failures, so the existing collapsed
  technical-details disclosure shows the provider error.

### Out of scope

- Provider store, automation algorithm, transport, authorization, or backend
  action payload changes.
- Per-MCP-version or per-provider schema variants (rejected in the ADR).
- Rewriting nested combinators such as `show_rich_output_kandev.blocks.items`.
- Any frontend change: the empty-turn notice and the technical-details
  disclosure are already wired; Tasks 03 and 04 are backend-only.
- New recovery actions, remediation links, or changes to what counts as agent
  output for the empty-turn notice beyond the error-terminated turn case.

## Technical approach

Use the [system design](../../specs/integrations/system-design/mcp-tool-schema-portability.md)
for the exact enforcement seam and the handler constraints that replace each
dropped combinator. Keep every portable keyword the two schemas already use
(`additionalProperties: false`, enums, `minLength`, `minimum`, `minItems`,
`uniqueItems`, `minProperties`, root `required`). The handlers already validate
arguments and own the schema/backend agreement contract, so the moved checks live
next to the existing validation and run before any mutation.

Task 02 places the rule in the shared compiler so plugins and built-ins share one
enforcement point. `TestAllRegisteredToolSchemasCompile` then fails on a built-in
regression, and `validatePluginToolSnapshot` rejects a violating plugin tool.

Tasks 03 and 04 are the failure-surfacing half of the same incident and are
backend-only. Task 03 changes `handleRecoverableFailureLockedState`
(`internal/orchestrator/event_handlers_agent.go`) to attach the recovery status
message to the turn that failed instead of completing that turn first and letting
`getActiveTurnID` lazily open a second turn to hold the message; the failed turn
then reports `had_output=true` because its recovery/error entry is the turn's
outcome, per [`REQ-UI-EMPTY-TURN-NOTICE-001`](../../specs/ui/requirements/empty-turn-notice.md).
Task 04 populates `meta["error_output"]` in `createRecoveryStatusMessage` with
`routingerr.Sanitize(data.FailureDetails)` for post-start recoverable failures,
which the existing `ActionMessageDetails` disclosure renders, per the
[session-recovery-failures design](../../specs/agents/system-design/session-recovery-failures.md).

Sequence: Task 01 and Task 02 both touch schema code but different seams. Do Task
01 first so the built-in schemas are already portable when Task 02 turns the rule
into a hard failure; otherwise Task 02's registration test fails on the two
in-flight schemas. Task 02 depends on Task 01. Tasks 03 and 04 touch the
orchestrator recovery path only and are independent of Tasks 01 and 02 and of
each other; they can land in any order.

## Risks

- Handler validation must match the dropped schema constraints exactly, including
  fail-before-mutation and non-echo of argument values. Mitigate with tests that
  mirror each old schema branch.
- Other built-in schemas may already use a root combinator. Task 02's test run
  reveals any additional offender; rewrite it under the same rule before merge.
- Task 03 changes turn settlement ordering on the recoverable-failure path. It
  must keep the existing completion/cancellation guards intact and must not
  regress the clean empty-turn notice (an unrecognized `/command` still produces
  exactly one notice). Cover with orchestrator tests over the failure path and
  the existing empty-turn scenarios.
- Task 04 must not widen what is exposed: `error_output` stays sanitized and is
  omitted when empty, so raw stderr, URLs, and identifiers never reach durable
  metadata.

## Verification strategy

Each work order runs its backend package tests under TDD. The registration test
in Task 02 provides the repo-wide guarantee that no advertised built-in schema
carries a root combinator across every mode. Tasks 03 and 04 add orchestrator
tests: Task 03 asserts that a recoverable failure produces exactly one completed
turn reporting `had_output=true` with the recovery entry attached to it, and Task
04 asserts that `error_output` carries the sanitized failure detail for a
post-start recoverable failure and is omitted when sanitization leaves nothing.

## Work orders

- [Task 01: Rewrite change-request schemas and move constraints to handlers](task-01-rewrite-change-request-schemas.md)
- [Task 02: Enforce the portable subset at the compile seam](task-02-enforce-portable-subset.md)
- [Task 03: Suppress the duplicate empty-turn notice on error-terminated turns](task-03-suppress-error-turn-empty-notice.md)
- [Task 04: Surface sanitized failure detail on post-start recoverable failures](task-04-surface-recoverable-failure-detail.md)
