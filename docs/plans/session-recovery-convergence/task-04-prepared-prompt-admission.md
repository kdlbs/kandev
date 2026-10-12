---
id: "04-prepared-prompt-admission"
title: "Persist the prepared prompt at admission"
status: in_progress
wave: 4
depends_on:
  - 03-recovery-contract
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 04: Prepared prompt admission

## Evidence and scope

A focused lifecycle/SQLite reproduction dispatches an already-normalized
attachment descriptor successfully. The same descriptor normalized during
materialization fails before dispatch with a durable submission conflict.
SQL already contains the pre-materialization payload. Agentctl filename
collision handling and history-context injection can also change wire bytes.
The other-machine incident had attachments; its exact changed field and build
remain unknown. No incident payload is required in regression fixtures.

## Implementation

- Lifecycle passes final text and materialized descriptors into its existing
  admission callback, after preparation and before provider dispatch.
- Executor and backend adapter carry that typed callback unchanged. Existing
  cancellation, ownership and acceptance callbacks retain their ordering.
- Initial launch binds identity before startup and defers immutable SQL admission
  until final preparation. Direct and queue admission use the same final payload.
- Preserve source intent, retry identity, payload conflict and live-owner guards.
  Do not rewrite previously admitted payloads or replay old prompts.

## Validation

Extend lifecycle and orchestrator regressions for normalized metadata, filename
collisions, context preparation, queue claims, cancellation before admission,
SQL failure, and immutable retry refusal. Assert SQL/journal payload equality
and no provider dispatch on failed admission. Focused Go tests use -p 1 with
GOMAXPROCS=2 and GOMEMLIMIT=2048MiB. Browser E2E and PostgreSQL run only in CI.
Run documentation validation and scoped lint, then commit, push, open the
follow-up PR and address CI/review through merge. No delegation or new sessions.

## Results

The temporary pre-fix reproduction failed at the exact reported error, with
unchanged SQL bytes and no native dispatch. Its normalized control passed.
Permanent lifecycle/SQLite tests pass for metadata hydration, collision renaming,
exact SQL/agentctl-request payload equality, rejection before dispatch and
immutable duplicate refusal. Focused executor/backend adapter checks and
orchestrator delivery-retention, initial-admission, transformed-retry and
fallback tests pass. The broader focused orchestrator run passed every assertion
but exposed a fallback simulator leak; explicit cancellation and cleanup joining
fixed that leak, and the affected package rerun passes. Documentation catalog
validation and specification lint pass. CI owns PostgreSQL, browser E2E and
broader validation. Follow-up PR: https://github.com/kdlbs/kandev/pull/4435.

Focused commands use `go -C apps/backend test -p 1 -trimpath -tags fts5` with
`GOMEMLIMIT=2048MiB GOMAXPROCS=2`. Final orchestrator selection:
`Test(DeliveryRetention|PromptTask_LegacyCallerComposesSavedPromptOnFreshFallback)`.
No local E2E or PostgreSQL ran.

Review remediation preserves created-session Send Now claims until actual prompt
acceptance. Confirmed admission rejection or cancellation before admission restores input;
cancellation after admission or uncertain acceptance retains a recovery claim and prevents delayed admission by the stopped
worker. Typed persistence failures use initial-turn failure handling, while
ownership/cancellation rejections retain their existing cleanup. Regressions cover
all three claim outcomes, ordinary and callback-owned failure routing, retained
continuation ordering, and MCP readiness. CI remains responsible for E2E.

CI also exposed an outdated MCP initial-launch fake. It now invokes prepared
admission before reporting prompt acceptance, preserving the real startup order.
The focused `TestPeerMessageInitialLaunch_` suite passes, including queued
follow-ups during startup and workflow-turn ownership. This remediation changes
test support only; production admission guards remain unchanged.
