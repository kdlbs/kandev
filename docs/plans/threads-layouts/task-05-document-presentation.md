---
id: "05-document-presentation"
title: "Document Threads presentation"
status: done
wave: 5
depends_on:
  - "04-display-settings"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-005
  - REQ-UI-THREADS-DECK-004
  - REQ-UI-THREADS-DECK-005
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-005.4
  - AC-UI-THREADS-SAVED-VIEWS-005.5
  - AC-UI-THREADS-SAVED-VIEWS-005.6
  - AC-UI-THREADS-DECK-004.6
  - AC-UI-THREADS-DECK-004.7
  - AC-UI-THREADS-DECK-004.8
  - AC-UI-THREADS-DECK-005.1
  - AC-UI-THREADS-DECK-005.5
  - AC-UI-THREADS-DECK-005.7
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 05: Document Threads presentation

## Summary

Update the existing Threads user guide to explain choosing layouts and
auto-hide, their saved-view behavior, and the phone interaction. Synchronize
the scoped engineering notes with the implemented contracts.

## In scope

- Update the Threads section of `docs/public/sessions-and-review.md` as a
  concise how-to: Display, Save/Discard, Grid, Maximum chats, auto-hide,
  required actions, visible touch composers, CI-only collapse, and the existing Open task.
- Check `docs/public/tasks-and-workflows.md`, root README, and screenshot
  catalog for directly contradicted Threads wording; edit only affected text.
- Update `apps/web/components/threads/AGENTS.md` with stable direct tiles,
  two-row/phone geometry, the saved preference owner, and composer ownership.
- Record final work-order results and promote the package status only after
  implementation matches the reviewed designs and required checks passed.

## Out of scope

Publishing docs, a new docs page/navigation entry, production screenshots,
runtime demos, code cleanup, and a generic QA/review task.

## Acceptance

1. The existing guide explains the complete user flow and defaults, including
   five total chats, height/phone fallbacks, preserved drafts, and visible Stop
   or required actions.
2. Engineering notes name the actual implemented ownership and preserve
   viewport budgets and session submission behavior.
3. Documentation validators pass; each work order records its own actual
   commands/results without claiming unrun tests or prior-package counts.

## Verification

Use /docs-maintainer. Run from the repository root.

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/public docs/specs docs/plans/threads-layouts apps/web/components/threads/AGENTS.md README.md docs/screenshots.md
git status --short -- docs/plans/threads-layouts
```

This work order changes documents only. It does not rerun product suites
without a new product change or unresolved concern. Final package results
reference the Task 04 integration checks and each earlier work-order result.

## Files likely touched

- `docs/public/sessions-and-review.md`
- `docs/public/tasks-and-workflows.md`, `README.md`, and
  `docs/screenshots.md` only if existing statements need correction.
- `apps/web/components/threads/AGENTS.md`
- The paired deck/saved-view requirement/design status and relevant details,
  plus this plan and completed work-order Results.

## Dependencies

Task 04 and its successful integration checks. Document the final behavior.

## Risks

Public copy must not imply auto-hide is enabled by default, that Grid doubles
a saved chat limit, or that phones show multiple live conversations. Do not
call future draft behavior shipped before the implementation is complete.

## Parallelism

sequential

## Inputs

- [Complete package and previews](plan.md)
- Current public Threads section and scoped Threads engineering notes.
- Actual implementation/test results from Tasks 01-04.

## Results

Done, 2026-09-11, after Task 04's successful integration checks.

Public docs updated: `docs/public/sessions-and-review.md`. The page remains a
how-to guide. Its Threads section now has action-first Display/Save/Discard
steps, unchanged defaults and total-chat limit, Grid/height/phone behavior,
CI-only collapse, keyboard reveal, draft preservation, required actions, and
visible touch composers. Existing Open task and task-action flows remain.
The concise steps follow the requested i-have-adhd communication style.

Checked `docs/public/tasks-and-workflows.md`, root README, and screenshot
catalog: no contradictory layout/composer wording required edits. No new
page, navigation entry, published screenshot, or diagram is needed for this
short settings flow.

Updated the scoped Threads engineering guide with actual preference, Display,
disclosure, activity, native scroll, and viewport ownership. Synchronized the
design with the implemented context reporting and immediate region hiding;
there is no height/opacity animation. Promoted the owning requirements to
active, system designs to current, and this package to implemented. Earlier
packages and their historical verification counts remain unchanged.

All listed validation commands passed: public-doc validator test (one suite),
46 published pages, 36 specification-linter tests, complete specification lint,
scoped diff/whitespace checks, and work-order status inventory. No product
files changed in this work order; Task 04 records the final 23 desktop and
19 mobile browser tests, and Tasks 01-03 retain their own focused evidence.
Physical-device keyboard testing was not available. Changes remain local and
uncommitted; no push, publication, deployment, or additional session.
