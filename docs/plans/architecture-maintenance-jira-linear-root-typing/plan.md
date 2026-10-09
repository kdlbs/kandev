---
created: 2026-10-09
status: in_progress
requirements:
  - REQ-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001
system_design:
  - ../../specs/architecture-lint/system-design/root-state-slice-typing.md
legacy_specs: []
---

# Implementation Plan: Architecture maintenance and Jira/Linear root typing

## Overview

Reconcile the architecture-maintenance roadmap with verified work on current
main, then type the Jira and Linear issue-watch slices in sequence. The source
changes preserve state behavior and remove only the six existing root-store
cast entries. Jira follows the roadmap update; Linear follows Jira in the same
workspace.

The design checkpoint is based on main
`d7a44e96ede93d50c6277773f4174de716272f02` (2026-10-09). The measured
`ARCH-FRONTEND-ROOT-STATE-CAST` baseline is 45 entries. Expected totals are 42
after Jira and 39 after Linear, subject to a fresh measurement at execution
entry.

Implementation entry is current main
`64f8830d0d7b1ea98da96d3373d68bc0386cf49e` (2026-10-09). Re-measurement still
finds 45 entries, including exactly three Jira and three Linear identities.
The main-only delta since the design checkpoint touched unrelated plan
documentation; the reviewed source paths, root-state inventory, and workspace
lockfile are unchanged.

## Scope

### In scope

- Correct current status in the five named architecture-maintenance records
  using verified merged evidence, retaining old inventory dates and counts.
- Type Jira's recipe-only issue-watch slice and cover isolated defaults, real
  root composition, hydration, and unrelated root-state references.
- Type Linear's equivalent slice after Jira and add matching regression
  coverage.
- Remove only the three Jira and three Linear entries from the root-state-cast
  baseline.

### Out of scope

- Jira or Linear API, credentials, provider behavior, cache ownership, UI, or
  layout changes.
- Changes to Query ownership, Query lint rules, Office aliases, or unrelated
  root-state slices. The roadmap records already merged work only.
- New umbrella issues, scheduling automation, or edits/deletions to the July
  source audit files.
- Browser/E2E work. This package changes no rendered UI or mobile interaction.

## Contract coverage and gap

The new architecture-lint requirement and design define the narrow internal
compile-time contract for Jira and Linear. The existing Azure DevOps requirement
is limited to Azure DevOps, the Features plan is pattern evidence, and the
migrated System Query guard protects four different System snapshots. None of
those records is treated as Jira/Linear acceptance authority. There is no
generic guarantee for every Zustand slice; this package adds none.

Architecture-lint owns the new contract because it owns the existing root-store
cast rule and exact shrink-only baseline. Integrations continues to own
provider behavior. No ADR is needed because the change introduces no new
runtime boundary or meaningful architecture choice.

## Technical approach

### Roadmap reconciliation

Update only `docs/architecture-maintenance/README.md`,
`server-state-migrations.md`, `dependency-cleanup.md`, `lint-roadmap.md`, and
`historical-audit.md` for verified current-status corrections and this finite
increment. Record current main separately from the 2026-09-27 snapshot. Keep
unresolved candidates proposed/deferred with their existing design or evidence
gaps. After the PR opens, add its live URL to the Jira/Linear
implementation-complete, delivery-pending record. Do not describe either slice
as merged until merge evidence exists.

### Jira slice

Use the existing slice state and action signatures in
`apps/web/lib/state/slices/jira/`. Narrow the creator to its Immer recipe setter
and call it with root `set` only. Preserve the existing defaults, action
ordering, loaded/loading/reset behavior, hydration, and unrelated root-state
references. Remove exactly the three Jira entries from
`config/architecture-lint/frontend_root_state_cast.json`.

### Linear slice

After Jira is complete, apply the same bounded shape to
`apps/web/lib/state/slices/linear/`. Preserve its six actions and defaults,
cover real root composition and hydration, and remove exactly the three Linear
baseline entries. Do not refactor other slices or introduce shared typing
infrastructure.

