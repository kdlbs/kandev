---
id: "01-empty-turn-unclassified-failure"
title: "Count empty normal turn ends as unclassified failures"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.3
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.5
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.6
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.7
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.13
system_design:
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
---
# Task 01: Count empty normal turn ends as unclassified failures

## Summary

Produce the `empty_turn_completion` origin at stream completion and admit it in the dynamic engine without weakening any other origin.

## In scope

- Write `TestEmptyTurnCompletionCountsUnclassifiedFailure` first and confirm that it fails on the base commit.
- `apps/backend/internal/agent/runtime/dynamic/types.go`: add the origin; waive `DiagnosticComplete` and provider ID only for it; match `agent_runtime_error` at `prompt_send`.
- `apps/backend/internal/agent/runtime/dynamic/unclassified_fingerprint.go`: fixed, versioned fingerprint for the origin.
- `apps/backend/internal/agent/runtime/dynamic/engine.go`: `CandidateUnclassifiedEnabled` reuses the engine's threshold rule.
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`: `UnclassifiedPolicyEnabled` loads the logical profile and reports whether the concrete candidate's policy is enabled.
- `apps/backend/internal/orchestrator/dynamic_empty_turn.go`: the producer, its evidence, and the counted-turn record keyed by session, execution ID, and prompt generation.
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`: call the producer while the stream guard is held, before the ordinary completion follow-up; retire the counted-turn record at turn start.
- `apps/backend/internal/orchestrator/dynamic_evidence.go` and `service.go`: consume the counted-turn record in the completion-clear path; hold the record map.
- Public sentence in `docs/public/agents-and-profiles.md`.

## Out of scope

- Post-start runtime failures without a complete diagnostic. They remain manual under the accepted diagnostic identity contract.
- Catalogue changes, new codes, UI, runtime flags, and E2E tests.
- Task-environment recovery, lifecycle launch builders, provider usage rules, and agent control clients.

## Acceptance

- Policy absent, disabled, vetoed, or step unknown: no route call and unchanged completion.
- Cancelled, errored, terminal-replay, stale, or output/effect turns: not counted.
- Threshold N: N consecutive empty ends on one candidate advance once; below N, the route is `action_required`.
- Other origins still require a complete diagnostic and provider ID.
- Persisted state contains only the fingerprint and bounded identity fields.

## Validation

```bash
cd apps/backend
go test ./internal/agent/runtime/dynamic -count=1
go test ./internal/agent/runtime -run 'Resolver|Unclassified|Dynamic' -count=1
go test ./internal/orchestrator -run 'Unclassified|EmptyTurn|Streak|CompleteStream|CompleteEvent|DynamicFailure|DynamicRoute' -count=1
go build ./... && go vet ./internal/agent/runtime/... ./internal/orchestrator/...
```

Documentation: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `.github/scripts/pr-docs.cjs` `validateCoverage` on the changed-file list.

## Review remediation results

The exact-source contained completion-caller harness failed when a complete-frame answer preserved a previous streak. It passes with output observation and the existing reset boundary, under the race detector. The permanent real-stream-handler assertion now requires the previous streak to be cleared as well as the answer persisted and no successor launched. Real-package validation and final exact-head CI remain hosted/external.
