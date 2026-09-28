---
id: "10-improvements"
title: "Improvement proposals and pending changes"
status: pending
wave: 5
depends_on:
  - "06-autonomy-ui"
  - "08-reply-with-condition"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-IMPROVEMENTS-001
  - REQ-COORDINATOR-IMPROVEMENTS-002
  - REQ-COORDINATOR-IMPROVEMENTS-003
acceptance_criteria:
  - AC-COORDINATOR-IMPROVEMENTS-001.1
  - AC-COORDINATOR-IMPROVEMENTS-001.2
  - AC-COORDINATOR-IMPROVEMENTS-001.3
  - AC-COORDINATOR-IMPROVEMENTS-002.1
  - AC-COORDINATOR-IMPROVEMENTS-002.2
  - AC-COORDINATOR-IMPROVEMENTS-002.3
  - AC-COORDINATOR-IMPROVEMENTS-003.1
  - AC-COORDINATOR-IMPROVEMENTS-003.2
  - AC-COORDINATOR-IMPROVEMENTS-003.3
system_design:
  - ../../specs/coordinator/system-design/improvements.md
---

# Task 10: Improvement Proposals And Pending Changes (WP-12)

## Summary

Adds the `improvement` proposal kind and `propose_improvement_kandev`, the
improvement card with its evidence and diff, the approve branch that stores a
pending change, and the settings list where a manager applies or discards it.

## In scope

- Backend: the tool and its ordered validation, evidence checked against
  `coordinator_unattended_turns` and workspace tasks, the prompt paragraph in
  `prompt.go` ([Tool](../../specs/coordinator/system-design/improvements.md#tool));
  the approve branch and recovery
  ([Approve](../../specs/coordinator/system-design/improvements.md#approve));
  the pending-change routes with the base comparison and the phase 1 PATCH
  context write ([Pending changes](../../specs/coordinator/system-design/improvements.md#pending-changes));
  guard refusals; `coordinator_improvement_total`.
- Web: `app/coordinator/components/improvement-card.tsx` branching from
  `ProposalCard`, "Runs behind it" through task 06's run read, the diff through
  the existing diff viewer, the approve gate and the reply control from task
  08; the settings "Changes waiting for you" list
  ([Card](../../specs/coordinator/system-design/improvements.md#card)).
- Copy in six locales.

## Out of scope

- Targets other than the context; automatic approval; editing an improvement.

## ASCII UI preview

See [plan UI-07](plan.md#ascii-ui-previews).

```text
| Improvement  [Changes coordinator context]                         |
| Runs behind it   09:12 completed 0.42 USD . Run record expired     |
| [Show the change]                                                  |
| [Approve as a reviewable change] [Reject] [Reply with a condition] |
```

## Acceptance

- A valid call stores one `pending` improvement with the server-read
  `context_before` and counts toward 25; each invalid field (including no run,
  a foreign run, a foreign task, 0 or 11 references, an unchanged context) is
  refused naming the field and stores nothing.
- The card shows the evidence (expired runs as such), keeps Approve disabled
  until the diff is shown, has no Edit, and a raised `create_task` setting never
  approves an improvement.
- Approval stores one pending change and leaves the coordinator unchanged;
  Apply writes the context as a PATCH does (conversation replaced), is 409
  `context_changed` when the base differs, and Apply and Discard settle once
  under concurrency and are refused to readers and a coordinator principal.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Improvement|PendingChange' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'PendingChange.*Race' -race -count=1
cd apps/backend && go test ./internal/mcp/... -run 'Coordinator' -count=1
cd apps/web && pnpm test -- app/coordinator app/settings/workspace
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/improvements.spec.ts
```
