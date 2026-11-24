---
id: "07-copilot-dialect"
title: "Add the Copilot subagent dialect"
status: done
wave: 3
depends_on:
  - "05-control-labels"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-002
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-004
acceptance_criteria:
  - AC-AGENTS-SUBAGENT-LIFECYCLE-002.1
  - AC-AGENTS-SUBAGENT-LIFECYCLE-002.2
  - AC-AGENTS-SUBAGENT-LIFECYCLE-002.3
  - AC-AGENTS-SUBAGENT-LIFECYCLE-002.4
  - AC-AGENTS-SUBAGENT-LIFECYCLE-002.5
  - AC-AGENTS-SUBAGENT-LIFECYCLE-002.6
  - AC-AGENTS-SUBAGENT-LIFECYCLE-004.1
system_design:
  - ../../specs/agents/system-design/subagent-provider-lifecycle.md
---

# Task 07: Add the Copilot subagent dialect

## Summary

Add a Copilot dialect that shows subagent cards and nests child calls only when the link is safe. Add its control-operation entries and show its reported type, name, description, and prompt.

## In scope

- `newCopilotACPDialect`, `subagentFrame`, and the `subagentResult` hook.
- Binding state in `adapter_copilot_subagents.go`.
- Copilot-only name and prompt presentation; other providers retain their current presentation.
- Conservative nesting for unknown background IDs and self-parent exclusion.
- Sanitized captured foreground, parallel, and background frames.
- A Copilot row in the nesting table in `apps/backend/internal/agentctl/AGENTS.md`.

## Out of scope

- Filtering child replies from assistant text.
- Synthetic grouping of unattributed calls.

## Acceptance

- The parallel capture produces two cards and no misattributed child call.
- The background capture nests its child through the parsed ID.
- A non-Copilot frame with `agent_type` stays generic.

## ASCII UI preview

UI-03: Subagent card. See the [full preview](plan.md#ascii-ui-preview).

```text
[>] explore  reader  Read a.txt   background
    expanded:
    Description: Read a.txt contents
    Prompt: Read a.txt and report its contents.
    Result: apple
    +- Viewing a.txt
```

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/...)
bash scripts/lint-go-changed <changed-go-files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_copilot.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_copilot_subagents.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_copilot_test.go`
- `apps/backend/internal/agentctl/AGENTS.md`

## Dependencies

Task 05

## Risks

- Copilot's parent frame has no `_meta`, so recognition depends on `agent_type` and `prompt` staying in `rawInput`.
- Adding a dialect may change other Copilot behavior if the default dialect had implicit assumptions; existing tests must still pass.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/agents/system-design/subagent-provider-lifecycle.md)
- `apps/backend/internal/agentctl/AGENTS.md`, `apps/web/AGENTS.md`
- Captures in `/tmp/acp-pr7/`, or a recapture with `acpdbg`

## Results

Complete. Copilot foreground, parallel, and background fixture tests passed in
the ACP/stream race suite. Regression tests cover unknown background identities
and exclude the current card from foreground parent candidates. Copilot-only UI
identity/prompt and generic-provider fallback tests passed. Desktop and phone
subagent E2E, web lint, changed-file Go lint, and specification validation passed.
