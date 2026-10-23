---
id: "01-input-seam"
title: "Isolate the engine input seam"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.3
  - AC-UI-MARKDOWN-FILE-EDITING-001.5
  - AC-UI-MARKDOWN-FILE-EDITING-002.8
  - AC-UI-MARKDOWN-FILE-EDITING-002.15
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 01: Isolate the engine input seam

## Summary

Add a narrow input factory to the pinned engine without changing its native
Chromium behavior. Make the runtime patch, declarations, and frozen install
reproducible. This is an internal seam with no visible UI change.

## In scope

- Patch context creation, attachment, focus, suspension, caret geometry, and
  disposal. Preserve the model, controller, parser, renderer, and history.
- Define the local transport context and native EditContext implementation.
  Keep imports below `HybridMarkdownEditor`. Include a native-selection callback
  with canonical anchor/active offsets and instance/source revision.
- Register the exact-version pnpm patch and its lockfile identity.
- Test one disposal, controller-before-transport teardown, suspension, source
  updates, stale callbacks, and failed initialization with a fake transport.

## Out of scope

- Textarea input, global API changes, parser changes, and a package upgrade.
- Browser support claims or user-facing mode changes.

## Acceptance

- A fake transport proves injection, source synchronization, focus, suspension,
  and teardown. Its selection-only callback updates model selection without
  source/history mutation. The unmodified package fails these regressions first.
- Native Chromium passes existing rendered editing, table, comment, exact-byte,
  position, and failure/retry scenarios with no source or UI regression.
- A clean frozen install applies runtime and declarations. Upgrading the
  package cannot silently bypass or partially apply the patch.

## Verification

Commands start at the repository root. Run browser commands sequentially.

```bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web exec vitest run components/editors/markdown/hybrid-markdown-editor.test.tsx components/editors/markdown/hybrid-markdown-comments.test.tsx components/editors/markdown/markdown-table-edit.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/markdown-file-editing.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts
git diff --check
```

Also verify patch application in a temporary install directory so cached
node_modules cannot hide missing patch registration. Do not create a branch or
reset this workspace for that check.

## Files likely touched

- `apps/patches/@vscode__markdown-editor@0.0.2-84.patch` (new).
- `apps/package.json`, `apps/pnpm-lock.yaml`.
- `apps/web/components/editors/markdown/input/markdown-input-transport.ts` (new).
- `apps/web/components/editors/markdown/input/markdown-native-input.ts` (new).
- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx` and its tests.

## Dependencies

None.

## Risks

The engine currently owns context attachment and root focus. Partial delegation
would leave two lifecycle owners. Patch runtime and declarations together.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [Browser transport design](../../specs/ui/system-design/markdown-file-editing.md#browser-input-transport).
- [Input decision](../../decisions/2026-10-21-scoped-markdown-browser-input.md).
- Existing adapter lifecycle and real-engine browser tests.

## Results

Completed with TDD. The unpatched real engine ignored the factory. Attachment
failure initially leaked the transport. An initial seam also added duplicate
selection synchronization. Each regression failed before its correction.

Final checks passed: five focused files with 38 tests, TypeScript, changed-file
ESLint, pnpm 9.15.9 frozen install, and direct patch application against a
pristine extracted package. Native Chromium passed eight existing scenarios;
the position handoff then passed after splitting transport publication from
selection synchronization. All seven existing phone scenarios passed.

Root browser logs: `/tmp/kandev-run.e2e.p9LRjidT.log` (initial eight passes and
position failure), `/tmp/kandev-run.e2e.GhC3S5Er.log` (corrected position pass),
`/tmp/kandev-run.e2e.7qbgem10.log` (seven phone passes). At the Task 01 checkpoint, the new missing-API rendered editing regression
remained red for Task 02. Later browser results are recorded in the plan.
