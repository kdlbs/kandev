---
created: 2026-10-17
status: completed
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
system_design:
  - ../../specs/office/system-design/automations-settings-02.md
legacy_specs: []
---

# Implementation Plan: GitHub PR event selection

## Overview

Honor the saved Opened selection before GitHub PR polling can search or fire.
One sequential work order adds independent regression evidence, gates the real
evaluator, verifies existing editor output, and corrects the public limitation.
ROOT reviewed this package and later explicitly released implementation in the
same primary session on 2026-10-17. Task 01 is now complete.

## Evidence and cause

`GitHubEvaluator.checkPRTrigger` decodes `GitHubPRTriggerConfig.Events` but
never consults it. An editor-cleared `events: []` therefore still reaches
`ListAutomationPRs` and `Service.FireTrigger`. The supplied qualification used
real stores, credential routing/provider adapter, evaluator, firing service and
subscriber with only external HTTP transport replaced: empty events produced
one search and one event; explicit opened produced one event; opened with draft
exclusion and a draft PR produced none. This is synthetic transport evidence,
not a live GitHub or agent-launch claim.

ROOT receipts live under
`/tmp/kandev-root-pr-event-discovery-20261014-32mmpj23/`:
`qualified-proof.json`, `qualification-receipt.json`, `qualification.log`,
`current-main-check.json`, and `post-4496-main-check.json`. Do not read, copy,
replay, or delete the private candidate/source files. Author permanent tests
from current repository patterns and the observable contract.

Recovery on 2026-10-17 found clean local HEAD
`cbfc1e7665661367175ae69f1c3e9d560deb6abd` and authoritative remote main
`b0b7ece665120b2ad63ff76e093566125e52ffde`. All five implicated file blobs and
the existing settings requirement/design parts are identical between these
commits. The five local hashes also match the qualification/post-4496 receipts.
Qualification was not replayed. Main drift alone does not require a rebase.

## Scope

### In scope

- AC-OFFICE-AUTOMATIONS-SETTINGS-001.19 in the existing settings owner pair.
- Exact existential selected-opened eligibility immediately after config decode.
- Independent real backend regression and real existing-checkbox evidence.
- Minimal public paragraph correction after the behavior is fixed.

### Out of scope

- New event kinds, All repos, labels, hydration, pagination, provider/auth policy.
- Schema/migration, SDK/API, retry, concurrency, cursor, scheduler, lifecycle.
- Production frontend/layout/copy, screenshots, README polish or global cleanup.
- Delegates, extra tasks/sessions, unrequested audits or merge authorization.

## Technical approach

