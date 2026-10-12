---
id: "01-single-start-owner-for-automation-tasks"
title: "Single start owner for automation tasks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.10
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Single start owner for automation tasks

## Summary

Make the automation dispatch the only initial starter of a task the automation
creates, so a feeder pull's workflow auto-start cannot race it.

## In scope

- Mark newly created tasks before feeder publication, never reused continuations.
- Reject ownership injection through actual registered REST/WS adapters.
- Preserve ownership in ordinary metadata replacement and merge on both dialects.
- Fence queued starts and uncertain retry acknowledgement, cancellation and Stop.
- Release the exact owner after definitive admission or termination through CAS.
- Reconcile startup and later workflow entry using existing durable evidence.
- Update requirements, system design and the public automation explanation.

## Out of scope

Rendered UI, migrations, flag changes and competing retry ownership machinery.
Existing failed-stop/no-replacement and generic retry/recovery fences are retained.

## Acceptance

- A feeder move cannot start an automation's new task before its dispatch.
- The internal request's run identity is present at task construction.
- Queued dispatch remains owned; a definitive drop releases its exact marker.
- Restart and lease age cannot release unresolved ownership across instances.
- Stale cleanup cannot clear a successor, and successful release publishes once.
- Missing/read-uncertain ownership and failed Stop remain fenced. A dropped
  unbound legacy run also retains ownership if a session exists or its session
  read is uncertain; session state alone cannot prove provider termination.
- Continuations preserve original task/session identity and marker.
- Registered wire callers and ordinary metadata writes cannot inject or erase it.

## Verification

Contained local proof uses extracted actual source and stdlib-only tests with
explicit fake boundaries; it does not compile the full orchestrator or lifecycle.
The work-order repository/caller commands run in hosted CI:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'Automation|AutoStart|TaskMoved|Feeder|WIP|Ceiling|Startup' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/models ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite -count=1)
(cd apps/backend && go vet ./internal/orchestrator ./internal/task/...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

PostgreSQL contracts require the hosted isolated PostgreSQL fixture. Full actual
Backend/Frontend/E2E CI remains an exact-head external delivery gate.

## Owned files

The automation request adapter, task construction/protection, workflow guard,
initial-start ownership reconciler and ceiling replay/drop boundaries; their
regression tests; the linked ceiling requirements/design and public automation
explanation. Preserve all current-main contracts in these files.

## Dependencies

Current durable retry launch ledger, immutable admission, metadata CAS and
failed-cleanup guards. No new schema or parallel owner.

## Risks

Unresolved legacy markers stay fenced until definitive evidence is available;
explicit task/session recovery remains subject to existing retry controls.

## Parallelism

`sequential`

## Inputs

Issue #4323, REQ-AGENTS-SESSION-CEILING-001,
AC-AGENTS-SESSION-CEILING-001.10 and the linked system design.

## Results

Implementation and contained extracted-source proofs are complete. RED observed
unresolved startup release and stale cleanup erasing a successor. GREEN passed
with race detection. Actual repository/caller/full-suite verification is required
in hosted CI before delivery; immutable receipts are external so validation does
not require status-only source pushes.
