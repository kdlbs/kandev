---
created: 2026-10-21
status: completed
requirements:
  - REQ-WORKSPACES-REPOSITORY-SAVE-001
system_design:
  - ../../specs/workspaces/system-design/workspace-settings-updates.md
legacy_specs: []
---

# Implementation Plan: Repository Save Acknowledgements

## Overview

Retain acknowledged repository and custom-script writes across failed multi-request
Save attempts. The user authorized bounded autonomous implementation, including
this design refinement, TDD, local validation and a normal commit before parent
validation. No delivery is authorized at this checkpoint.

## Scope

Repository settings page acknowledgement reconciliation, submitted versus newer
drafts, retry, error, Reset and late sibling settlement. Existing backend actions,
shared Save coordinator, cache ownership and desktop/phone layout remain in place.
No schema, security, durable journal or ambiguous-transport exactly-once redesign.

## Technical approach

The baseline at ce6ed5c971137a3c1d1360d75c111b51fde980d5 publishes responses only
when Promise.all succeeds. One rejected script discards sibling canonical IDs;
retry POSTs acknowledged scripts again. New-repository creation and deletion
batches also lose progress. Extract the narrow save orchestration from
workspace-repositories-client.tsx, settle all script operations, reconcile accepted
baseline and complete draft separately, then propagate failure. Skip clean scripts
on retry. Join pending page-level saves and wire repository discard to baseline.

Source discovery preceded PR inventory. PR #1512 changes this client's script
cache synchronization only; PR #4621 is disjoint. Recheck main and target patches
before delivery. No query-cache migration is included.

## UI-01: Existing Save recovery (desktop and phone)

```text
Repository card (existing scroll content)
  Build: acknowledged canonical script
  Lint: unsaved draft
Fixed shared Save surface
  Couldn't save    Reset    Save changes
```

Shared composition and controls are unchanged. Phone keeps its settings page,
44px touch actions and safe-area-aware save surface. The error must remain visible
until retry, and successful rows must not become duplicate resources. No markup,
layout or copy change is planned.

## Tests

`workspace-repositories-save.test.tsx` exercises the actual page hook, real actions,
fetchJson, cards and Save coordinator with only controlled fetch transport.
Causal RED: acknowledged Build plus definite Lint rejection, then Save retry must
retain Build's unique identity. Cover new/established repositories, clean/all-failure
controls, late settlements, newer edits/removal, partial deletes and Reset.
All map to AC-WORKSPACES-REPOSITORY-SAVE-001.1 through .4.

## E2E tests

`repository-save-retry.spec.ts` (chromium) and
`mobile-repository-save-retry.spec.ts` (mobile-chrome) prove failed Save, retry,
real persistence, uniqueness and reload using disposable Git repositories.

## Work orders

- [x] [Task 01: Reconcile acknowledged Save progress](task-01-reconcile-save.md)

## Verification results

Local implementation and validation are complete. Parent exact-diff validation
passed, and delivery was authorized. PR #4648 carries the change; final current-head
CI, semantic reviews and protected merge confirmation remain external delivery gates.

- Permanent causal RED on baseline: 10 failed / 5 passed. Failures include lost
  canonical identity, early sibling rejection, partial delete/update loss, Reset,
  and duplicate new repository creation. Log: `/tmp/attempt7-red.log`.
- A later regression for scripts added during new-repository Save was proved RED
  by restoring the prior owner reconciliation line, then restoring the fix. It
  failed on the temporary repository owner, as expected.
- Final targeted Vitest: 4 files, 70 tests passed (18 new save regressions).
- Managed fresh-build chromium E2E: 2 passed; sequential Pixel 5 E2E: 2 passed.
  Both used one worker, zero retries, disposable checkouts/records and real backend
  script persistence. Reload retained one repository and exactly Build and Lint;
  Build kept its canonical identity. No shared seed repository or user instance
  was modified. No task agents/executors were launched; all source edits were in
  this isolated worktree, and test disk writes were disposable fixture checkouts.
- Typecheck passed with Node 24 and an 8 GB heap cap; the default 4 GB run exhausted
  its heap. ESLint passed with zero warnings. i18n checks and the new-code ratchet
  passed; no product copy or catalogs changed.
- Specification catalog/lint passed. Public docs: 63 validator tests passed and
  54 published pages validated. PR documentation coverage passed using the
  repository validator with this work order, plan and owning requirement/design.
- Phone screenshot `/tmp/attempt7-phone-retry.png` was inspected. Existing settings
  composition is retained, with reachable controls and no horizontal page overflow.
- Initial new-repository E2E scaffold used a nonexistent name placeholder and
  timed out; corrected to the existing `my-repo` input. Final runs passed without
  test retries. No production changes were needed for that fixture correction.

Public docs updated: `docs/public/use-kandev.md` (how-to recovery guidance).
Exact successful commands are recorded in the work order.

## Risks

An ambiguous lost response can still represent an unacknowledged server write.
No claim extends beyond the mounted page or existing backend APIs. Repository ID
promotion changes contributor identity; test the actual coordinator at that boundary.
