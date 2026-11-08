---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-COPY-COMMIT-ID-001
system_design:
  - ../../specs/ui/system-design/copy-full-commit-identifier.md
legacy_specs: []
---

# Implementation Plan: Copy Full Commit Identifier

## Overview and scope

Deliver the existing loaded commit header's explicit, source-neutral full-identifier copy action in one sequential work order. The [requirements](../../specs/ui/requirements/copy-full-commit-identifier.md) and [design](../../specs/ui/system-design/copy-full-commit-identifier.md) own the local UI action; repositories and providers retain data ownership. Existing file navigation and clipboard feedback contracts continue to apply. No commit rows, history, menus, index actions, backend, Git writes, new metadata surfaces or header visibility changes are included.

The requested scope and source are confirmed; source inspection verifies that `target.sha` already reaches `CommitHeaderMeta`. No material design choice remains unresolved. The separate, small button keeps the 575-line consumer edit minimal. Use the existing hook and labels; preserve the prior success deadline after failure and prevent old feedback from claiming success for a changed identifier.

## Technical approach

Add a small component under `apps/web/components/task/` and integrate it where `commitSha.slice(0, 7)` currently renders in `commit-detail-content.tsx`. Copy the exact current string via `useCopyToClipboard`, with accessible localized action and success feedback. Reuse the existing commit detail phone sheet and its visible index action as the mobile exemplar: one existing scroll owner, inline action, no new overlay, and no changed safe-area or navigation behavior. The inline control has 28px desktop geometry and at least 44px square on phone or coarse pointers. Preserve file index, diffs, metadata and source routing. Add one concise how-to to `docs/public/sessions-and-review.md` near its Commits paragraph.

## ASCII UI preview

UI-01: Desktop loaded detail (AC-UI-COPY-COMMIT-ID-001.1–.5).

```text
Commit message
Author · time · a1b2c3d [copy]    <- visible, keyboard-operable 28px action
Files (2)                         <- existing index and diffs
```

UI-02: Phone loaded detail (same criteria; existing full-height detail sheet).

```text
+----------------------------+
| Commit Changes       Close |
| Commit message             |
| Author · time              |
| a1b2c3d  [copy]            | <- visible, at least 44px square
| Files (2)                  |
| existing diff content      | <- existing single scroll owner
+----------------------------+
```

When copying succeeds, the action provides localized accessible copied feedback; on failure it remains available and makes no new success claim. The short identifier remains visible. Header absence, loading, empty and error behavior stay as today. Geometry is illustrative; ordering, discoverability, containment and measured target sizes are required. The narrow fine-pointer phone uses the phone target, while a coarse-pointer tablet also has a touch target.

## Tests

Extend the existing `apps/web/components/task/commit-detail-content.test.tsx` suite mounting the actual consumer. Preserve its existing regression cases, fixtures, assertions and test meaning; do not rewrite them to make them pass. With an identifier longer than seven characters, verify the visible short value and exact helper argument for local and GitHub targets, then rerender with a different identifier and verify the new exact value. Verify actual helper failure gives no new accessible success, permits retry, and successful copy gives accessible feedback; cover prior-success failure and target-change feedback association. New copy cases may stub unrelated expensive diff/provider dependencies as needed, but do not mock the copy hook or assert implementation classes. Before production edits, run the new mounted consumer cases and record a causal RED for the absent copy control.

## E2E tests

Create focused `apps/web/e2e/tests/git/commit-copy-identifier.spec.ts` (`chromium`) and `apps/web/e2e/tests/git/mobile-commit-copy-identifier.spec.ts` (`mobile-chrome`); use real isolated Git commit-detail fixtures and their returned full SHA. Explicitly set retries to 0 and worker count to 1; do not inherit the existing commit-navigation describe's retries=1. Desktop verifies actual clipboard contents, keyboard activation, 28px square geometry and captured synthetic detail. Phone verifies visible touch activation, exact clipboard contents, at least 44px square hitbox, containment and no document overflow; capture the real synthetic detail. A narrow fine-pointer phone case and coarse-pointer case measure the action, with focus and neighboring controls unobstructed. Map these to AC .1, .2, .4, .5; mounted tests cover .3 and source parity. Capture original desktop assets, manifest and hashes externally before the next managed capture clears `.pr-assets`, then do the same for phone assets.

## Work orders

- [x] [Task 01: Copy the current commit identifier](task-01-copy-commit-identifier.md)

Task 01 is sequential and has no dependencies. This design checkpoint authorizes no implementation. A later separate execution request and exclusive HEAVY release are required.

## Verification results

Task 01 passed the focused mounted suite (13 tests), touched-file lint, serialized typecheck, i18n checks, public-doc validation (63 tests, 54 pages), specification checks, and desktop (2) and mobile (1) E2E cases. The pre-change mounted and desktop/mobile E2E runs each failed on the absent copy button. Synthetic captures and manifests were preserved externally with hashes. See the work order for exact commands, actual session joins and logs.

## Risks

- Clipboard success can arrive asynchronously; feedback must correspond to the current identifier and failure must preserve the shared feedback contract.
- A nominal responsive class can still clip or create an undersized target; measure actual bounds and containment on all requested pointer/viewport cases.
- Managed capture clears `.pr-assets`; preserve each run's originals, manifest and hashes before the next capture.
- Public documentation has concurrent edits; preserve the whole existing paragraph and current main content, and reconcile actual overlaps before publication.
