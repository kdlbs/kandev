---
created: 2026-10-08
status: implemented
requirements:
  - REQ-UI-REVIEW-COMMENT-DELIVERY-001
system_design:
  - ../../specs/ui/system-design/review-comment-delivery.md
legacy_specs: []
---

# Implementation plan: Preserve review comments until delivery is confirmed

## Overview

Restore the Review retry path when Fix comments is rejected or transport is
unavailable. One sequential work order covers the actual shared mounted send
path, pending admission, selective acknowledged removal, and faithful rendered
regressions. The reviewed package was implemented after ROOT's later explicit
release in this same primary session.

Task `7f4d60cb-9de6-4151-8514-8b8443340a24`, sole primary session
`62e1b1e9-fa72-4310-87d0-2df5074684b8`, branch
`feature/preserve-review-comm-mfk`, source HEAD
`25d62239e990add8fe313722022f6476f4bf4c5a`.
ROOT reviewed all four design files and released implementation after design
END `b8cd1d61-bfc0-485e-a2ed-1a2933c0c922`. The global local-heavy lease was
NONE during design and exclusively granted to this task for implementation.

## Scope

### In scope

- Asynchronous delivery acknowledgement through the real hook/dialog/top bar.
- Local pending admission and existing button disabled state.
- Captured submitted row/task/session ownership and selective success clearing.
- Actual desktop mount and phone wrapper coverage, real store persistence,
  rejection/missing-client retry, pending edits/additions, and success controls.
- Minimal post-implementation reconciliation of the existing Review public
  guidance and review-file-comments design's superseded failure paragraph.

### Out of scope

Backend/API/schema changes, other send routes, global store action changes,
automatic retry, exactly-once claims, new draft/navigation coordination,
new layout/copy/touch controls, browser/build/full suites, runtime flags,
reviewer/CI policy changes, unrelated cleanup, and delegation.

## Technical approach

The [owning design](../../specs/ui/system-design/review-comment-delivery.md)
defines the boundaries. `useReviewDialog` owns admission, awaited `message.add`,
current-owner guard, selective `markCommentsSent`, and successful close.
`ReviewDialog`, ReviewDialogSurface, and ReviewTopBar propagate its promise and
pending state; they stop clearing/closing on invocation. FixCommentsButton
keeps its label and overview behavior and disables only while pending.
No global CommentsStore action or storage format changes.

| Consumer/transport shape | Behavior | Verification |
| --- | --- | --- |
| Desktop TaskReviewDialogMount / available WS | Wait for correlated request success, clear exact unchanged submitted rows, close if no review notes remain | Actual full mounted integration, including rejection/retry |
| Phone SessionMobileReviewDialog / same WS | Same settlement through shared mount; retain coarse-pointer overview-first activation | Actual phone wrapper, real button, coarse-pointer matchMedia fixture |
| Tablet TaskReviewDialogMount | Same shared contract without a separate adapter | Source call audit plus shared mount checks; no tablet geometry change |
| Missing client / missing owner / empty selection | Never acknowledge or remove notes; missing client gives existing error feedback | Actual absent-client regression and no message.add assertion |
| Rejected request or uncertain transport failure | Preserve current notes and storage; no automatic resend | Deferred error/rejection, exact retained data and error feedback |

Existing onSendComments production callers are all on this shared path. Existing
normal/passthrough/run-comment consumers keep their valid whole-ID store action
and their own send contracts. Routing, Markdown, UUID generation and 10000ms
message timeout remain unchanged.

## ASCII UI preview

UI-01: Existing Review, opened through the task's Review entry point.

```text
Desktop: existing sidebar | diff and notes
  Pending send: [Fix comments (2): disabled] [Close]
  Rejection:    [Fix comments (2): enabled]  [Close]  + existing error toast
  Success: unchanged submitted notes removed; close if none remain

Phone: existing diff and notes (sidebar hidden)
  Tap Fix comments -> existing overview -> tap again to send
  Pending send: [Fix comments (2): disabled] [Close]
  Rejection:    [Fix comments (2): enabled]  [Close]  + existing error toast
  New/edited notes after success: remain here with Fix comments enabled
```

Before the correction, both wrappers invoke a synchronous clear/close while
the request is pending. After it, acknowledgement governs removal and close.
Control order, fixed header, diff scroll owner, spacing, touch geometry, labels,
and dismissal remain existing production presentation. These annotations depict
behavior, not new copy or a pixel specification. UI-01 maps to all `.1` through
`.9` criteria and the rendered delivery suite.

## Tests

Author `apps/web/components/task/dockview-review-dialog.delivery.test.tsx`
independently after release. Run the following named scenarios through actual
TaskReviewDialogMount and actual SessionMobileReviewDialog:

| Scenario name | AC suffixes |
| --- | --- |
| retains exact file and line notes on rejection and acknowledges a deliberate retry | `.1`, `.4`, `.5`, `.6`, `.8` |
| unavailable transport preserves persisted feedback and allows a later send | `.2`, `.5`, `.6`, `.8` |
| deferred delivery retains notes and admits only one rapid or reopened send | `.3`, `.8` |
| acknowledged unchanged feedback clears exactly the submitted IDs and closes | `.4`, `.5`, `.7`, `.8` |
| acknowledgement preserves edited text, new notes and unrelated sources/sessions | `.4`, `.5`, `.7`, `.8` |
| acknowledgement preserves changed repository and anchor metadata under the same ID | `.4`, `.5`, `.8` |
| deleted submitted feedback stays deleted while other notes survive | `.4`, `.7` |
| a previous session acknowledgement does not close or clear the current Review | `.4`, `.5`, `.7`, `.8` |
| explicit dismissal and rejected uncertain transport never reopen or automatically resend | `.3`, `.7`, `.9` |

