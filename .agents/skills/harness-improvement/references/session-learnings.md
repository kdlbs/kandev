# Session Learnings

Use this when the user asks for "session learnings", "harness improvements", "worth recording", or feeds feedback from another agent.

## Convert Feedback Into A Durable Change

For each learning, extract:

- **Trigger:** when future agents should apply it.
- **Failure mode:** what went wrong or wasted time.
- **Correct action:** exact behavior, command, fallback, or interpretation.
- **Verification:** how an agent knows the workaround succeeded.
- **Home:** skill, agent, script, AGENTS.md, or command.

Prefer this shape inside skills:

```md
If <condition>, do <action>. Why: <short reason>. Verify with <command/output>.
```

## Placement Rules

- PR/CI/review-thread behavior belongs in `.agents/skills/pr-fixup/SKILL.md`.
- PR creation/body/push behavior belongs in `.agents/skills/pr/SKILL.md` or `.agents/skills/push/SKILL.md`.
- Full repo verification belongs in `.agents/skills/verify/SKILL.md`.
- TDD/test-level guidance belongs in `.agents/skills/tdd/SKILL.md` or `.agents/agents/test-engineer.md`.
- E2E commands and flake triage belong in `.agents/skills/e2e/SKILL.md`.
- Runtime debugging belongs in `.agents/skills/debug/`.
- Backend/frontend conventions belong in the closest scoped `AGENTS.md`.
- Repeated fragile commands should become or update a `scripts/` helper, with syntax and mocked behavior checks.

## Deduplication Checklist

Before adding text:

1. `rg` for the key command, error string, or concept.
2. If the guidance already exists, strengthen the existing paragraph instead of adding a duplicate.
3. If two skills now cover the same workflow, route one to the other or delete the obsolete one.
4. Update references in router skills such as `using-agent-skills` when names change.

## Output Contract

Summarize the change as:

- Learning recorded: one sentence.
- Files changed: paths.
- Validation: commands run.
- Residual risk: anything not verified.
