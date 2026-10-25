---
id: "01-harness-planning-gate"
title: "Harness planning gate"
status: done
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 01: Harness planning gate

## Acceptance

- Repo-wide guidance states that workflow-generated implementation envelopes
  do not opt features or behavior-changing fixes out of spec-driven
  development.
- The spec-driven-development skill requires approved artifacts or an explicit
  user instruction to skip planning before production/permanent test edits.
- The embedded Improve Kandev prompt carries the same planning gate before its
  implementation checklist.

## Verification

```bash
git diff --check -- AGENTS.md .agents/skills/spec-driven-development/SKILL.md apps/backend/config/workflows/improve-kandev.yml
rg -n "Workflow-generated phase text|Prompt-Precedence Gate|PLANNING GATE" \
  AGENTS.md .agents/skills/spec-driven-development/SKILL.md \
  apps/backend/config/workflows/improve-kandev.yml
cd apps/backend && go test ./config/workflows -run TestLoadTemplates_AllValid -count=1
```

## Files likely touched

- `AGENTS.md`
- `.agents/skills/spec-driven-development/SKILL.md`
- `apps/backend/config/workflows/improve-kandev.yml`

## Dependencies

None.

## Parallelism

Sequential; completed during the planning session as an explicit harness
improvement.

## Inputs

- Root **Single-Session Model Workflow** guidance.
- `.agents/skills/harness-improvement/SKILL.md`.
- `.agents/skills/spec-driven-development/SKILL.md`.

## Output contract

Report the learning recorded, exact files changed, validation commands/results,
and any remaining ambiguity in what counts as an explicit planning opt-out.

## Recorded result

- Learning recorded: workflow-generated implementation language is Phase 5
  guidance and is not an explicit opt-out from feature planning.
- Markdown/YAML diff checks passed.
- All three guardrail markers were found in their intended files.
- `go test ./config/workflows -run TestLoadTemplates_AllValid -count=1`
  passed.
