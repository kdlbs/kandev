---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-FILE-TREE-PATH-COPY-001
system_design:
  - ../../specs/ui/system-design/file-tree-path-copy.md
legacy_specs: []
---

# Implementation Plan: File Tree Path Copy

## Overview

Add one node-scoped Copy path action to desktop and touch Files menus, including filename search. One sequential work order owns the complete slice, its targeted tests, and a small public guide update. The design is based on checkout and local `origin/main` at `fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58`; recheck the actual base before implementation and reconcile overlapping changes without adopting unrelated PR work.

## Scope

### In scope

- Exact `node.path` for a single file, folder, or filename result; existing clipboard guard and feedback; no primary row or filesystem side effects.
- Desktop context menu and the existing visible phone overflow menu, including bulk suppression and isolated-workspace browser evidence.
- Brief how-to addition to `docs/public/sessions-and-review.md` near Files workspace guidance.

### Out of scope

- Bulk path copying, new directory search, selection modes, menu redesign, clipboard API/fallback/concurrency changes, unrelated rename or data-attribute changes, and generic QA or broad local review.

## Technical approach

Add the localized action in `apps/web/components/task/file-context-menu-items.tsx` and account for it in the availability and separator logic in `file-context-menu.tsx`. Add the touch action in `file-browser-parts.tsx`; pass actual tree selection count to its touch menu even when delete is unavailable, and use single-result semantics for search. Both call `useCopyRepositoryPath` with `node.path` unchanged. Do not edit that hook or `copyPathToClipboard`. Inspect the current implementation at execution time because nearby rename extraction and data attributes can move independently. Keep row propagation and existing actions intact.

## UI composition

On desktop, a single tree node or filename search result exposes Copy path in its existing context menu alongside the available node actions. On phone, the same secondary action appears in the existing visible row overflow menu. The row remains the primary open or expand target. Existing grouping and separators are retained, with no fixed ordering beyond inclusion. The phone trigger and menu item have at least 44px touch targets; the shared responsive menu owns scrolling and safe-area treatment. Multi-selection omits Copy path. Unsafe paths show the existing refusal, and a failed clipboard write has no success claim. Rendered desktop and phone checks cover these choices and clipboard results (AC 001.1-001.6).

## Tests

In `apps/web/components/task/file-tree-path-copy.test.tsx`, first add focused failing tests for action absence, then implement and rerun: file and directory exact paths with spaces/Unicode and multi-root prefixes; search result exact identity; desktop and touch bulk suppression even without delete; event isolation and unchanged file callbacks; unsafe refusal and failed-copy behavior through the real shared hook/utility boundary. Existing `lib/utils/copy-repository-path.test.ts` covers the guard and safe bytes; do not duplicate its implementation. Keep tests meaningful at menu boundaries.

## E2E tests

Create `apps/web/e2e/tests/task/file-tree-path-copy.spec.ts` (`chromium`) and `apps/web/e2e/tests/task/mobile-file-tree-path-copy.spec.ts` (`mobile-chrome`), with their shared isolated fixture in `file-tree-path-copy-helpers.ts`. Use the real isolated backend, task workspace and routed Files UI as in `file-tree-chat-context.spec.ts` and `mobile-file-tree-chat-context.spec.ts`. Grant clipboard permissions and assert `navigator.clipboard.readText()` after desktop context-menu and phone visible overflow actions for a tree file, directory, and filename result; include a multi-root-prefixed path using the existing multi-repository task setup. Assert no file opens, no folder expansion or selection change, and unchanged fixture bytes. Phone checks trigger/item bounds, viewport containment and no document overflow, and captures or inspects the rendered menu. Component tests cover bulk and clipboard failure paths that are impractical to produce reliably through native clipboard access.

## Work orders

- [x] [Task 01: Copy node paths](task-01-copy-node-path.md)

## Verification results

Focused Vitest passed 60 tests across eight files; scoped ESLint passed. Real-backend Chromium and mobile-chrome flows each passed with actual clipboard reads; the final capture runs passed one test each. Specification and public-doc checks are recorded in the work order. The review correction also passed 24 focused tests, scoped ESLint, and the desktop and phone native E2E capture runs, including trusted keyboard access and causal search waits. Hosted validation remains external to this package.

## Risks

- Files paths can contain multi-root prefixes, unlike the existing Changes action; copying a displayed label or stripping the prefix changes identity.
- Touch bulk state cannot be inferred from the optional delete action. Existing failure behavior is silent after fallback failure, so tests must not require an invented toast.
- Nearby concurrent changes to rename and row attributes require integration against the then-current source, not wholesale file replacement.
