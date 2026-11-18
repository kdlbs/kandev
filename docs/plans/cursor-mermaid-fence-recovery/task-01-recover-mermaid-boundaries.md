---
id: "01-recover-mermaid-boundaries"
title: "Recover glued Mermaid boundaries in chat"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MERMAID-RENDERING-002
acceptance_criteria:
  - AC-UI-MERMAID-RENDERING-002.1
  - AC-UI-MERMAID-RENDERING-002.2
  - AC-UI-MERMAID-RENDERING-002.3
  - AC-UI-MERMAID-RENDERING-002.4
  - AC-UI-MERMAID-RENDERING-002.5
  - AC-UI-MERMAID-RENDERING-002.6
  - AC-UI-MERMAID-RENDERING-002.7
system_design:
  - ../../specs/ui/system-design/mermaid-markdown-boundaries.md
---

# Task 01: Recover glued Mermaid boundaries in chat

## Summary

Recover the diagram/prose boundary through three existing frontend files.
Reuse the current diagram renderer, parser, render timer, cancellation handling, and Markdown context.

## In scope

- A small source-selection helper in `mermaid-utils.ts`.
- The existing `useMermaidRender` operation and a following-Markdown callback.
- A `pre` override for explicitly tagged Mermaid blocks.
- Focused parser/component tests and recovery cases in the existing desktop/phone specs.

## Out of scope

- Backend or transport changes, reply reconstruction, stored-message rewrites, or dependency updates.
- Standalone parser-plugin modules, fence-position metadata, recovery components, async controllers, caches, or timers.
- Untagged diagrams, task-plan rich text, arbitrary diagram repair, or new UI controls.

## Acceptance

1. The named component regression fails before the fix and passes with the real diagram source and separate Markdown prose.
2. All seven linked criteria pass, including valid-label preservation, exact stored content, current context, and cancellation behavior.
3. Every command in Verification passes. Results records actual outcomes, including both browser projects.

## ASCII UI preview

### UI-01: Chat diagram followed by prose

Desktop and phone share this structure:

```text
[Diagram viewport]
  A -> B

Following paragraph.
```

See the [combined preview](plan.md#ascii-ui-preview).
The diagram/prose order and sibling placement satisfy AC-002.1, AC-002.5, and AC-002.7.
Spacing is illustrative. Keep the existing diagram viewport and transcript scroll owner.

## Implementation sequence

1. Read the linked requirement/design, scoped web guidance, `/tdd`, `/mobile-parity`, and `/e2e`.
2. Mark the task `in_progress` and synchronize the plan.
3. Add `recovers prose glued to a closing Mermaid fence` in `markdown-mermaid-recovery.test.tsx`.
4. Run it before production edits and record the expected failure.
5. Extend `mermaid-utils.test.ts` with real-parser recovery, valid-label candidate scanning, invalid-prefix, and no-candidate cases.
6. Implement source selection in `mermaid-utils.ts` and call it from the current debounced render operation.
7. Add the explicit Mermaid `pre` route and following-Markdown callback in `markdown-components.tsx`.
8. Complete component assertions for sibling placement, inherited task/file context, quiet probes, and stale-result rejection.
9. Extend the existing desktop/mobile fixture and specs with recovery, reload, and exact saved-content assertions.
10. Run Verification, record results, and synchronize completed task/plan status.

After complete-source validation fails, candidate lines are checked in source order until the first prefix validates.
Complete valid source is preserved. If no prefix validates or a probe unexpectedly rejects, the original source is selected.
Publish the selected diagram and following text together through the existing render lifecycle.
Use the selected source for the diagram's code view.
Keep Mermaid loading, configuration, debounce, cancellation, and final-error behavior in their existing owner.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)

(cd apps/web && pnpm exec vitest run \
  components/shared/mermaid-utils.test.ts \
  components/shared/markdown-mermaid-recovery.test.tsx \
  components/shared/markdown-components.test.tsx \
  components/shared/mermaid-block.test.tsx \
  components/shared/mermaid-block-streaming.test.tsx)

(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  components/shared/mermaid-utils.ts \
  components/shared/mermaid-utils.test.ts \
  components/shared/markdown-components.tsx \
  components/shared/markdown-mermaid-recovery.test.tsx \
  components/shared/mermaid-block.tsx \
  e2e/tests/chat/mermaid-fit-helpers.ts \
  e2e/tests/chat/mermaid-fit.spec.ts \
  e2e/tests/chat/mobile-mermaid-fit.spec.ts)

(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)

(cd apps/web && pnpm e2e:run --project chromium tests/chat/mermaid-fit.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-mermaid-fit.spec.ts)

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all

