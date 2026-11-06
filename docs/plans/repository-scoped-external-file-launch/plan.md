---
created: 2026-10-27
status: implemented
requirements:
  - REQ-UI-EXTERNAL-FILE-TARGET-001
system_design:
  - ../../specs/ui/system-design/repository-scoped-external-file-launch.md
legacy_specs: []
---

# Implementation Plan: Repository-scoped external file launch

## Overview

Thread each file action's repository identity into the existing external-editor request, resolving it against the selected session's worktree associations. First prove the observed failure through an independently authored rendered toolbar test; then implement the bounded resolution and verify every shared caller. There is one sequential work order and no schema or backend change.

The existing embedded-target pair covers a different downstream embedded dispatch and explicitly leaves external behavior unchanged. This new bounded UI pair owns the shared external file handoff; Workspaces and Tasks retain path and session authority.

## Scope

In scope: Review desktop and phone, Monaco desktop and overflow, CodeMirror overflow, external-editor file requests, regression tests, and the owning user documentation. Out of scope: folder and copy-path behavior, embedded dispatch/code-server, file-tree resolution, native CLI behavior, SSH escaping, containment, repository attachment/recovery, cache/runtime inventory, permissions, layout and copy changes.

## Technical approach

Follow the exact identity, placement, default, and unavailable cases in the [system design](../../specs/ui/system-design/repository-scoped-external-file-launch.md). Review's ID is file-specific; the editor panel's ID is the active session's primary and must not override a named file scope. Add a session-domain hook and only the small pure resolver it needs; reuse `useSessionWorktrees` and session selectors. Forward context into `file-actions-dropdown.tsx`, applying the result only to external editor items. `openSessionInEditor` already accepts `worktree_id`. The backend checks worktree or association IDs and defaults omitted IDs to the first worktree or repository checkout.

A unique file-specific Review ID selects its associated worktree even without workspace placement metadata. If a name independently resolves uniquely to a different worktree, disable the action; missing or ambiguous redundant name evidence is not a contradiction. A name-only scope that does not map to exactly one observed worktree, including unproved nested scopes, is disabled. There is no speculative path alias or tree-root inference. Unscoped legacy/single-repository default remains available under the design's evidence limits. The user's qualification shows secondary routing fails and the primary control passes at the HTTP-domain boundary; it does not prove a native launch.

## UI-01: Existing file actions

The menus and copy remain unchanged. The only changed rendered state is the existing editor item becoming disabled when the target cannot be established; no new explanation copy is added. These sketches show structure, not dimensions. AC 001.1-001.5.

```text
Desktop Review: file header -> Open with
  VS Code                 [enabled if target resolved]
  ... existing items ...

Phone Review: file header -> More actions
  ... existing actions ...
  VS Code                 [disabled if target unknown]
  ... existing items ...
```

Phone retains its existing overflow and scrolling behavior; desktop editor and compact editor overflow retain their existing controls. Verify real rendered desktop and phone menu selection, enabled and disabled states, and actual opener call in HappyDOM. This is a target-data and disabled-state correction with no new mobile layout, navigation, touch, or scroll behavior; the mobile-parity data exception applies. No new mobile Playwright scenario is planned, and no Chromium result is inferred from HappyDOM.

## Tests and E2E evidence

The new `components/review/review-diff-toolbar.external-editor.test.tsx` proves AC 001.1, 001.3-001.5 through actual rendered menu, real hook/request/store and HTTP-domain stub, including phone. Capture secondary RED and primary PASS before production using two explicit associations, a legacy primary `worktree_path` and no `workspace_path`; also cover file-specific ID without a name and legacy primary controls. New `components/editors/file-actions-repository-target.test.tsx` covers the actual Monaco desktop, Monaco overflow and CodeMirror overflow consumers and the external/embedded/folder/copy separation (AC 001.2-001.5). A new domain resolver suite covers exact identity, name placement, ambiguity and legacy/default matrix (AC 001.1-001.4). Test names are specified in the work order. Tests must be independently authored; no qualification fixture access.

Existing `review-diff-toolbar.test.tsx`, `monaco-editor-toolbar.test.tsx`, and both `use-open-session-in-editor` suites are reviewed `browser` project tests. New unreviewed suites belong to `browser-locales`, also HappyDOM. Each project run must actually collect tests and retain its native process handle, exit and counts. The existing E2E `apps/web/e2e/tests/settings/vscode-open-file.spec.ts` exercises the editor action, but does not establish the multi-repository target; targeted rendered boundary tests provide this correction's evidence under the data exception. No native editor process or browser end-to-end outcome is claimed.

## Bounded legacy review follow-up

A source-supported legacy state can leave one live worktree without a repository ID while the session's primary repository ID is available to the unscoped CodeMirror overflow. The current resolver disables that external action. The [system design](../../specs/ui/system-design/repository-scoped-external-file-launch.md#legacy-single-worktree-follow-up) pins the exact existing-default fallback and exclusions; the existing work order owns the independently authored RED and focused GREEN checks. This is a data-only correction to the existing menu, so the earlier responsive capture and mobile parity assessment remain applicable. No new screenshot or capture build is required. The follow-up was implemented after separate authorization; the causal RED, focused verification and delivery record are in the existing work order.

## Public documentation

`docs/public/developer-tools.md` is a how-to/reference for editor use and already describes **Open with**. Update that paragraph during implementation to explain repository-scoped external file selection and that unavailable/ambiguous targets disable the external action; do not imply embedded runtime root mapping is fixed. Review `docs/public/sessions-and-review.md`, root `README.md`, `docs/screenshots.md`, relevant specs and decisions for contradictions. No screenshot or new page is required.

## Work orders

- [x] [Task 01: Route external file actions to their repository](task-01-route-file-actions.md)

## Verification results

Task 01 completed: 14 focused new and 43 existing tests passed in HappyDOM, scoped ESLint and web typecheck passed, and staged ratchets, documentation coverage, catalog, specification and public documentation checks passed. One-worker mobile-chrome capture documented desktop-width and phone-width Review menus after correcting its disposable navigation fixture; it does not prove native editor launch. See the work order for the RED, counts and limits.

## Risks

For name-only targets, session placement may be unavailable or differ from the runtime scope, in which case the conservative disabled state reduces availability. Session-primary IDs from editor panels cannot be treated as file IDs. Nested submodule names and attached folders are intentionally not guessed. HappyDOM cannot establish native editor launch or Chromium behavior. External-only gating must not alter embedded behavior.
