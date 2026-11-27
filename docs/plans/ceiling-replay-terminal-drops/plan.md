---
created: 2026-10-12
status: in_progress
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
  - REQ-OFFICE-SCHEDULER-001
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
  - ../../specs/office/system-design/scheduler-02.md
legacy_specs: []
---

# Implementation Plan: Stop retrying ceiling replays that can never succeed

## Overview

The session ceiling sweep runs every 20 seconds and preserves a deferred launch
whenever its replay fails for a non-ceiling reason. Two kinds of record can
never succeed on a later pass, yet the sweep keeps them forever:

- A replay whose launch fails command validation. `startTask` creates a session
  before the lifecycle manager rejects the built command, so every pass leaves
  one more failed session for the same task. The sweep's warning names neither
  the error nor its source, so the loop is hard to diagnose.
- A record whose workflow entry is structurally unreplayable, such as a legacy
  sessionless `start` or a binding that cannot be derived. Entry validation
  reports these as `unavailable`, the same disposition as a failed read, so the
  record is retried indefinitely and keeps refusing later automatic launches for
  the task as conflicts.

Office run retries have the same gap for launch-validation failures: four
backoff attempts each rebuild the same invalid command.

## Work orders

| Order | Work order | Result | Depends on |
| --- | --- | --- | --- |
| 01 | [Drop invalid launch replays](task-01-drop-invalid-launch-replays.md) | Replay results carry their cause; launch-validation failures drop the record and skip Office backoff. | None |
| 02 | [Drop structurally unavailable entries](task-02-drop-structurally-unavailable-entries.md) | Entry validation separates permanent dead ends from read failures; dead ends are dropped. | 01 (shares `ceiling_replay.go` result plumbing) |

## Scope

### In scope

- A replay result type that carries the error and a short detail, used by
  every per-kind replay and by `settleCeilingReplay`.
- A permanent-failure outcome for launch-validation errors, dropped with a new
  `ceiling_dropped_launch_invalid` reason.
- Immediate escalation of Office runs whose launch fails command validation.
- A `permanently unavailable` entry disposition for unreplayable shapes, and an
  error return for step, transition, and task reads during entry validation.

### Out of scope

- An operator route to discard a deferred launch.
- Clearing a queued `start` when a task moves into a step that cannot run an
  agent. The sweep already drops such a start through its step auto-start check.
- Queue ordering, retry backoff for retained records, and ceiling configuration.
- Office runtime token scoping.

## Risks

- The lifecycle producer identifies built-command validation with a typed error.
  Replay and Office consumers preserve its cause through wrappers and use
  `errors.As`; provider and environment message text cannot mark it permanent.
- A new `permanently unavailable` disposition must not reach a path that treats
  `unavailable` as retryable. Every caller of entry validation is updated in
  work order 02.

## Verification

Each work order adds focused unit tests beside the changed code. Each test is
run against the unchanged base first and must fail on behavior, not on
compilation. Validation commands:

```bash
cd apps/backend && go test ./internal/orchestrator/ -run '<changed tests>'
cd apps/backend && go test ./internal/office/service/ -run '<changed tests>'
cd apps/backend && go build ./... && go vet ./internal/orchestrator/ ./internal/office/service/
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## PR review remediation

The producer now emits a typed built-command failure, and all replay kinds carry
it to settlement, including cold-resume ensure and dynamic relaunch paths.
The start eligibility check preserves failed workflow reads for already bound
records. Permanent regressions cover the real producer, session-backed replay
paths, Office agent/continue failures, transient controls, and bound-step reads.
The idle-focus lease regression now inspects the replay result's outcome while
retaining its error and all lease/record assertions.

Bounded exact-source RED/GREEN proofs reproduced the wrapped-validation and
bound-step-read failures and passed after remediation. These proofs provide no
full-package, SQL, or runtime coverage. Hosted complete validation is pending.
Public recovery guidance is updated in the agent profile troubleshooting and
Office provider-routing pages.

Stored workflow-binding decode and incompleteness failures are now terminal.
A four-field durable sweep regression prevents these records from retrying;
genuine repository read failures remain retained. Bounded exact decoder/prefix
RED/GREEN proof passes with no package or SQL coverage credit.
