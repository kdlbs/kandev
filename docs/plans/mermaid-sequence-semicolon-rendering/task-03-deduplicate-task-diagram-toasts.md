---
id: "03-deduplicate-task-diagram-toasts"
title: "Deduplicate task Mermaid error toasts"
status: done
wave: 3
depends_on: ["02-log-failed-diagram-source"]
plan: "plan.md"
spec: "../../specs/ui/requirements/mermaid-rendering.md"
---

# Task 03: Deduplicate Task Mermaid Error Toasts

## Acceptance

1. Chat and task-plan Mermaid failures share one in-memory task-ID registry and show at most one
   error toast per task during a frontend runtime, including after task-panel remounts.
2. A different task can show its own first toast, while non-task Mermaid failures retain the
   existing unsuppressed behavior.
3. Suppressed toasts do not suppress full console diagnostics, inline errors, debounce,
   successful-render recovery, or stale-render cancellation.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- \
  components/shared/mermaid-block-streaming.test.tsx \
  components/shared/mermaid-error-toast.test.tsx
pnpm --filter @kandev/web exec eslint \
  components/shared/mermaid-block.tsx \
  components/shared/mermaid-block-streaming.test.tsx \
  components/shared/mermaid-error-toast.tsx \
  components/shared/mermaid-error-toast.test.tsx \
  --max-warnings 0
```

## Files likely touched

- `apps/web/components/shared/mermaid-error-toast.tsx`
- `apps/web/components/shared/mermaid-error-toast.test.tsx`
- `apps/web/components/shared/mermaid-block.tsx`
- `apps/web/components/shared/mermaid-block-streaming.test.tsx`
- `docs/plans/mermaid-sequence-semicolon-rendering/plan.md`
- `docs/plans/mermaid-sequence-semicolon-rendering/task-03-deduplicate-task-diagram-toasts.md`

## Dependencies

Task 02. Keep its full-source logging call ahead of toast suppression so every failed render remains
diagnosable.

## Parallelism

Sequential. This task builds on Task 02's chat rejection path and shares `mermaid-block.tsx` plus
its streaming tests.

## Inputs

- `docs/specs/ui/requirements/mermaid-rendering.md`
- `docs/plans/mermaid-sequence-semicolon-rendering/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`
- Existing deduplication patterns in `apps/web/hooks/use-session-failure-toast.ts` and
  `apps/web/hooks/use-task-deleted-toast.ts`.

## Risks

- A hook-local `Set` does not survive task-panel remounts; keep the task history at module scope.
- Do not assign unscoped Mermaid errors to whatever task happens to be focused.
- Capture the task ID associated with the render/event so an asynchronous rejection cannot consume
  another task's allowance after focus changes.
- Reset module state explicitly in tests so cases remain isolated.

## Output contract

Report RED/GREEN evidence, files changed, exact verification results, blockers or residual risks,
and update this task plus `plan.md` to `done`.

## Completion evidence

- RED: a MermaidBlock test lacked its required task state, demonstrating the new render path now
  captures task context before showing a toast.
- GREEN: `pnpm --filter @kandev/web test -- components/shared/mermaid-block-streaming.test.tsx components/shared/mermaid-error-toast.test.tsx` (8 tests).
- GREEN: the final focused suite passed 36 tests; changed-file ESLint passed with `--max-warnings 0`.
