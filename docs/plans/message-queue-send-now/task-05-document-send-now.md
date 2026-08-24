---
id: "05-document-send-now"
title: "Document Send Now behavior"
status: done
wave: 4
depends_on: ["02-replacement-turn-dispatch", "03-send-now-queue-controls"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-send-now.md"
---

# Task 05: Document Send Now behavior

## Intent

Update the existing public queue and session guidance so users can distinguish
Send Now from Run next, Clear all, and the workflow-aware Cancel control.

## Acceptance

1. `coordination.md` documents selected/all Send Now behavior, FIFO bulk
   concatenation, and its non-workflow-completing replacement semantics.
2. `sessions-and-review.md` explains when to use Run next, Send Now, Clear all,
   or Cancel without duplicating the full queue contract.
3. Public-doc validation and internal links pass; no new page or navigation
   entry is introduced.

## Files likely touched

- `docs/public/coordination.md`
- `docs/public/sessions-and-review.md`

## Dependencies

- Task 02 final backend behavior and Task 03 final labels.

## Parallelism

Parallel-safe with Task 04 after dependencies complete. Files are disjoint and
neither task changes a shared contract or package configuration. User
authorization is still required before using subagents.

## Inputs

- Spec: What, Failure Modes, Out of Scope.
- Plan: Public Documentation.
- Public docs type: `coordination.md` is primarily explanation/reference;
  `sessions-and-review.md` is primarily a how-to guide.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The public-doc files are disjoint from Task 04, but final status/results edits
to the shared `plan.md` record are serialized after both tasks finish.

## Output contract

Report public files changed, the terminology/behavior clarified, exact validator
results, link issues or risks, and synchronize this task plus `plan.md`
status/results.

## Results

- Updated `docs/public/coordination.md` and `docs/public/sessions-and-review.md` to distinguish Run next, selected/all Send Now, Clear all, and normal Cancel, including FIFO bulk behavior and non-workflow-completing replacement semantics.
- GREEN: `node --test scripts/validate-public-docs.test.mjs` — 58 tests passed; `node scripts/validate-public-docs.mjs` — 41 published docs pages validated; `git diff --check` passed.
- No new public page or navigation entry was introduced.
