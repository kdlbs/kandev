---
id: "01-normalize-tool-file-paths"
title: "Normalize tool file paths"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-TREE-PATH-SCOPE-001
acceptance_criteria:
  - AC-UI-FILE-TREE-PATH-SCOPE-001.1
  - AC-UI-FILE-TREE-PATH-SCOPE-001.2
  - AC-UI-FILE-TREE-PATH-SCOPE-001.6
system_design:
  - ../../specs/ui/system-design/file-tree-path-scope.md
---

# Task 01: Normalize Tool File Paths

## Summary

Give tool-originated workspace files one relative identity before editor, pending-cursor, and scroll
state is written, while leaving external absolute content reads unchanged.

## In scope

- Add the shared frontend workspace-path classifier and normalizer.
- Handle POSIX and Windows roots, separator normalization, drive-letter case, and complete-segment
  containment.
- Normalize read and edit tool opens inside `useOpenFileAtLine` before every editor and cursor call.
- Use the same containment decision for the edit-card open affordance instead of raw string prefix
  matching.
- Reuse the helper in the LSP opener where it replaces equivalent local path logic without changing
  LSP behavior.
- Add desktop and phone end-to-end coverage for an agent read card containing the active workspace's
  absolute file path and a line target.

## Out of scope

- File-tree expansion, restore, refresh, or backend validation from Task 02.
- Blocking an external absolute path from the read-only content endpoint.
- New editor or phone-viewer UI.

## Acceptance

- A contained absolute tool path opens, keys its pending cursor, and scrolls as the corresponding
  workspace-relative path.
- A path outside the workspace or sharing only its string prefix remains absolute and does not gain
  workspace identity.
- Desktop and phone open the same relative workspace file at the requested line through their
  existing editor/viewer surfaces.

## TDD start

First add a `useOpenFileAtLine` regression that passes `/workspace/src/app.ts` with workspace root
`/workspace` and expects `src/app.ts` in `setPendingCursorPosition`, `onOpenFile`, and
`scrollEditorIfMounted`. It fails today because all three receive the absolute path. Add the prefix
collision and external absolute cases before changing production code.

## ASCII UI preview

Relevant excerpt from [UI-01](plan.md#ui-01-open-a-workspace-file-from-an-agent-tool-card):

```text
Desktop: Read /workspace/public/.../case.json -> Editor public/.../case.json
Phone:   Read /workspace/public/.../case.json -> native viewer public/.../case.json
External /opt/reference.md                    -> existing read-only viewer; Files unchanged
```

The rendered components and navigation remain unchanged. Only the path identity crossing the
existing open action changes.

## Verification

Run from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/workspace-file-path.test.ts \
  hooks/use-file-editors.test.tsx components/task/chat/messages/tool-edit-message.test.tsx \
  hooks/use-lsp-file-opener.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/workspace-file-path.ts hooks/use-file-editors.ts \
  components/task/chat/messages/tool-edit-message.tsx hooks/use-lsp-file-opener.ts \
  --max-warnings 0)
(cd apps/web && pnpm e2e:run --project chromium \
  tests/task/chat-read-absolute-workspace-path.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-file-viewer.spec.ts --grep "absolute workspace read link")
```

## Files likely touched

- `apps/web/lib/workspace-file-path.ts`
- `apps/web/lib/workspace-file-path.test.ts`
- `apps/web/hooks/use-file-editors.ts`
- `apps/web/hooks/use-file-editors.test.tsx`
- `apps/web/hooks/use-lsp-file-opener.ts`
- `apps/web/hooks/use-lsp-file-opener.test.ts`
- `apps/web/components/task/chat/messages/tool-edit-message.tsx`
- `apps/web/components/task/chat/messages/tool-edit-message.test.tsx`
- `apps/web/e2e/tests/task/chat-read-absolute-workspace-path.spec.ts`
- `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts`

## Dependencies

None.

## Risks

- The pending cursor map must use the normalized identity before the editor mounts.
- Tool edit cards currently use a permissive prefix test; changing it must preserve their existing
  copy-path fallback for external files.
- `file:///` roots used by Monaco tests need canonical handling without treating an arbitrary URI as
  a tree path.

## Parallelism

`sequential`

## Inputs

- `AC-UI-FILE-TREE-PATH-SCOPE-001.1`, `.2`, and `.6`.
- Path identity and tool-file open flow in the system design.
- ADR 0016's external absolute content-read exception.

## Results

Implemented the shared path classifier and normalized workspace-contained read and edit paths before
editor, pending-cursor, and scroll state is written. The edit-card containment check and LSP alias
conversion now use the same segment-aware helper. External absolute paths remain unchanged.

Validation passed:

- Four focused unit files passed with 56 tests.
- Frontend typecheck and changed-file ESLint passed.
- The Chromium desktop regression passed with one test and observed only relative tree requests.
- The Pixel 5 phone regression passed with one test and reached the requested line in the existing
  native viewer.
