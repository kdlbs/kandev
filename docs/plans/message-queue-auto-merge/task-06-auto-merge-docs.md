---
id: "06-auto-merge-docs"
title: "Document automatic queue merging"
status: completed
wave: 6
depends_on: ["05-auto-merge-e2e"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-auto-merge.md"
---

# Task 06: Document automatic queue merging

## Acceptance

1. Public coordination, session, operations, configuration, and WebSocket docs
   explain the default-on switch, strict same-source fallback, capacity-first
   behavior, returned surviving ID, independent manual switch, and current Task
   Behavior settings location.
2. Related queue management/manual merge specs link to this feature and show the
   normalized three-field setting without changing manual merge behavior.
3. After all implementation evidence passes, the new spec/index and design
   package statuses are synchronized, links resolve, public-doc validators pass,
   and `git diff --check` is clean.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `docs/public/coordination.md`
- `docs/public/sessions-and-review.md`
- `docs/public/operations.md`
- `docs/public/configuration.md`
- `docs/public/websocket-api.md`
- `docs/specs/ui/requirements/message-queue-management.md`
- `docs/specs/ui/requirements/message-queue-merge.md`
- `docs/specs/ui/requirements/message-queue-auto-merge.md`
- `docs/specs/INDEX.md`
- `docs/plans/message-queue-auto-merge/plan.md`
- `docs/plans/message-queue-auto-merge/task-*.md`

## Dependencies

Task 05.

## Parallelism

Sequential.

## Inputs

- Full spec and completed task results.
- Plan: `Risks and boundaries` plus public-documentation impact.
- Existing public queue sections found by searching `Message Queue`,
  `message.queue.add`, and `KANDEV_QUEUE_MAX_PER_SESSION`.
- Docs-maintainer rule: document observable behavior and configuration, keep
  public terminology current, and run both validator commands.

## Risks

- Do not imply that automatic merging bypasses a full queue or retroactively
  compacts pending rows.
- Distinguish message-queue auto merge from PR/MR auto-merge terminology.
- Mark the feature shipped only after implementation and E2E evidence exist.

## Output contract

Report summary, files changed, exact commands/outcomes, generated/cleanup
evidence, blockers, risks, and update this task plus `plan.md` status in the same
conversation.

## Results

- Updated public coordination, sessions, operations, configuration, and
  WebSocket documentation with the current settings path and observable
  automatic-merge contract.
- Linked the management and manual-merge specs, normalized the documented
  three-field settings shape, and marked the implemented feature spec shipped.
- Verification passed:
  - `node --test scripts/validate-public-docs.test.mjs` (60 tests);
  - `node scripts/validate-public-docs.mjs` (41 published pages);
  - `git diff --check`.
- Pseudo-locale regeneration completed, build output remained ignored, and the
  lockfile stayed unchanged.
- Blockers: none.
