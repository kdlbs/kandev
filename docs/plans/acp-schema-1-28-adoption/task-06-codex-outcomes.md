---
id: "06-codex-outcomes"
title: "Apply Codex child outcomes"
status: done
wave: 3
depends_on:
  - "05-control-labels"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-001
acceptance_criteria:
  - AC-AGENTS-SUBAGENT-LIFECYCLE-001.1
  - AC-AGENTS-SUBAGENT-LIFECYCLE-001.2
  - AC-AGENTS-SUBAGENT-LIFECYCLE-001.3
  - AC-AGENTS-SUBAGENT-LIFECYCLE-001.4
  - AC-AGENTS-SUBAGENT-LIFECYCLE-001.5
  - AC-AGENTS-SUBAGENT-LIFECYCLE-001.6
system_design:
  - ../../specs/agents/system-design/subagent-provider-lifecycle.md
---

# Task 06: Apply Codex child outcomes

## Summary

Read `agentsStates` from completed `wait` and `closeAgent` frames and update each known child's existing card with its status and result.

## In scope

- Control-tool tracking and `codexControlCardUpdates` in `handleACPUpdate`.
- Sanitized captured parallel and interrupted frames in testdata.
- The Codex row of the nesting table in `apps/backend/internal/agentctl/AGENTS.md`.

## Out of scope

- Parsing child output beyond `agentsStates`.
- Background-work attestation.

## Acceptance

- Replayed parallel frames end with both cards `completed` and their own results.
- Replayed interrupt frames end with the card `shutdown`, and a timed-out `wait` changes nothing.

## ASCII UI preview

UI-03: Subagent card result. See the [full preview](plan.md#ascii-ui-preview).

```text
[>] general-purpose  Read a.txt   gpt-5.6-luna
    Result: The command output was: apple
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

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/codex_v2_subagent_outcomes_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/`
- `apps/backend/internal/agentctl/AGENTS.md`

## Dependencies

Task 05

## Risks

- A background child with no `wait` keeps `pendingInit`, and only the turn-active check clears its spinner.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/agents/system-design/subagent-provider-lifecycle.md)
- `apps/backend/internal/agentctl/AGENTS.md`, `apps/web/AGENTS.md`
- Captures in `/tmp/acp-pr7/`, or a recapture with `acpdbg`

## Results

Added per-session tracking for validated Codex `wait` and `closeAgent` tool calls. Completed `agentsStates` frames now update only existing correlated cards, preserve status rank, attach each child's result, and interpret a still-running child reported by `closeAgent` as shutdown. Card updates are emitted before the control row's own update and do not attest background work.

Added sanitized parallel and interrupted captures. Replay tests cover sibling isolation, completed results, the empty timed-out wait, close-to-shutdown, unknown children, nonterminal state after a terminal result, and preservation of the control rows. The full ACP transport and streams race suite, changed-file Go lint, docs/spec validation, and `git diff --check` passed.