Compare a fixed independently authored wire Markdown expectation, captured
task/session, UUID-shaped client_message_id, message.add count, real store
indexes and reached persistence assertions. Keep actual acknowledgement success
controls; no helper predicate-only or mocked send-hook tests. Existing
use-review-dialog, review-file and format suites are scoped compatibility
controls. Fixtures mock only external WS/fetch, not first-party providers,
stores, components, formatters or internal adapters. Required auxiliary requests
must have explicit transport fixtures; unexpected calls fail by name.

## E2E tests

The mobile-parity pure state/data exception is justified in the owning design:
this package changes local admission/acknowledgement inside the existing shared
control, without layout, copy, touch gestures, scrolling, navigation or
breakpoint changes. End-to-end consumer evidence is the actual full rendered
Review send-button composition through both first-party mounts with only
external transport replaced. No new Playwright file or browser/build resources
are authorized. Presentation changes would invalidate this exception and
require a ROOT scope/resource checkpoint before proceeding.

## Work orders

- [x] [Task 01: Confirm Review delivery before removing feedback](task-01-confirm-review-delivery.md)

Exactly one work order, wave 1, no dependencies; sequential in this primary.

## Verification results

Design-only gates passed on 2026-10-08:

- `python3 scripts/list-docs.py validate`: exit 0, 364 decisions and 1472 specs.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all specifications passed.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 validator tests passed.
- Real `.github/scripts/pr-docs.cjs` prospective preflight: exit 0, `covered`,
  errors `[]`, all six projected production paths and the single work order.
- Documentation diff and untracked-file whitespace/newline checks passed;
  index empty and only these four design artifacts are untracked.

Native receipts: `/tmp/kandev-child87-design-receipts-20261008/` and the durable
task plan. All commands returned terminal native exits with no session handles;
no product test/install/lint/typecheck/build/browser/hooks/commit ran. The
validator tests above are dependency-free documentation checks, not product
regressions. Those design results were not implementation authorization. The
later release, implementation checks and reconciliation are recorded below.

Implementation gates passed on 2026-10-08:

- One pnpm 9.15.9 frozen install, Node 24.21.0, no lockfile change.
- Full mounted desktop/phone RED reached actual store and persistence loss on
  rejection and missing client. Original RED02 exit 1: 14 failed, 4 passed,
  including acknowledged success controls. RED01's missing state seeding was
  a noncausal fixture failure and is not counted RED.
- GREEN02: 44 passed, 6 failed only in external error-reporting fetch cleanup.
  After transport fixture correction, GREEN03 passed those six cases.
  Typecheck found three fixture-only type errors; after actual-action seeding
  and DOM cleanup repair, GREEN04 passed all 18 mounted scenarios.
  The 32 unchanged controls passed in GREEN02 and were not replayed.
  No protected ROOT proof code was copied or executed.
- ESLint passed six production modules and both test files; affected helper
  lint also passed. Full typecheck passed after fixture repair.
- `i18n:check` and `i18n:ratchet` passed. Catalog orphan warnings are
  informational; no new localized strings or presentation changes were added.
- Catalog and specification lint passed; 36 specification-validator and
  62 public-doc-validator tests passed; 47 published pages validated.
- Real coverage on all 14 actual changed paths passed with status `covered`,
  errors `[]`; `git diff --check` passed.
- Public docs updated: `docs/public/sessions-and-review.md`, a how-to guide,
  replaces its loss warning with acknowledged retention and deliberate retry.
  The older file-comment design links to the new settlement contract.

Original native starts, sessions, terminal chunks/exits, complete raw streams,
wrapper/group/start identities and wait/reap evidence are retained in
`/tmp/kandev-child87-receipts-20261008/` and the durable platform plan. GREEN01
failed before spawning a child because its wrapper expected numeric `180`,
not `180s`; that invocation error is not a test result. Its actual exit and
ROOT checkpoint were preserved before correcting the argument. Passing checks
were replayed only for affected fixture/status changes or required hooks.

The first normal commit attempt stopped at `i18n-new-code`: after staging,
the hook found two synthetic feedback strings in the new test helper that the
unstaged ratchet had skipped. Documented, reasoned exemptions identify only
those test data values. No hook or detector was bypassed or modified; affected
lint/ratchet checks and a normal new commit attempt follow this correction.

Implementation completion does not authorize merge. Normal commit/push/ready
PR and one frozen-head hosted observer follow the standing delivery gates;
ROOT retains the separate serial merge grant and final verification.

## Risks

- Any remaining synchronous clear or close in the actual consumer chain would
  lose notes or hide retry; full first-party composition must expose it.
- ID-only clearing can erase edited metadata/text; match actual submitted row
  identity before the existing deletion action.
- Full Review providers issue auxiliary reads; missing-provider or unhandled
  transport fixture failures are not causal RED.
- A timeout does not establish whether the server accepted feedback. Preserve
  notes and avoid auto-retry or exactly-once/rollback claims.
- Late settlement must not close a different session, and explicit dismissal
  must never be reversed.