The [existing settings design](../../specs/office/system-design/automations-settings-02.md#github-pr-event-selection)
owns the correction; automation-runs remains the reading/watching consumer.
Part 1 is 32,722 bytes against a 32,768-byte limit, so the section fits in the
existing part 2 rather than creating a new repair specification or rewriting
unrelated legacy text. Add only one criterion to the existing requirement.

In `apps/backend/internal/automation/evaluator.go`, return after decoding when
no exact `opened` entry exists. Preserve existing subsequent operations. New
picker defaults explicitly contain opened; saved missing, null, empty, whole
JSON null, or unsupported-only config never receives that default at evaluation.
Mixed selection containing opened is eligible regardless of position.

| Boundary | Identity/transport | Outcome and evidence | Unsupported shape |
| --- | --- | --- | --- |
| GitHub PR polling | Existing workspace automation credentials and provider adapter; synthetic HTTP transport in tests | Selected opened reaches real search/firing; unselected never searches | Unsupported-only events return; no additional event handling |
| Editor/save | Real translated checkbox; existing draft transport and real backend JSON store | Clear emits/persists empty selection; select emits/persists opened | Missing/null displays unchecked; no default injection |

## Tests

All map to AC-OFFICE-AUTOMATIONS-SETTINGS-001.19:

- `apps/backend/internal/automation/evaluator_pr_events_test.go`:
  `TestGitHubPREventSelection` covers cleared, missing, null, whole-config null,
  unsupported-only, explicit default, and mixed selection with opened last.
  Matching repository/PR fixtures make the unchecked cases causally decisive.
- `TestGitHubPREventSelectionControls` covers disabled automation/trigger,
  missing/foreign ownership, no explicit repos, branch/author mismatch, and
  draft exclusion. Eligible positive controls prevent an unconditional return.
- `TestGitHubPREventSelectionPersistenceAndDedup` saves and reloads config
  through real service/store paths, confirms cleared polling reserves nothing,
  reselects opened for the same unseen PR, and verifies one firing after repeat
  evaluation. Check zero negative-case provider searches, events, admissions/run
  rows, dedup bindings, last-triggered and evaluation-cursor effects.
- `apps/web/components/automations/trigger-configs/github-pr-config.test.tsx`:
  real `GitHubPRConfig`, translated accessible checkbox, clear/select callbacks
  and rerendered state; missing/null unchecked and mixed membership controls.
  Keep unrelated provider-selector fixtures narrow and use the real checkbox.
- Existing `automation-trigger-drafts.test.ts` verifies forwarding complete
  config through the editor save path. Do not alter save semantics.

## End-to-end evidence and mobile assessment

The joined evidence covers actual checkbox output, saved JSON, provider
boundary and emitted automation event. No agent runtime is needed to establish
eligibility; absence of admission is checked in the real run store. There is
no browser/Playwright work item: this is backend/data behavior with unchanged
desktop/phone rendering, controls, touch targets, navigation, copy and scroll
ownership. Component evidence satisfies the mobile-parity data/state exception.
No ASCII UI preview is applicable.

## Work orders

- [x] [Task 01: Honor Opened selection](task-01-honor-opened-selection.md)

Task 01 ran sequentially under one ROOT-granted global heavy lease. The work
order records exact RED/GREEN, package, frontend and docs checks. Publication
and hosted checks follow through normal hooks; merging still requires a later
ROOT grant.

## Verification results

Design checkpoint completed on 2026-10-17:

- `python3 scripts/list-docs.py validate`: exit 0; 383 decisions and 1,613
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all files passed.
- Node v24.21.0 `validateCoverage` local dry-run (command in the work order):
  actual four-file Markdown diff `ok=true`, `status=exempt`, `errors=[]`;
  separate planned-evaluator input `ok=true`, `status=covered`, `errors=[]`,
  with the one work order and settings design accepted. The planned path was
  validator input only; no runtime file was changed and no status was posted.
- `git diff --check` and untracked work-order inventory: clean; four artifacts
  present, unstaged and uncommitted.

Implementation completed after the later explicit ROOT release:

- Frozen `pnpm install --frozen-lockfile` in `apps/`: exit 0, once because
  dependencies were absent; Node 24.21.0 and pnpm 9.15.9. No lockfile changes.
- Independent causal RED: exit 1, expected search1 versus search0 for cleared,
  missing, null, unsupported-only and differently cased selections. Explicit
  opened/mixed selections and all 11 filter/ownership controls passed before
  the fix. Final GREEN: exit 0, package 1.593s; all three top-level regression
  tests passed, including persistence/reselection and repeated-PR dedup.
- Owning package command from Task 01: exit 0, package 9.697s, race enabled.
- Canonical changed-Go complete-package lint: exit 0, zero issues, using the
  documented GOMAX2/GOMEM3GiB/internal5m/outer360kill10 limits.
- Focused Vitest command: exit 0, 2 files and 10 tests passed. Scoped ESLint:
  exit 0, zero warnings/errors after correcting the duplicate ARIA-attribute
  literal required by the normal zero-warning commit hook.
- Public-doc validator tests: exit 0, 63 passed. Public-doc validator: exit 0,
  48 published pages validated. Catalog and all-spec lint: exit 0, unchanged
  383 decisions/1,613 specifications, all files passed.
- Node 24 actual changed-file coverage dry-run: `ok=true`, `status=covered`,
  `errors=[]`; one work order and the owning settings design accepted.
- Normal staged `(cd apps/web && pnpm run i18n:ratchet)`: exit 0, no UI source
  changes, guard allowlist intact (643 entries). `git diff --check`: clean.

Setup/fixture corrections are separate from causal evidence: the initial Go
compile reached the 180-second timeout without test output; a subsequent
fixture assertion incorrectly expected a successful firing to leave its cursor
unchanged. The decisive RED followed that correction. Initial GREEN then
exposed an incorrect duplicate-history expectation; the existing PR unique
constraint leaves one row, and only the test expectation changed. An initial
ratchet invocation from the repository root failed before running the check;
the documented web-directory command passed. All original native handles were
joined; tracked Go process groups were confirmed absent on completion.

No production frontend, model, registry, save, provider, schema or lifecycle
changes were made. Typecheck, Playwright and screenshots are inapplicable to
this backend correction with test-only frontend changes. Fixtures replace HTTP
transport and do not prove live GitHub/account or agent execution. The initially deferred duplicate-string warning became a required correction
when the normal commit hook enforced zero warnings. It was corrected before
publication; no optional cleanup or extra push was added.

## Risks

- Existing configs that omitted/nullified events previously fired accidentally;
  they now remain inactive until opened is explicitly selected, matching the UI.
- Fixtures must supply real routing and a matching repo/PR so zero effects
  cannot be explained by auth, ownership or filter setup failure.
- Search response branch/draft fields need the existing adapter's actual shape;
  do not change hydration or client behavior to make fixtures pass.
- Design validation does not prove runtime behavior. ROOT's later implementation
  release and a freshly observed independent RED/GREEN remain required.