node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/ui/requirements/mermaid-rendering.md',
  'docs/specs/ui/system-design/mermaid-markdown-boundaries.md',
  'docs/plans/cursor-mermaid-fence-recovery/plan.md',
  'docs/plans/cursor-mermaid-fence-recovery/task-01-recover-mermaid-boundaries.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/web/components/shared/markdown-components.tsx', status: 'modified' },
    ...documents.map(filename => ({ filename, status: 'modified' })),
  ],
  fileContents: Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE

git diff --check
git status --short -- docs/plans/cursor-mermaid-fence-recovery
```

The coverage preflight includes a representative source-change trigger during design-only validation.
The phone run reuses the preceding desktop build only without intervening source changes.
Use the managed worker budget and run browser projects sequentially.

## Files likely touched

Production files:

- `apps/web/components/shared/mermaid-utils.ts`: source-selection helper.
- `apps/web/components/shared/mermaid-block.tsx`: integrate selection and render following Markdown.
- `apps/web/components/shared/markdown-components.tsx`: explicit Mermaid `pre` routing and Markdown callback.

Tests:

- `apps/web/components/shared/mermaid-utils.test.ts`: extend real-parser cases.
- `apps/web/components/shared/markdown-mermaid-recovery.test.tsx`: new focused renderer regression and lifecycle/context cases.
- `apps/web/e2e/tests/chat/mermaid-fit-helpers.ts`: allow seeded content while retaining the wide default.
- `apps/web/e2e/tests/chat/mermaid-fit.spec.ts`: add desktop recovery case.
- `apps/web/e2e/tests/chat/mobile-mermaid-fit.spec.ts`: add phone recovery case.

Update delivery records and the paired design's lifecycle status after implementation.
Do not change the normalizer, backend, lockfile, locales, or completed companion plans.

## Dependencies

None. Resolve current base versions of the referenced contracts before implementation.

## Risks

- Valid labels can contain apparent fences. The complete-source check must precede the prefix check.
- Superseded async results must not publish old prose or a prior task's result.
- Recovered prose must inherit the existing Markdown/link policy and remain outside `pre`.

## Parallelism

`sequential`. No subagents are authorized.

## Inputs

- [Requirements](../../specs/ui/requirements/mermaid-rendering.md), requirement 002 and its seven acceptance criteria.
- [System design](../../specs/ui/system-design/mermaid-markdown-boundaries.md), all sections.
- [Plan evidence and scope](plan.md).
- Existing `MermaidBlock`, `MarkdownCode`, `sanitizeMermaidCode`, and their tests.
- Existing desktop/mobile Mermaid-fit specs and fixture helper.

## Results

Implemented the bounded recovery for explicitly tagged Mermaid blocks. Source selection runs within the existing Mermaid render lifecycle, and recovered prose is rendered as a sibling through the existing Markdown components and link context. Stored message content and provider segmentation remain unchanged.

- Initial implementation TDD RED/GREEN: the named component regression and desktop/phone recovery cases failed before the fix and passed afterward. Focused Vitest passed 5 files and 94 tests, including real-parser source-selection cases and the Mermaid renderer suites.
- Initial implementation Oxlint, typecheck, `i18n:ratchet`, documentation checks, and the managed production Vite build passed. Desktop `chromium` Mermaid-fit passed 2/2; phone `mobile-chrome` Mermaid-fit passed 2/2.
- Static-review follow-up: the file-preview Mermaid routing regression and five ineligible backtick-run cases were confirmed RED before correction. The focused rerun passed 3 files and 92 tests, covering parser selection, Markdown preview, and chat recovery.
- PR review follow-up: the reference-definition and latest-code-view regressions were confirmed RED before correction. The focused rerun passed 3 files and 94 tests, covering parser selection, Markdown preview, reference-link recovery, and the latest code view after a failed update. Frontend typecheck, focused Oxlint, Prettier, and `git diff --check` passed.
- Additional PR review follow-up: the multiline-label candidate and Mermaid initialization-order regressions were confirmed RED before correction. The focused rerun passed 3 files and 95 tests, covering later-candidate recovery and initialization immediately before render.
- Follow-up Oxlint passed for the changed frontend files, Prettier passed for all nine changed TypeScript/TSX files, and `git diff --check` passed.
- The broad build, i18n, and desktop/phone browser projects were not rerun for the review corrections. Their results above apply to the earlier implementation. Frontend typecheck was rerun after the PR review follow-ups and passed.
- No dependencies, locales, backend code, public documentation, or persisted message formats changed.
