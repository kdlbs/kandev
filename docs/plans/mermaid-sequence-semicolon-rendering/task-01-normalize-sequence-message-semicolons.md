---
id: "01-normalize-sequence-message-semicolons"
title: "Normalize sequence-message semicolons"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/mermaid-rendering.md"
---

# Task 01: Normalize sequence-message semicolons

## Acceptance

1. The reported sequence diagram normalizes literal semicolons in message prose to `#59;` and no
   longer produces the reproduced Mermaid parser error.
2. Existing `#59;` escapes, valid inline sequence-statement separators, and semicolons in other
   Mermaid diagram types retain their existing meaning.
3. Focused tests cover LF and Windows CRLF source without changing renderer-specific code.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- components/shared/mermaid-utils.test.ts
pnpm --filter @kandev/web exec eslint \
  components/shared/mermaid-utils.ts \
  components/shared/mermaid-utils.test.ts \
  --max-warnings 0
```

## Files likely touched

- `apps/web/components/shared/mermaid-utils.ts`
- `apps/web/components/shared/mermaid-utils.test.ts`
- `docs/plans/mermaid-sequence-semicolon-rendering/plan.md`
- `docs/plans/mermaid-sequence-semicolon-rendering/task-01-normalize-sequence-message-semicolons.md`

## Dependencies

None. Both chat Markdown and task-plan Mermaid rendering already call `sanitizeMermaidCode`.

## Parallelism

Sequential. The production helper and its regression tests are one focused TDD change.

## Inputs

- `docs/specs/ui/requirements/mermaid-rendering.md`
- `docs/plans/mermaid-sequence-semicolon-rendering/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`
- The reported diagram and confirmed Mermaid 11.16.0 reproduction from the task conversation.

## Risks

- Preserve valid semicolon-separated inline statements rather than converting every semicolon
  after a message colon.
- Do not double-encode entity escapes.
- Keep sequence-specific behavior out of flowchart, state, ER, and other Mermaid diagram types.

## Output contract

Report RED/GREEN evidence, files changed, exact verification results, blockers or residual risks,
and update this task plus `plan.md` to `done`.

## Completion evidence

- RED: the inline sequence-separator regression initially converted `; B->>A` to `#59; B->>A`.
- GREEN: `pnpm --filter @kandev/web test -- components/shared/mermaid-utils.test.ts` (26 tests).
- GREEN: `pnpm --filter @kandev/web exec eslint components/shared/mermaid-utils.ts components/shared/mermaid-utils.test.ts --max-warnings 0`.
