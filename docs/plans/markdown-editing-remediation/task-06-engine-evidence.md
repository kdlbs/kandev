---
id: "06-engine-evidence"
title: "Prove real-engine behavior and reconcile records"
status: completed
wave: 6
depends_on: ["05-position-handoff"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.3
  - AC-UI-MARKDOWN-FILE-EDITING-001.6
  - AC-UI-MARKDOWN-FILE-EDITING-001.8
  - AC-UI-MARKDOWN-FILE-EDITING-001.9
  - AC-UI-MARKDOWN-FILE-EDITING-002.9
  - AC-UI-MARKDOWN-FILE-EDITING-002.10
  - AC-UI-MARKDOWN-FILE-EDITING-002.11
  - AC-UI-MARKDOWN-FILE-EDITING-002.12
  - AC-UI-MARKDOWN-FILE-EDITING-003.6
  - AC-UI-MARKDOWN-FILE-EDITING-003.7
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 06: Prove Real-Engine Behavior and Reconcile Records

## Summary

Replace irrelevant preservation evidence with tests of the real pinned engine
through the production adapter. Record supported user behavior and close the
original table work orders only after their targeted checks pass.

## In scope

- Add exact-byte fixtures for BOM, LF/CRLF, frontmatter, headings, emphasis,
  nested/task lists, escaped table pipes, fences, math, footnotes, raw HTML,
  links, images, comments, and unsupported syntax.
- Exercise the real controller through rendered input, focus/blur, mode
  changes, targeted edits, undo/redo, replacement, and save.
- Remove `markdown-source-preservation.ts` if it still has no production
  caller. Keep mocked adapter tests for lifecycle wiring, not fidelity claims.
- Record real-engine table insertion, resize, delimiter, and touch evidence
  for original Tasks 06 and 07.
- Add a short Markdown editing how-to under the workbench documentation.
  Explain modes, capability fallback, unsaved changes, Save, Reload, and retry.

## Out of scope

- A generic backend audit, full-repository verification, or unrelated CI fixes.
- Upgrading the engine to make tests pass without a separate compatibility review.
- Claiming native Safari or Firefox coverage from Chromium simulation.

## Acceptance

- The corpus reaches the production adapter and actual engine. Tests compare
  exact canonical source after real interactions, including one intended edit
  and undo/redo. Directly setting the expected model text is not evidence.
- The completed desktop and phone flows pass, including recovery, repository
  comments, draft transfer, phone controls, position handoff, and table behavior.
- Original plan records distinguish historical and current results. Public
  documentation matches tested behavior. Unavailable checks remain explicitly
  unverified and do not produce a completed status.

## Verification

Retain or replace the existing preservation test file with meaningful
production-boundary coverage. Browser corpus assertions belong in the
existing desktop and phone Markdown specs. Do not add a fake implementation
that mirrors the intended result.

Run the completed E2E files sequentially. Record native browser smoke results
separately if the environment provides those browsers. From repository root:

```bash
(cd apps/web && pnpm exec vitest run components/editors/markdown components/task/markdown-file-editor.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project=chromium tests/task/markdown-file-editing.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

These include the original table work orders' required checks. Record actual
command results against the final implementation head. If integration with
main changes relevant code, rerun the affected work-order checks there.

## Files likely touched

- `apps/web/components/editors/markdown/markdown-source-preservation.ts`
- `apps/web/components/editors/markdown/markdown-source-preservation.test.ts`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.test.tsx`
- `apps/web/e2e/tests/task/markdown-file-editing.spec.ts`
- `apps/web/e2e/tests/task/mobile-markdown-file-editing.spec.ts`
- `docs/public/sessions-and-review.md`
- `docs/plans/markdown-file-editing/plan.md`
- `docs/plans/markdown-file-editing/task-06-positional-table-edits.md`
- `docs/plans/markdown-file-editing/task-07-table-edge-editing-chrome.md`
- This plan and its work-order result sections.
- The paired requirement and design status, only after full conformance.

## Dependencies

Tasks 01 through 05. Their regressions become part of the completed browser
files. This work supplies engine fidelity evidence, not a second generic review.

## Risks

Upstream behavior can expose additional source mutations. Preserve failing
fixtures and repair the bounded adapter defect before claiming conformance.
Do not normalize expected bytes to hide the failure.

The existing PR includes unrelated work and conflicts with main. Record that
delivery limitation without broadening this package into unrelated remediation.

## Parallelism

`sequential`

## Inputs

- [Validation strategy](../../specs/ui/system-design/markdown-file-editing.md#validation-strategy).
- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [Original package](../markdown-file-editing/plan.md).
- `docs/public/README.md` and the docs-maintainer skill.

## Results

Completed on 2026-10-20 in the uncommitted remediation worktree on
`feature/improve-markdown-pre-a36`, based on
`b03dce19114c4a63b8a6716894906dbc7f351c95`.

- Targeted unit tests: 311 passed across 31 files. This combines the 255-test
  package aggregate, 51 repository-opening tests, and five Dockview caller tests.
- Desktop Chromium: all 10 Markdown scenarios verified. The full run passed
  nine scenarios. The repository-comment scenario passed separately after the
  final qualifier-forwarding fix and fixture correction.
- Phone Chromium: the complete seven-scenario Markdown file passed. The two
  existing file-confirmation scenarios also passed at 390px and 820px.
- The real engine preserved exact BOM, LF, CRLF, frontmatter, unsupported
  constructs, and surrounding source through edits, undo/redo, mode changes,
  and save. No expected bytes were normalized.
- Desktop and phone table interactions passed. Original Tasks 06 and 07 now
  record these results instead of inferring completion from source presence.
- TypeScript, changed-file ESLint with zero warnings, translation checks,
  production builds, and whitespace checks passed.
- English and Portuguese 320px phone screenshots were inspected. Save, mode
  selection, and actions fit while a remote update is pending.

Browser runs used `--host --shards 1`, with one worker and no overlapping suites.
The completed phone run and final repository regression reused the final build
with `--no-build`. The host required both `KANDEV_INTERNAL_CONFIG_FILE` and
`KANDEV_INTERNAL_CONFIG_HOME_FILE` to be unset and
`KANDEV_SERVER_HOST=127.0.0.1`. Chromium also required the host Nix library paths.
The tests wait for menu closure and CodeMirror's asynchronous Android Enter
update before subsequent input. They assert the actual selection and source.

The unused preservation helper and its tests were removed. Public editing
instructions, requirements, design, and delivery records now match the tested
behavior. Document catalog and specification lint passed. Public documentation
validation passed for 47 pages, with all 63 validator tests passing. Reference
coverage passed for all nine changed work orders. Whitespace checks passed.

Native Safari and Firefox were not available. Their native input behavior
remains unverified. The missing-EditContext tests are capability simulations.
The PR's existing conflict with main remains outside this package. No commit,
push, merge, or GitHub comment was performed.
