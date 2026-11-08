---
created: 2026-11-07
status: done
requirements:
  - REQ-COORDINATOR-ANSWER-001
  - REQ-COORDINATOR-ANSWER-002
  - REQ-COORDINATOR-ANSWER-003
  - REQ-COORDINATOR-ANSWER-004
  - REQ-COORDINATOR-ANSWER-005
  - REQ-COORDINATOR-ANSWER-006
system_design:
  - ../../specs/coordinator/system-design/answer-proposals.md
legacy_specs: []
---

# Implementation Plan: Coordinator answer proposals

## Overview

Adds the `answer` proposal kind: the coordinator drafts the answer to a
watched card's clarification or permission request and a manager approves,
edits or rejects it. Stacked on the raisable-classes work (PR #5149): it uses
that branch's policy and class code and does not change the raisable set.
Decision: [ADR 2026-11-07](../../decisions/2026-11-07-coordinator-answer-proposals.md).

## Work orders

| Order | Result | Depends on |
| --- | --- | --- |
| [01](task-01-kind-policy-flag.md) | Kind, policy class, flag, tool, propose and approve delivery | none |
| [02](task-02-staleness-recording.md) | Refusal settle, recording, activity and summary counts | 01 |
| [03](task-03-answer-card.md) | Answer card, settings label, locales, e2e | 01 |

## Verification

`cd apps/backend && go test ./internal/coordinator/... ./internal/mcp/... ./internal/runtimeflags/... -count=1`;
`cd apps/web && pnpm run typecheck && pnpm run i18n:check`;
E2E decision: the Needs you page and the settings May do table change, so one
Playwright spec covers propose, edit-approve, stale and reject on desktop and
phone widths.

## ASCII UI previews

UI-ANS-01 (answer card, desktop):

```text
| Answer  Q: Which database should the cache use?     [Needs your decision] |
| Draft:  (•) Redis   ( ) Memcached                       confidence: Medium |
| Sources: docs/adr/0007.md:42 [copy] · Q-12 [copy] · https://… ↗           |
| Why: ADR 7 fixes Redis for session data; no ADR covers a cache.            |
| [Approve] [Edit] [Reject] [Reply with condition]        Open task          |
```

UI-ANS-02 (phone): the same fields stacked; actions in a bottom row of
44-pixel targets; sources collapsed behind "3 sources".

## Build decisions

Round 7 (2026-11-08) accepted these defaults:

- R7-01: the refusal status is 409.
- R7-02: the refusal body carries a bare message.
- R7-03: validation errors are ordered alphabetically, then in schema order.
- R7-04: draft validation runs in entry order, then checks the exact set.
- R7-05: a pending open answer blocks a new propose with `answer_pending`.
- R7-06: `checkOrdersActive` runs after `DedupeOpen`.
- R7-07: AC-004.4 is narrowed to "phase 2 on".

Scope corrections (R6-04, R6-05): task 01 also covers `approve_kinds.go` and
`internal/mcp/handlers/coordinator_propose_kinds.go` and owns AC-001.10; the
card lives in `apps/web/app/coordinator/proposal-card/`.

## Verification receipt

- `make fmt`, `make lint-format`, `pnpm run i18n:ratchet`, `scripts/lint-go-changed` (0 issues), `sqlguard`, web `lint` and `typecheck`, `git diff --check`, `list-docs.py validate`, `lint-spec-files.py --all`: clean.
- `make test` (backend): `internal/coordinator`, `internal/mcp/...`, `internal/profiles`, `internal/runtimeflags` pass. 24 other packages fail with environmental git/worktree/process/timeout errors in code this work does not touch. The two packages whose files this work edited (`internal/common/config`, `internal/backendapp`) fail on the identical tests at a clean HEAD (4c39850539): `TestConfigSourceLoadsStableYAMLFields` and the six `TestWorkspaceSourceMaterializer_*` tests.
- Playwright desktop and mobile answer specs pass.
- Not covered: the PostgreSQL-gated supersede race test.

## Risks

- The clarification resolver's outcome validation is unexported; two additive
  exports in `internal/clarification` (`ValidateOutcome`,
  `BundleQuestions`) are the only allowed change outside the coordinator,
  tool, flag and web packages.
- A refused proposal is `rejected` with a reserved error code, never `failed`:
  `failed` counts as open in the unique index, the cap and the pending list.
  A test must prove a refused row frees the slot.
- `ParsePolicy` must leave an absent `answer` absent; a lowering or settings
  save that fills it would store `denied` over the default.
- Adding a seventh class touches every `AllActions` loop and policy fixture;
  run the full coordinator tests, not only the new ones.
- No periodic check (deferred); staleness settles at propose and at approve.
- Stacked: re-read #5149's head before building and again before the PR.
