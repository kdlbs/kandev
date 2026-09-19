---
created: 2026-09-06
status: implemented
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
  - ../../specs/ui/system-design/ci-pr-automation-02.md
  - ../../specs/ui/system-design/ci-pr-automation-03.md
legacy_specs: []
---

# Implementation Plan: PR Auto-Fix Outcome Retries

## Overview

Replace enqueue-time permanent acknowledgement with a durable per-PR attempt
lifecycle. Bind every delivered GitHub auto-fix prompt to its exact turn,
require a task-bound outcome signal, and make an undispositioned or unverified
attempt eligible on a later settled watch cycle. Keep queued work coalesced and
retain the existing 10-round safety cap.

Upgrade only untouched legacy `ci-auto-fix` prompt rows. Add the immutable
outcome instruction outside saved prompt content, explain the retry behavior in
the existing desktop/mobile help surface, and update public review guidance.

## Scope

### In scope

- Persist queued, running, awaiting-progress, acknowledged, and retryable
  attempt states per linked GitHub PR.
- Correlate queue entry, session, turn, feedback signature, and provider
  generation across direct delivery, queued replacement, queue drain, turn
  completion, and restart.
- Add `report_pr_auto_fix_outcome_kandev` with `action_taken`,
  `non_actionable`, and `blocked` outcomes bound to the current auto-fix turn.
- Retry an undispositioned successful or recoverably failed turn on a later
  settled PR evaluation.
- Retry reported action after two complete watcher intervals without
  provider-visible progress.
- Include PR head and check execution timestamps in provider-generation
  detection.
- Preserve queue coalescing and the 10-round cap.
- Upgrade exact untouched legacy built-in prompt revisions and preserve edits.
- Update shared desktop/mobile help, translations, public docs, and focused E2E
  evidence without changing the existing popover or drawer composition.

### Out of scope

- GitLab MR auto-fix outcome parity.
- Parsing final agent prose or shell history as completion evidence.
- Automatically rerunning GitHub Actions from the backend.
- Changing the one-minute PR watch cadence or the 10-round limit.
- Adding another status chip, drawer, popover, or notification surface.
- Retrying after explicit user cancellation, task archive/deletion, disabled
  auto-fix, or terminal PR state.

## Technical approach

### Durable attempt lifecycle

Extend `github_task_ci_pr_state` with one attempt state and the minimal identity
needed to compare-and-set it: queue entry, session, turn, feedback signature,
provider generation, outcome, bounded summary, outcome time, and progress
deadline. Keep the current checkpoint as the source for feedback delta.

Store methods make first-write outcome and turn-completion reconciliation
conditional on the same task/repository/PR/session/turn/signature tuple. A
restart reconciler keeps a live queue reservation intact and marks a terminal
undispositioned turn retryable.

### Dispatch and completion

Return queue-entry and turn identity from the shared dispatcher without adding
GitHub fields to its provider-neutral parameters. The GitHub adapter stores the
attempt and adds immutable identity to user-message/queue metadata. Queue drain
binds the accepted turn before completion can settle it.

Normal `agent.ready` and recoverable-failure paths call one narrow reconciler
with the completed turn ID. The reconciler changes only a matching running
GitHub auto-fix attempt. It does not alter ordinary messages, workflow
transitions, GitLab automation, cancellations, or terminal tasks.

### Outcome protocol

Register `report_pr_auto_fix_outcome_kandev` only for task MCP servers with the
GitHub provider. The MCP server supplies task and session identity; callers
supply only the outcome and a bounded plain-text summary. The backend resolves
the current turn and accepts the outcome only for its matching unresolved
attempt.

Append immutable outcome instructions after saved-prompt expansion. Keep them
hidden in structured chat and plain in passthrough input. The saved
`ci-auto-fix` prompt still controls repair technique and can omit
`{{pr.feedback}}`; it cannot change the outcome contract.

### Prompt compatibility and guidance

Add a `ci-auto-fix` legacy refresher that follows the existing
`changes-walkthrough` exact-hash and unchanged-timestamp pattern. Update the
shared round/help copy in all five locales. This is content-only inside the
existing `PRStatusChipDrawer` mobile surface: the existing drawer remains the
entry point and scroll owner, and no responsive structure or touch target
changes.

## Work orders

- [x] [Task 01: Persist PR Auto-Fix Attempt State](task-01-persist-pr-auto-fix-attempt-state.md)
- [x] [Task 02: Reconcile Turns Through Explicit Outcomes](task-02-reconcile-turns-through-explicit-outcomes.md)
- [x] [Task 03: Upgrade Prompt Defaults Safely](task-03-upgrade-prompt-defaults-safely.md)
- [x] [Task 04: Explain and Prove Retry Behavior](task-04-explain-and-prove-retry-behavior.md)

## Dependency order

1. Tasks 01 and 03 have independent implementation boundaries.
2. Task 02 depends on Task 01.
3. Task 04 depends on Tasks 02 and 03.

Implementation remains in the user-started primary session unless the user
later authorizes delegation.

## Verification strategy

- SQLite and Postgres-compatible store tests prove migration replay, CAS
  identity, state transitions, reset behavior, and restart reconciliation.
- Orchestrator tests prove direct and queued turn binding, naked-halt retry,
  outcome handling, progress deadlines, dedupe, and the 10-round cap.
- MCP handler/server tests prove task/session binding, provider membership,
  stale-call rejection, schemas, and tool-count contracts.
- Prompt-store tests prove known legacy upgrades and preservation of edited or
  unknown rows.
- Frontend component tests and desktop/mobile Playwright scenarios prove the
  shared explanation and visible round progression without geometry changes.
- Public-doc validation and repository spec lint protect documentation.

## Risks

- A dispatch/turn-completion race could leave an accepted prompt unbound. Bind
  the turn before terminal reconciliation and cover both event orders.
- Queue replacement can change the signature without consuming a round. The
  attempt CAS must follow the replacement row rather than an obsolete turn.
- Provider progress can be brief between one-minute polls. Include check start
  and completion timestamps and PR head identity, but accept that a very fast
  identical rerun can conservatively consume another bounded round.
- A stale outcome tool call could acknowledge a newer attempt. Exact
  task/session/turn/signature matching must fail closed.
- Prompt migration could overwrite user content. Require built-in identity,
  exact known hash, and equal creation/update timestamps in the conditional
  update.
- Automated retries can consume the cap faster. Help and public docs must make
  that consequence explicit.

## Verification results

Implemented and verified.

- `make -C apps/backend test` passed for all packages with ambient Kandev
  runtime variables removed so temporary configuration fixtures remain isolated.
- `make -C apps/backend lint` passed with zero issues.
- Focused backend tests for GitHub state, orchestrator reconciliation, MCP
  outcome reporting, and prompt upgrades passed.
- Web component tests, desktop Chromium E2E, and mobile Chromium E2E passed.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, web typecheck, and web lint
  passed.
- Public-doc validation, specification lint, and `git diff --check` passed.
