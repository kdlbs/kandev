---
id: "01-bounded-preparation"
title: "Fix oversized generic tool delivery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-001
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-002
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-003
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.1
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.2
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.3
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.4
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.5
  - AC-PLATFORM-BOUNDED-TOOL-DATA-002.1
  - AC-PLATFORM-BOUNDED-TOOL-DATA-002.2
  - AC-PLATFORM-BOUNDED-TOOL-DATA-002.3
  - AC-PLATFORM-BOUNDED-TOOL-DATA-003.1
system_design:
  - ../../specs/platform/system-design/bounded-tool-delivery.md
---

# Task 01: Fix oversized generic tool delivery

## Summary

Bound generic JSON input/output and duplicate content before delivery admission.
Use existing JSON fields for the truncation flag/preview, avoiding unrelated
protocol, SQL, hash and UI changes.

## In scope

Shared bounded representation, adapter admission, direct process defense and
regression tests through the real persistence/forwarding path.

## Out of scope

Full-output retrieval, new schemas/protocols/UI, typed-tool policy, permission
or workflow changes, broad logging and live-instance repair.

## Acceptance

1. The real forwarder retains an oversized MCP result, later ordinary output
   and actual terminal in order; tool status and source result stay unchanged.
2. Shared and adapter tests prove bounded JSON values/duplicates, unchanged
   small values, idempotence, UTF-8/escaping correctness and immutable snapshots.
3. Current journal/hash formats and existing generic rendering remain usable;
   omitted result data carries an explicit `truncated: true` marker.

## Verification

Run sequentially from the repository root with a writable Go cache:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agentctl/server/process -run 'Test.*(BoundedTool|NormalizedPayloadSnapshot|IncrementalShellNoopAndTerminal|OversizedOutputFailsWithoutPoisoningJournal)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agentctl/server/process -run 'Test.*BoundedTool' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run changed-file Go lint with `bash scripts/lint-go-changed <changed-go-files>`.
No browser flow changes, so no new E2E, locale or visual verification is needed.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/tool_display_bounds.go` and tests.
- ACP `adapter.go` and native Codex `adapter.go` emission seams and tests.
- `apps/backend/internal/agentctl/server/process/manager.go` and
  `bounded_tool_delivery_test.go`.
- `docs/public/sessions-and-review.md` for the bounded-copy behavior.

## Dependencies

None. Read the narrowed requirements/design and record decision in this package.

## Risks

Duplicate content can bypass a result-only bound. Reduction must not mutate a
shared provider payload or alter tool/control provenance.

## Parallelism

`sequential`

## Inputs

Existing generic Snapshot and detached durable producer tests are the patterns.
The user authorized this narrowed implementation on 2026-10-22.

## Results

Completed 2026-10-22.

- RED: `TestBoundedToolResultContinuesToTerminal` reproduced queue rejection and
  missing retained events. The small-value regression also caught nil-container
  changes and premature truncation of a 32 KiB ordinary string before correction.
- GREEN: the targeted command above passed all four affected packages after the
  final production edit. Successful, failed and cancelled tool statuses retained
  the oversized result, following text and real terminal in order.
- The race command above passed all four packages after the final edit.
- Complete streams, ACP and native Codex package suites passed during development.
- `bash scripts/lint-go-changed` over all eight added/changed Go files: 0 issues.
- Specification catalog/lint, focused work-order coverage preflight, public-doc
  tests/validation (54 pages), and whitespace checks passed.
- PR review regressions reproduced unstable deep/node-limited previews, lost
  content-only previews and shared fitting-content pointers. Reserved marker
  budgets, bounded text fallback and copied content fix these cases. Targeted
  tests and race checks passed again across all four affected packages.

Go commands used `GOCACHE=/tmp/kandev-tool-fix-go-cache` because the default host
cache is read-only in this execution environment. Lint also used an isolated
`GOLANGCI_LINT_CACHE`. No production instance was restarted or mutated.

No frontend code, protocol/schema/hash version, setting or dependency changed.
Public docs updated: `docs/public/sessions-and-review.md` (explanation).
