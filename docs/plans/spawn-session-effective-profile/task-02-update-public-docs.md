---
id: "02-update-public-docs"
title: "Update public MCP documentation"
status: done
wave: 2
depends_on: ["01-propagate-effective-profile"]
plan: "plan.md"
spec: "../../specs/agents/requirements/spawn-session-effective-profile.md"
---

# Task 02: Update public MCP documentation

## Intent

Document the effective profile response on the public Agent Communication explanation page.

## Inputs

- The spec `API surface` and `Scenarios` sections.
- The final tool description from Task 01.
- The existing `spawn_session_kandev` sections in `docs/public/agent-communication.md`.

## Acceptance

- Both documented success shapes include `agent_profile_id`.
- The page states that the field is the effective profile after workflow profile resolution.
- The page does not imply that an explicit profile always launches on a workflow step.
- The public documentation checks pass.

## Files likely touched

- `docs/public/agent-communication.md`

## Verification

```bash
cd "$(git rev-parse --show-toplevel)" && rg -n "spawn_session_kandev|agent_profile_id" docs/public/agent-communication.md && node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

## Dependencies

Task 01.

## Parallelism

`sequential`. The documented response must match the completed backend contract.

## Output contract

Report the changed page, its explanation classification, each command result, blockers, and risks. Update this task and `plan.md` in the same conversation.

## Results

- Updated `docs/public/agent-communication.md` with both documented
  `spawn_session_kandev` success shapes and the effective-profile precedence.
- `rg -n "spawn_session_kandev|agent_profile_id"
  docs/public/agent-communication.md` found the updated contract.
- `node --test scripts/validate-public-docs.test.mjs` passed all 60 tests.
- `node scripts/validate-public-docs.mjs` validated 41 published pages.
- Generated artifacts and temporary capture files: None. External side
  effects: None.
