---
id: "06-document-autopilot"
title: "Document the autopilot public contract"
status: done
wave: 3
depends_on:
  - "01-persist-task-contract"
  - "02-derive-runtime-contract"
plan: "plan.md"
spec: "../../specs/tasks/requirements/autopilot-mode.md"
---

# Task 06: Document the Autopilot Public Contract

## Acceptance

- Public automation and coordination docs describe `autopilot` creation, its false
  default and immutability, compatible session boundary, and visible UI state.
- Public MCP guidance describes the `kanban-task`, `office-task`, `configuration`,
  and `external` surfaces, and explains that context-specific capability groups
  control optional tools.
- Agent communication docs give accurate ask-parent and correlated-reply payloads,
  direct-parent authorization, immediate turn ending, top-level behavior, and stale
  answer semantics. It states that an autopilot root receives no question tool.
- Examples never advertise the operator clarification tool to autopilot tasks or
  task creation through the Office surface, and all public-doc validation passes.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
rtk rg -n "autopilot|ask_parent_question_kandev|reply_to_question_id" docs/public docs/specs docs/decisions
```

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/public/agent-communication.md`
- `docs/public/coordination.md`

## Dependencies

- Task 01 fixes the creation field and compatibility behavior.
- Task 02 fixes the public tool names and runtime inventory.

## Parallelism

Can run alongside Task 03 after the payload names in the spec are treated as fixed.
If implementation changes a public field or error, update the spec and this task
before final validation.

## Inputs

- Spec sections `Creation API`, `Parent question protocol`, `State model`, and `Permissions and boundaries`.
- Existing public MCP creation and parent/child communication documentation.

## Output contract

Report sections/examples changed, exact public request/response shapes documented,
compatibility and scope callouts, validation commands/results, and any internal-only
state intentionally omitted from public docs.

## Results

Done. Updated automation/MCP, agent communication, and coordination pages with the
short creation parameter, profile/capability rules, parent-question and correlated
reply payloads, direct-parent ownership, root behavior, and waiting semantics. Public
docs validation passes (58 tests; 41 published pages).
