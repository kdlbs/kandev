---
created: 2026-10-22
status: done
requirements:
  - REQ-INTEGRATIONS-SENTRY-FORM-LIFETIME-001
system_design:
  - ../../specs/integrations/system-design/sentry-instance-form-lifetime.md
legacy_specs: []
---

# Implementation Plan: Sentry Instance Form Ownership

## Overview

Preserve the current Sentry opening when an earlier accepted write settles.
The user explicitly authorized package creation followed by bounded TDD in one
session. Stop after a clean local commit for parent validation, before delivery.

## Scope

Only Sentry add/edit opening ownership and mounted contributor identity, focused
regressions and this vertical package. Exclude other integrations, shared
coordinator/Query/backend redesign and task rename.

## Technical approach

The confirmed defect is unconditional section closure from captured `onSaved`.
Use fresh local opening identity, mounted completion checks and guarded callbacks;
retain refresh
and truthful toasts. Distinguish mounted contributors to preserve same-text
replacement dirty/failure ownership. Form-local testing/busy/result producers
remain isolated by mount. Use real providers/coordinator/API with only fetch held.

## ASCII UI preview

UI-01: Settings > Sentry, after cancelling a pending save and opening a new form.
Shared desktop/phone structure; the phone uses the existing dedicated settings
page, single content scroller, and fixed safe-area-aware Save surface.

```text
Before older ACK: [New instance: name | URL | token] [Test] [Cancel]
Current defect:   [instance cards]                  (new form disappears)
After repair:    [New instance: name | URL | token] [Test] [Cancel]
Fixed viewport:                         [Reset] [Save changes]
```

The fields, selection and dirty state survive. Spacing is illustrative; no new
layout, copy or touch geometry is proposed. Maps to AC .1 and .4 and the focused
consumer/browser checks below.

## Tests

`sentry-form-lifetime.test.tsx` owns AC .1-.3: old create/edit success/failure
against add/other-edit/same-edit, identical drafts, next save/cancel, current
success/error/retry, unmount/remount, and test-result isolation. Existing form
and section suites protect established controls. Only admitted flows are tested;
the coordinator refuses a second save while the first is pending, so concurrent
reverse save completion is not admitted.

## E2E tests

`sentry-form-lifetime.spec.ts` (chromium) and
`mobile-sentry-form-lifetime.spec.ts` (mobile-chrome) exercise original persisted
create/update, cancel/reopen/new exact fields and selection, older ACK, and next
Save/Cancel. Use owned disposable fixtures and mock Sentry, never an operator
runtime or real token. Component RED supplies causal proof before browser GREEN.

## Work orders

- [x] [Task 01: Preserve current form](task-01-preserve-current-form.md)

## Verification strategy

Install frozen dependencies under Node 24. Run the work-order commands, normal
pre-commit and commit-msg hooks, then refresh main and paginated target patches.
Retain external receipts and every process handle to terminal. No push/PR yet.

## Verification results

- Frozen pnpm 9.15.9 install under Node 24.18.0: passed.
- Causal initial RED: 10 intended failures, 7 passing controls. The failures
  expose replacement closure, reused failure state and identical-text ownership.
- Poll-remount RED: one intended failure after list removal, restoration and new
  field entry; the old ACK then removes the remounted form. Earlier fixture-only
  errors are separate from this behavioral evidence.
- Final focused Vitest: 3 files, 35 tests passed (23 lifetime cases and 12 existing
  form/section controls). Covers success/failure, same-instance and identical-text
  reopen, unmount/remount, polling remount, next Save/Cancel and stale test results.
- Desktop production browser: 2 passed, 1 worker, zero retries, fresh build.
  Native-tap emulated Pixel 5: 2 passed, 1 worker, zero retries, same fresh assets.
  Both use the real backend with mock Sentry, hold only an accepted write's
  response, assert backend persistence and exact newer fields/selection, and
  exercise next Save or Cancel. No real provider or operator data was used.
- Changed-file ESLint: passed with zero warnings. Web typecheck with 8 GiB heap
  and rich-output catalog check: passed. i18n check and new-code ratchet: passed.
- Documentation catalog: passed (396 decisions, 1705 specifications).
  Specification-linter tests: 36 passed. Full specification lint: passed.
  Public-doc validator tests: 63 passed. Public docs: 54 pages validated.
  PR documentation coverage: covered with the new owning requirement/design.
- Public documentation: clarified submitted-write cancellation under Sentry in
  `docs/public/integrations.md` (existing integration how-to/reference page).
- External receipts: `../loop-receipts/attempt25/` outside the repository.
  Normal commit-hook, final HEAD/diff and terminal-process evidence is recorded
  there at the parent-validation checkpoint, before any push or PR.
- Refreshed main: `5aa31bec1a8a5e9cdb679811b02aa3e7b78271c2`, one unrelated
  agent-discovery commit beyond tested base `f61b11093c7f75465e64b7ad8cb78a9d9910b95b`.
  No Sentry or settings-coordinator source changed between those heads.
  Refreshed all 69 open PRs and paginated actual changed files: only #1512 touches
  these source targets, at `a8655ad92faa050a68140a904500fc99ebc0a04b`; it retains
  unconditional closure and does not change the instance form. This is a bounded
  target-overlap finding, not a correctness review of unrelated PRs.
- Limits: phone behavior is browser device emulation; no physical device,
  OS keyboard, real provider, cross-tab or transport exactly-once claim.
  Browser screenshot capture was disabled; behavior and selection assertions
  ran on the shipped production surface. No rendered layout changed.


## Risks

An identical replacement can fool text-only checks. Old errors must still be
reported without tagging the replacement. Phone evidence is browser emulation.

### Save availability during list refresh

Greptile identified that awaiting the post-write list GET held the shared Save
action busy for a replacement draft. A causal real-consumer test holds that GET
after the accepted write succeeds: RED leaves the current action in Saving;
the one-line correction starts the existing error-handled refresh independently.
The focused suites pass 35/35, including this pending-refresh control. Browser,
static, native CI and exact-head review results are recorded in the external
delivery receipts after the correction. No shared coordinator behavior changed.
