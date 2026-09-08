---
id: "02-log-failed-diagram-source"
title: "Log failed Mermaid diagram source"
status: done
wave: 2
depends_on: ["01-normalize-sequence-message-semicolons"]
plan: "plan.md"
spec: "../../specs/ui/requirements/mermaid-rendering.md"
---

# Task 02: Log failed Mermaid diagram source

## Acceptance

1. Every rejected chat or task-plan Mermaid render emits one searchable `[mermaid]`
   `console.error` containing the parser error and complete original diagram source.
2. The same entry includes complete normalized source only when normalization changed the input,
   and it uses flat multiline text that can be copied directly from browser logs.
3. Existing toast, inline-error, debounce, recovery, and stale-render behavior remains unchanged.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- \
  components/shared/mermaid-utils.test.ts \
  components/shared/mermaid-block-streaming.test.tsx
pnpm --filter @kandev/web exec eslint \
  components/shared/mermaid-utils.ts \
  components/shared/mermaid-utils.test.ts \
  components/shared/mermaid-block.tsx \
  components/shared/mermaid-block-streaming.test.tsx \
  components/editors/tiptap/tiptap-mermaid-extension.ts \
  --max-warnings 0
```

## Files likely touched

- `apps/web/components/shared/mermaid-utils.ts`
- `apps/web/components/shared/mermaid-utils.test.ts`
- `apps/web/components/shared/mermaid-block.tsx`
- `apps/web/components/shared/mermaid-block-streaming.test.tsx`
- `apps/web/components/editors/tiptap/tiptap-mermaid-extension.ts`
- `docs/plans/mermaid-sequence-semicolon-rendering/plan.md`
- `docs/plans/mermaid-sequence-semicolon-rendering/task-02-log-failed-diagram-source.md`

## Dependencies

Task 01. Build on the final shared normalizer signature and its normalized output.

## Parallelism

Sequential. This task shares `mermaid-utils.ts` and its tests with Task 01, then wires both
renderer-specific rejection paths.

## Inputs

- `docs/specs/ui/requirements/mermaid-rendering.md`
- `docs/plans/mermaid-sequence-semicolon-rendering/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/debug/references/instrumentation.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Risks

- Full diagram source may contain sensitive text. Keep the requested diagnostic client-side and
  rely only on the existing user-initiated browser-log export path.
- Log flat text rather than a collapsed object so users can copy the complete source.
- Report only after a render promise rejects; do not log transient streaming prefixes that never
  reach Mermaid because debounce cancellation suppresses them.

## Output contract

Report RED/GREEN evidence, files changed, exact verification results, blockers or residual risks,
and update this task plus `plan.md` to `done`.

## Completion evidence

- RED: the chat rejection regression observed no `console.error` call.
- GREEN: `pnpm --filter @kandev/web test -- components/shared/mermaid-utils.test.ts components/shared/mermaid-block-streaming.test.tsx` (32 tests).
- GREEN: changed-file ESLint completed with `--max-warnings 0`.