If either slice cannot accept the recipe-only setter without a broader root
type or runtime change, stop and escalate before expanding scope.

## Tests

- `AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.2`: the Jira and Linear
  slice tests cover isolated defaults and all six action semantics; root-store
  tests cover actual composition, the existing initial-state merge, mutation
  isolation between stores, and unchanged unrelated references; hydration
  tests cover the existing `hydrateState` path without provider-specific routes.
- `.1` is verified by web typecheck and direct root creator calls without
  assertions.
- `.3` is verified by the architecture scan, root-state-cast rule test, and
  an exact comparison that permits only the current slice's three baseline
  removals.
- `.4` is verified by the linked merged evidence, dated/current snapshot
  distinction, and live PR delivery status in the roadmap records.

## E2E tests

None. The change has no rendered UI, user-visible behavior, or mobile surface.

## Work orders

- [x] [Task 01: Reconcile the architecture-maintenance roadmap](task-01-refresh-maintenance-roadmap.md)
- [x] [Task 02: Type the Jira issue-watch slice](task-02-type-jira-slice.md)
- [x] [Task 03: Type the Linear issue-watch slice](task-03-type-linear-slice.md)

## Verification results

Task 01 is complete. On current main
`64f8830d0d7b1ea98da96d3373d68bc0386cf49e`, the roadmap source snapshot was
measured at 45 root-state-cast entries, 29 compatibility registrations with
27 Office aliases, and four Query-owned System snapshots. The scoped catalog,
specification lint, and diff checks passed; details are in Task 01.
Task 02 is complete. The pinned baseline comparison against main
`64f8830d0d7b1ea98da96d3373d68bc0386cf49e` confirms 45 to 42 with only the
three Jira identities removed. The TDD compiler red was the direct creator
call requiring three arguments; the setter-only call passes after the change.
Focused Jira/root-store/hydration tests (55), web typecheck, five focused
architecture tests, the architecture scan, and `git diff --check` passed.
Task 03 is complete. The pinned current-main comparison confirms 45 to 39 with
exactly three Jira and three Linear identities removed. The combined focused
suite passed 66 tests across four files; web typecheck, full web lint, focused
architecture tests, the architecture scan, docs catalog/spec lint, and
`git diff --check` passed. The branch was fast-forwarded to current main
`3fed5570cec533f468c25ed03c84967bdc972588`; the intervening main paths are
disjoint and the exact baseline remains 45 to 39. Trusted-main PR-docs
preflight over all 19 actual changed paths returned `ok: true`, status
`covered`, and no errors. The latest docs catalog validated 368 decisions
and 1,498 specifications. The three scoped work orders are complete; hosted
delivery evidence remains pending until the combined PR is open and reviewed.

## Risks

- The branch may outlive the measured main snapshot. Re-fetch and re-measure
  before the first source change; report drift without deleting unrelated
  entries or forcing the old total.
- Shared repository state can include unrelated edits. Use actual worktree
  status and merge-base diffs, and preserve every unrelated change.
- A type incompatibility that needs whole-root typing or runtime behavior is an
  escalation, not permission to redesign the store.

## Open questions

None. The user supplied the order, exact source boundaries, preservation
requirements, and escalation condition.

## Final verification commands

Run the focused regression suites together after both slices:

```bash
(cd apps/web && pnpm exec vitest run lib/state/slices/jira/jira-slice.test.ts lib/state/slices/linear/linear-slice.test.ts lib/state/store.test.ts lib/state/hydration/hydrator.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
python3 -m unittest discover -s scripts/architecture_lint_tests -p 'test_frontend_root_state_cast.py'
python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The exact baseline delta is checked against the measured task base: Jira removes
only its three marker occurrences (45 to 42 at the design snapshot), then
Linear removes only its three (42 to 39). Re-measure and update these expected
totals if the task base changes.

Run the PR-documentation evaluator from the trusted main scripts over the
actual final changed-file set, with this plan, all three work orders, and the
new requirement/design supplied as file contents. The final result must be
`ok: true` with no errors. Do not use proposed source paths as a substitute for
actual changed files.
