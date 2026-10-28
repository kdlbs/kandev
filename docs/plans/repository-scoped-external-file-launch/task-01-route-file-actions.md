---
id: "01-route-file-actions"
title: "Route external file actions to their repository"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-EXTERNAL-FILE-TARGET-001
acceptance_criteria:
  - AC-UI-EXTERNAL-FILE-TARGET-001.1
  - AC-UI-EXTERNAL-FILE-TARGET-001.2
  - AC-UI-EXTERNAL-FILE-TARGET-001.3
  - AC-UI-EXTERNAL-FILE-TARGET-001.4
  - AC-UI-EXTERNAL-FILE-TARGET-001.5
system_design:
  - ../../specs/ui/system-design/repository-scoped-external-file-launch.md
---

# Task 01: Route external file actions to their repository

## Summary

Prove the current secondary-file misroute at the rendered Review menu and HTTP-domain boundary, then route external file actions using the correct session worktree. Preserve the existing legacy default only where the design permits it and disable unresolvable targets.

## In scope

- Review desktop and phone, Monaco desktop/overflow, CodeMirror overflow, shared file actions and session-domain target resolution.
- Independently authored real-caller regression and focused identity/compatibility tests; the editor paragraph in public developer-tools documentation.

## Out of scope

File-tree actions, folder and copy-path routing, embedded editor dispatch, code-server, native CLI line handling, SSH URL escaping, security/containment, attachments, recovery, caches, runtime/worktree inventory, settings, copy and layout changes.

## Acceptance

1. Before production changes, actual rendered Review toolbar -> actual menu -> real opener and `useRequest` -> stubbed HTTP-domain regression collects and fails solely on the secondary worktree ID; identical primary control passes. Capture commands, project, counts, failure and exit; fixture/setup failures are not RED.
2. Desktop/phone Review send the selected worktree and relative path for same-path siblings using a unique file-specific ID even without `workspace_path`; actual editor consumers resolve named scopes from observed placement. Name-only ambiguity, missing identity, independently proven conflicts and unscoped multi-worktree cases cannot launch. Legacy/single-repository cases and unrelated menu behavior follow the design.
3. Focused new and existing tests, scoped lint/type checks, staged-change i18n ratchets, public docs and spec/diff checks pass with actual counts and results recorded. State HappyDOM/native and Chromium limits accurately.

## UI-01: Existing file actions

