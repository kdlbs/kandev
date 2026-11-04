---
id: "02-repair-compiler-compatibility"
title: "Repair native compiler compatibility"
status: completed
wave: 2
depends_on: ["01-preserve-verification-evidence"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-010
  - REQ-PLATFORM-CI-PERFORMANCE-011
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-010.1
  - AC-PLATFORM-CI-PERFORMANCE-010.2
  - AC-PLATFORM-CI-PERFORMANCE-010.3
  - AC-PLATFORM-CI-PERFORMANCE-010.4
  - AC-PLATFORM-CI-PERFORMANCE-010.5
  - AC-PLATFORM-CI-PERFORMANCE-011.1
system_design:
  - ../../specs/platform/system-design/web-native-verification.md
---

# Task 02: Repair native compiler compatibility

## Summary

Install the exact tsrs candidate and repair the shared TypeScript 7 compatibility errors.
Keep the existing required typecheck active until the reference, fast mode, and retained API consumers pass.

## In scope

- Add `tsrs@0.20.1-ts7.1.0-beta` as a web development dependency and update the workspace lockfile.
- Keep workspace TypeScript 5 and existing package compilers unchanged.
- Refresh the initial six-file, 12-diagnostic inventory against the current source.
- Complete Observer mock interfaces, including `scrollMargin`, with unchanged behavior.
- Type WebSocket test states with their closed values instead of broad casts.
- Narrow `buildResizeBuffer` to its actual allocation type without changing bytes or adding a runtime copy.
- Preserve all existing terminal, mock, event, and callback assertions.
- Use the Task 01 helper to compare TypeScript 5, TypeScript 7.1 beta, tsrs default, and tsrs fast.
- Exercise cold, fresh incremental, and changed-invalid-source cache cases.
- Exercise supported checker counts against matching reference counts.
- Retain the classic TypeScript API needed by lint tools and RuleTester.

## Out of scope

- Changing the required `typecheck` command, SDK/desktop compilers, libraries, strictness, exclusions, or heap budgets.
- Ordinary-mode substitution, `noCheck`, broad `any`, double casts, and new suppression comments.

## Acceptance

1. The legacy compiler, pinned reference, default tsrs, and fast tsrs pass the repaired complete project with identical non-dependency inputs.
2. Fast mode preserves the reference corpus diagnostics, and all basic injected errors remain detectable after cold and warm verification.
3. Existing terminal and Observer tests pass, frame bytes remain identical, and classic compiler API consumers remain usable.

## Verification

Task 01 provides the comparison command. It provisions the reference outside workspace dependencies.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/review/review-diff-list-auto-mark.test.tsx components/task/chat/messages/kandev/rich-output/chart-block.test.tsx components/threads/thread-column-activation.test.tsx hooks/use-lazy-load-sentinel.test.ts lib/terminal/send-pty-input.test.ts components/task/use-passthrough-terminal.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
python3 .github/scripts/web-toolchain-evidence.py compare-types --web-root apps/web --mode fast
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

TDD first adds a resize-frame regression where the return-type repair needs runtime evidence.
Existing mock-only repairs retain every assertion and scenario.
The comparison report records exact tool versions, reference commit, effective checker settings, complete diagnostics, and input sets.
Any relevant missing fast-mode diagnostic keeps this task incomplete.

## Files likely touched

- `apps/web/package.json` and `apps/pnpm-lock.yaml`.
- `apps/web/components/review/review-diff-list-auto-mark.test.tsx`.
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.test.tsx`.
- `apps/web/components/threads/thread-column-activation.test.tsx`.
- `apps/web/hooks/use-lazy-load-sentinel.test.ts`.
- `apps/web/lib/terminal/send-pty-input.test.ts`.
- `apps/web/components/task/use-passthrough-terminal.ts` and its existing test.
- `.github/scripts/fixtures/web-toolchain/` and this package's evidence.

## Dependencies

Task 01.
Read the design's Compiler boundary and TypeScript migration sections.
Read `apps/web/AGENTS.md` before source changes.

## Risks

- The upstream fast preset can remove diagnostics near implementation limits.
- An aliased native `tsc` can change unrelated package behavior.
- A buffer cast can conceal a SharedArrayBuffer mismatch instead of describing the actual allocation.

## Parallelism

`sequential`

## Results

Completed on November 3, 2026.

- Added `tsrs@0.20.1-ts7.1.0-beta` as a web development dependency with the
  exact version in `apps/web/package.json` and the workspace lockfile. The
  workspace keeps TypeScript 5 and every other package compiler.
- Repaired the current six-file, 12-diagnostic inventory: the four
  `MockIntersectionObserver` declarations now implement `scrollMargin`; the
  WebSocket test doubles use their closed `readyState` literals; and
  `buildResizeBuffer` declares the `Uint8Array<ArrayBuffer>` it actually
  allocates. No assertion, frame byte, suppression, or compiler option changed.
- `compare-types` passed for workspace `tsc` 5.9.3, reference TypeScript
  `7.1.0-beta`, tsrs default, and `tsrs --fast` on the repaired project in cold
  and fresh incremental runs. Matching `--checkers 4` settings applied to the
  reference and both tsrs modes on the project; the corpus also passed at the
  supported one-, four-, and eight-checker settings. The required corpus reported eight diagnostics
  in every compiler and mode for the invalid project, zero for the valid
  project, and the expected TS2589 for the instantiation project. A cold, warm,
  and changed-invalid probe rejected stale cached success. Input inventories
  matched.
- `pnpm run typecheck` and `NODE_OPTIONS=--max-old-space-size=8192 pnpm exec
  tsc --noEmit` both pass. The focused terminal and Observer tests pass.
- `pnpm run lint`, `pnpm run i18n:ratchet`, and the resident resize-frame test
  pass.
