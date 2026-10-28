---
created: 2026-10-27
status: implemented
requirements:
  - REQ-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001
system_design:
  - ../../specs/integrations/system-design/review-thread-context.md
legacy_specs: []
---

# Implementation Plan: Review thread context

## Overview and scope

Deliver one sequential host UI work order: resolve context from the complete
supplied thread while preserving the existing author-filtered presentation and
provider interfaces. The requirement and design are active/current following implementation
and targeted checks. No plugin implementation, transport, new nesting,
new copy, or broader comment filtering is in scope.

## Technical approach

In `change-request-detail-comments.tsx`, derive a complete thread association
from `detail.comments` in addition to the existing filtered display projections.
Map each displayed fragment root to its supplied full thread; use that thread
for whole-thread action availability and formatting. Keep fragment identities
for reply actions and preserve all other actions. Make the existing whole-thread
button keyboard-named and touch-sized in its own context without expanding
unrelated desktop controls. See the [system design](../../specs/integrations/system-design/review-thread-context.md)
for direct-parent, missing-parent, callback, and ordering boundaries.

| Consumer and shape | Intended behavior | Evidence / fallback |
| --- | --- | --- |
| GitHub feedback via existing HTTP model | Existing `in_reply_to`/`author_is_bot` mapping supplies direct parent and author type | Rendered component tests and seeded PR detail E2E; missing parent becomes root |
| Compatible plugin host detail model | Same supplied `id`, optional `parentId`, `author.isBot`, and `onAddContext` contract | Shared component tests; no plugin or transport changes |
| Missing parent or no callback | Orphan is a root; no callback means no action | Component test; no fetching or invented parent |

## ASCII UI preview

UI-01: Comments section, mixed two-comment thread, bot disclosure closed.
The shared composition applies on desktop and phone; the phone has the same
single scrolling Review detail and a visible touch-sized action. The identity
and visual grouping below are structural; spacing is illustrative. The plus
is the existing whole-thread action. AC-001.1, .2, .4 and .5 apply.

```text
Comments (2)                       [add all]
  human root: Please update         [+ thread]
> Bot comments (1)
```

Activating `+ thread` adds both the human root and hidden bot reply to pending
context. If the bot is the root, its visible human reply remains in the human
section and its action includes the bot root. Expanding the bot disclosure
keeps the existing bot rows and can expose another action for the same thread.
The phone action target is at least 44 x 44 pixels; desktop remains compact.
No new copy, surface, scroll owner or navigation is proposed.

## Tests and E2E tests

- `components/integrations/change-request-detail.test.tsx`: rendered actions
  exercise both mixed root types, mixed siblings and exactly-once content,
  supplied order, metadata/PR URL, two-comment threads, homogeneous controls,
  parent missing, no callback, display visibility, per-comment/all-comment
  actions and thread reply identity (AC-001.1–.4).
- `e2e/tests/pr/pr-detail-thread-context.spec.ts` (`chromium`) and
  `e2e/tests/pr/mobile-pr-detail-thread-context.spec.ts` (`mobile-chrome`):
  seed a real mixed thread, enter the existing Review detail, activate the
  action, and inspect the resulting pending context, including both bodies
  (AC-001.1, .2, .5). Phone coverage asserts the actual touch hitbox and no
  horizontal document overflow. Use existing mock feedback and native UI;
  screenshots for a later ready PR use fresh managed `prCapture` assets.

## Work orders

- [x] [Task 01: Complete supplied thread context](task-01-complete-thread-context.md)

## Verification results

Rendered component Vitest: 19 passed. Scoped ESLint passed. Canonical frontend
typecheck passed with a 6144 MiB Node heap after the default 4 GiB heap failed
with OOM. Focused fresh managed desktop and mobile Playwright each passed with
one worker, retries disabled and trace enabled. Specification catalog and lint
validation passed.

## Risks

- A visible cross-group fragment may identify a reply rather than the provider
  root. Context must use the complete parent thread while reply callbacks retain
  their existing fragment identity.
- Mobile's current shared add button is 24 pixels; the scoped touch target needs
  rendered geometry evidence and must not enlarge unrelated desktop controls.
- Mock feedback must preserve bot and reply fields through the actual UI path;
  assert the pending context itself instead of relying on a toast.