[Full preview](plan.md#ui-01-existing-file-actions). AC 001.1-001.5.

```text
Desktop Review: Open with -> VS Code [enabled if resolved]
Phone Review: More actions -> VS Code [disabled if unknown]
```

Use existing menus, labels and accessible disabled state. No composition, touch geometry or scrolling change; verify actual phone overflow in the rendered boundary test.

## Verification

Only after the separate implementation request and exclusive heavy grant: prepend the pinned pnpm 9.15.9 directory `/home/jcfs/.local/share/mise/installs/pnpm/9.15.9`, Node 24.21.0 `bin`, and Go 1.26.0 `bin` to PATH; confirm versions. Run `pnpm install --frozen-lockfile` once in `apps/` if needed. Run heavy operations sequentially and join every actual native handle. Each `scripts/run-quiet` invocation gets a unique task-owned `KANDEV_RUN_QUIET_DIR`; do not use `--summary` for these unit checks. Obtain a fresh typecheck memory grant before typecheck. Resolve ordinary fixture/lint corrections at the approved cap; pause for unknown, auth, resource/OOM or out-of-scope failures.

Author `components/review/review-diff-toolbar.external-editor.test.tsx` first. Its test names must include `secondary repository routes the selected file` and `primary repository control`. Before production changes run:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales components/review/review-diff-toolbar.external-editor.test.tsx --maxWorkers=1 --no-file-parallelism)
```

Assert secondary failure is the missing `worktree_id`. The primary control accepts an omitted ID because the known first worktree is the backend default, or an explicit primary ID after the fix; it does not execute the backend. Do not count fixture setup errors as RED. Then implement and run the following after creating the two additional test files named below:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales components/review/review-diff-toolbar.external-editor.test.tsx components/editors/file-actions-repository-target.test.tsx hooks/domains/session/use-external-file-target.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && pnpm exec vitest run --project browser components/review/review-diff-toolbar.test.tsx components/editors/monaco/monaco-editor-toolbar.test.tsx hooks/use-open-session-in-editor.test.ts hooks/use-open-session-in-editor.wire.test.tsx --maxWorkers=1 --no-file-parallelism)
(cd apps/web && pnpm exec eslint --max-warnings 0 <every-actual-changed-ts-and-tsx-file-including-tests>)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Replace the lint placeholder with the complete actual owned TS/TSX file list, including helpers and tests, and record it. Stage actual owned additions before both staged-change ratchets and current actual PR-documentation coverage preflight; use normal hooks and no bypass. Check the actual PR-docs preflight interface before running it and record its result. Do not stage at the design checkpoint. Confirm test discovery, project and nonzero counts; a green empty selection is invalid. Update requirement/design status only after implementation matches them, and record all actual results here and in the plan.

The new Review fixture mounts `StateProvider`, store, `TooltipProvider`, real dropdown, opener and request; stub only HTTP-domain `openSessionInEditor` and unrelated display dependencies. Use two explicit associations with same `src/shared.ts`, distinct worktree and repository IDs and paths, a legacy primary `worktree_path`, no `workspace_path`, and click the actual VS Code menuitem. Add explicit Review ID without name and relevant legacy-primary controls. Add phone overflow with identical secondary assertion. The editor consumer suite renders actual Monaco dropdown and overflow, and CodeMirror overflow; assert HTTP payload and guard against the session-primary ID overriding a named secondary. Cover same-path, ID with absent or unresolvable redundant name, name-only placement, independently proven ID/name conflict, duplicate/missing association or placement, unknown and nested name-only scope, missing session, single/legacy/checkout fallback, multiple unscoped worktrees, external vs embedded item, folder and copy controls. Reuse `workspaceRelativeFilePath` and `isWorkspaceTreePath` where their contracts fit; do not use `getSessionWorkspacePath` legacy worktree fallback as the root for named placement or add path aliases. Tests must assert observable requests or disabled state, not copy implementation logic.

## Files likely touched

- `apps/web/components/review/review-diff-toolbar.tsx` and new rendered test.
- `apps/web/components/editors/file-actions-dropdown.tsx`, `editor-toolbar-overflow.tsx`, `monaco/monaco-editor-toolbar.tsx`, `codemirror/codemirror-editor-toolbar.tsx`, and new consumer test.
- `apps/web/hooks/domains/session/` target hook/resolver and tests; reuse `use-session-worktrees.ts` and existing session selectors.
- `docs/public/developer-tools.md`, this work package and paired specifications.

## Dependencies

None. Separate implementation authorization and exclusive heavy grant are prerequisites to execution, not work-order dependencies.

## Risks

Name is a workspace scope, not a display name. The editor panel's repository ID denotes the session primary, not necessarily the open file. Unknown placement or nested scope without an authoritative file-specific ID intentionally disables launch. Backend membership rejection remains authoritative. HappyDOM does not prove native or Chromium behavior.

## Parallelism

Sequential.

## Inputs

Paired requirement and system design, `use-session-worktrees.ts`, `file-tree-editor-target.ts` as a contrasting path shape, `service.go` session resolution, existing toolbar and opener tests, and `docs/public/developer-tools.md`.

## Results

- Independently authored Review RED: browser-locales, 2 collected; secondary failed on omitted worktree ID and primary passed. Shared overflow follow-up RED: the external item was disabled when the raw primary ID accompanied the secondary name.
- Final browser-locales focused suites: 3 files, 14 tests passed; after the fixture-only type correction, the changed editor suite passed 6 tests. Existing browser suites: 4 files, 43 tests passed. Both environments use HappyDOM.
- Scoped ESLint passed for all eight changed TS/TSX files. Web typecheck passed at 6144 MiB after correcting required test fixture props. i18n check and ratchet, E2E sleep ratchet, staged PR documentation coverage, catalog/spec lint, public docs tests (63 passed), public docs validation (54 pages), and staged diff check passed.
- Managed one-worker mobile-chrome disposable capture passed after correcting fixture navigation: desktop-width and phone-width Review menus were captured. The fixture was removed. Screenshots show menu presentation; no native editor launch or multi-repository Chromium routing is claimed.
