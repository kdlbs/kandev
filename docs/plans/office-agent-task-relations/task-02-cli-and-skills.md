---
id: "02-cli-and-skills"
title: "Expose supported relation flags in the CLI and skills"
status: done
wave: 2
depends_on:
  - "01-runtime-create-relations"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENT-TASK-RELATIONS-004
acceptance_criteria:
  - AC-OFFICE-AGENT-TASK-RELATIONS-004.1
  - AC-OFFICE-AGENT-TASK-RELATIONS-004.2
  - AC-OFFICE-AGENT-TASK-RELATIONS-004.4
  - AC-OFFICE-AGENT-TASK-RELATIONS-004.5
system_design:
  - ../../specs/office/system-design/agent-task-relations.md
---

# Task 02: Expose Supported Relation Flags in the CLI and Skills

## Summary

Make `agentctl kandev task create` send `--blocked-by` and `--workspace-mode`,
and keep refusing the four flags the runtime still rejects, as it does today. Update the Office skills, context, specs, and public docs that
describe the command.

## In scope

- `taskCreate`: keep the `--priority`, `--workspace-group-id`,
  `--default-child-workspace`, and `--default-child-ordering` definitions and
  limit the reject-before-send loop to them. Parse
  `--blocked-by` (split, trim, reject empty entries) into a JSON array. Send a
  non-empty `--workspace-mode`. Update help text to the accepted values.
- CLI tests: payload shape for each flag, the existing
  `--<flag> is not supported by Office runtime task create` refusal for each
  still-refused flag with no request sent, an empty `--blocked-by` entry failing before send,
  and server `400` / `403` bodies surfacing through `handleResponse`.
- Skills: `kandev-protocol/SKILL.md` (example and command table) and
  `kandev-task-ops/references/tasks.md`. Document the flags, the blocker scope
  rule, a fan-in example, a `new_workspace` batch-child example, and the
  parent-waits-for-children stall to avoid.
- `config/prompts/office-context.md`, if it lists `task create` flags.
- Specs: the `agents-02.md` CLI synopsis and the `agents-03.md` scenario that
  says the flags are rejected. Point both at the new design.
- Public docs: the Office `task create` example in
  `docs/public/automation-and-mcp.md`, via `/docs-maintainer`.

## Out of scope

- Server behavior (work order 01) and wake admission (`REQ-OFFICE-SCHEDULER-002`).
- MCP `create_task_kandev` and Kanban docs.

## Acceptance

- CLI tests prove the two flags reach `/api/v1/office/runtime/tasks` with
  the documented JSON keys, and that the still-refused flags fail with their
  existing message before any HTTP request.
- Skill and prompt sync tests pass, and no bundled skill mentions `--priority`,
  `--workspace-group-id`, `--default-child-workspace`, or
  `--default-child-ordering` for `task create`.
- Spec catalog and linters pass with the updated `agents-02` / `agents-03`
  text.

## Verification

From `apps/backend`:

```bash
rtk go test ./cmd/agentctl/... -run 'TaskCreate' -race -count=1
rtk go test ./internal/office/skills/... ./internal/mcp/server/... -run 'Sync|Sysprompt' -race -count=1
rtk proxy grep -rn -e '--priority' -e '--workspace-group-id' -e '--default-child-workspace' -e '--default-child-ordering' internal/office/configloader/skills config/prompts
```

The grep must print nothing. From the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/cmd/agentctl/kandev_task.go`
- `apps/backend/cmd/agentctl/kandev_task_create_test.go` (new; `kandev_test.go` is over 800 lines)
- `apps/backend/internal/office/configloader/skills/kandev-protocol/SKILL.md`
- `apps/backend/internal/office/configloader/skills/kandev-task-ops/references/tasks.md`
- `apps/backend/config/prompts/office-context.md`
- `docs/specs/office/system-design/agents-02.md`
- `docs/specs/office/system-design/agents-03.md`
- `docs/public/automation-and-mcp.md`

## Dependencies

Work order 01. The CLI must not advertise fields before the server accepts
them.

## Risks

- Changing skill content can trip the system-skill content-hash sync. Run the
  sync tests and follow their regeneration instructions if they fail.
- Agent-facing copy is English and is not localized UI. No i18n catalog
  changes.

## Parallelism

Sequential after 01.

## Inputs

- `docs/specs/office/system-design/agent-task-relations.md`: sections CLI
  surface and Agent-facing documentation.
- Existing `taskCreate` tests in `cmd/agentctl/kandev_test.go`.

## Results

- `taskCreate` keeps `--blocked-by` and `--workspace-mode` and no longer defines
  `--priority`, `--workspace-group-id`, `--default-child-workspace`, or
  `--default-child-ordering`. `parseBlockedBy` rejects empty entries before any
  request.
- Replaced the obsolete "reject everything" and shared-group CLI tests; new cases
  are in `cmd/agentctl/kandev_task_create_test.go`.
- `kandev-protocol` documents both flags, the blocker scope rule, a fan-in example,
  a `new_workspace` child example, and the parent/child stall to avoid.
  `kandev-escalation` now says the CLI cannot add a blocker to an existing task.
  `kandev-task-ops/references/tasks.md` and `office-context.md` do not document
  `task create`, so they were unchanged.
- `system_sync_test.go` contract tests updated to the new supported flag set.
- Specs `agents-02.md` and `agents-03.md` and public
  `docs/public/automation-and-mcp.md` updated. Public docs validators passed.
- Verification: `go test ./cmd/agentctl/...` (also under `-race`) and
  `go test ./internal/office/skills/...` passed; the removed-flag grep printed
  nothing; `list-docs.py validate` and `lint-spec-files.py --all` passed.

### Correction

- Removing the four unsupported flags was out of scope for #4547 and was
  reverted at the user's request. `taskCreate` again defines `--priority`,
  `--workspace-group-id`, `--default-child-workspace`, and
  `--default-child-ordering` with their original help text, and refuses each
  with `--<flag> is not supported by Office runtime task create` before any
  request. Whether to support or remove them is tracked in #4613.
- New regression `TestTaskCreate_StillRefusesUnsupportedFlagsWithRuntimeMessage`
  failed first (the flags were undefined) and passes after the restore.
  `AC-OFFICE-AGENT-TASK-RELATIONS-004.2` and the design's CLI surface were
  updated to match. The skills never documented these flags, so they were
  unchanged.
