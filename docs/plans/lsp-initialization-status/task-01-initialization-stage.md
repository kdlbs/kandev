---
id: "01-initialization-stage"
title: "Initialization stage disclosure"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 01: Initialization Stage Disclosure

## Acceptance

- A task-host `ready` handshake is presented as a launched server process waiting for the LSP initialize response, not as a process that has not started.
- At 60 seconds the presentation becomes a long-running warning; Kotlin names Gradle import only as a possible cause and never supplies an ETA or inferred percentage.
- The request remains live and Stop remains enabled before and after the threshold.

## TDD sequence

1. Add failing cases to `apps/web/lib/lsp/lsp-progress-view.test.ts` for immediate and 60-second held initialization.
2. Implement the minimal pure presentation state and shared copy.
3. Refactor the rendered details while keeping the focused tests green.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/lsp/lsp-progress-view.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint lib/lsp/lsp-progress-view.ts lib/lsp/lsp-progress-view.test.ts components/editors/lsp-status-button.tsx
```

## Files likely touched

- `apps/web/lib/lsp/lsp-progress-view.ts`
- `apps/web/lib/lsp/lsp-progress-view.test.ts`
- `apps/web/components/editors/lsp-status-button.tsx`
- `apps/web/components/editors/lsp-progress-details.tsx` if extraction keeps component size/ownership clear

## Dependencies

None.

## Parallelism

Sequential. Task 03 consumes the shared presentation and Task 04 tests it through the browser.

## Inputs

- Spec sections: What, Readiness and progress state, Failure modes, held-initialize scenarios.
- Existing `initializingSince` ownership in `lsp-client-progress.ts`.
- Kotlin LSP issues 148 and 189 as evidence, not implementation instructions.

## Output contract

Record RED/GREEN evidence, files changed, exact commands, remaining upstream limitations, and update this task plus `plan.md`.

## Result

- RED: `pnpm --dir apps/web exec vitest run lib/lsp/lsp-progress-view.test.ts` failed on the old “Preparing project…” state and the missing 60-second boundary.
- GREEN: the same focused suite passes 7 tests; focused ESLint and the full web typecheck pass.
- The presentation now distinguishes a launched process from protocol readiness, switches to long-running guidance at exactly 60 seconds, keeps Stop enabled, and names Gradle import only as a Kotlin possibility.
- Upstream limitation: servers can still omit progress or never answer `initialize`; Kandev intentionally does not infer completion or impose an automatic timeout.
