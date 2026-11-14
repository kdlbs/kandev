---
id: "01-passive-skip-notice"
title: "Persist an automatic no-changes review notice"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
acceptance_criteria:
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.5
system_design:
  - ../../specs/agents/system-design/workflow-review-no-changes.md
---

# Persist an automatic no-changes review notice

## Scope and preserved behavior

Connect `runCodeReviewCallback` through the real message adapter and task
service to the existing status-message renderer. Retain the runner's selected
session in an internal typed error while preserving `errors.Is`, `CodeFor`,
the manual error text, and the no-review-row contract. Reuse the existing
completed-lifecycle turn flag and existing task/entry identities for replay.

No provider/admission extension, model call, operating mutation, successful
review fabrication or new API/schema belongs to this delivery. Existing
normal-diff reviews, other errors and history remain unchanged.

## Existing UI preview

```text
Task conversation
  [info] No changes to review yet.
```

The existing localized status renderer and task conversation supply the same
notice on desktop and phones, including after message backfill.

## Verification status

The original producer baseline failed the actual callback's missing persisted
message, missing resolved-session context, and non-passive idempotent producer.
The candidate passed `go test -tags fts5 -trimpath -p 2 -race -count=1` with the
temporary overlay for orchestrator, review and backendapp (1.573s, 1.101s and
1.683s). The selection included existing manual/no-run, normal-diff and other
error controls. Actual SQLite readback confirmed no review run, passive
lifecycle-only turn, entry replay and original foreground turn/history
preservation.

The actual stored API message supplied the UI confirmation. The original
renderer retained fallback text; the candidate passed two focused UI checks
(fresh backfill/remount and unrelated persisted status). The first UI attempt
stopped before collection on temporary outside-project React resolution; only
that temporary config changed, and the passed Go checks were not repeated.
Normal hooks must pass before the upstream writer publishes. No permanent
tests are introduced.


## Rejected lifecycle message follow-up

Message persistence failure now invokes the existing unreferenced-turn rollback
for only a completed turn newly created by that request. Existing explicit turn
IDs and foreground turns are preserved. The same repository guard retains a
message that committed before response loss. Cancellation uses bounded cleanup
without changing the original failure or adding a background retry loop.

Disposable actual SQLite INSERT rejection checks reproduced empty turns in
ordinary, assigned-ID and idempotent entry points; cancellation reproduced the
same baseline failure. The formatted candidate passed all selected checks with
the race detector (task/service 2.459s), including retry, committed response-loss
and caller-supplied foreground-turn controls. Messages and original foreground
history were preserved. Full-PR documentation coverage passed. Normal commit
hooks must pass before publication. No permanent cases, new storage contract,
schema or operating mutation are added.
