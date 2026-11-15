---
id: "05-control-labels"
title: "Label subagent control operations"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-004
acceptance_criteria:
  - AC-AGENTS-SUBAGENT-LIFECYCLE-004.1
  - AC-AGENTS-SUBAGENT-LIFECYCLE-004.2
  - AC-AGENTS-SUBAGENT-LIFECYCLE-004.3
system_design:
  - ../../specs/agents/system-design/subagent-provider-lifecycle.md
---

# Task 05: Label subagent control operations

## Summary

Add the closed `SubagentControl` value and show localized labels for recognized control operations on Codex, Copilot, and Claude.

## In scope

- `GenericPayload.SubagentControl` and its dialect mapping.
- `tool-call-message.tsx` labels in seven locales.
- Mock-agent control row and E2E on desktop and phone.

## Out of scope

- Card updates from control operations.

## Acceptance

- A `wait` tool from a non-Codex agent keeps its title.
- Each mapped tool renders its label, and unknown values fall back to the title.

## ASCII UI preview

UI-04: Control operation rows. See the [full preview](plan.md#ascii-ui-preview).

```text
[v] Waited for subagents
[v] Messaged subagent
[v] Closed subagent
```

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/...)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/tool-call-message.test.tsx && pnpm run i18n:check)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/subagent.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-subagent.spec.ts
bash scripts/lint-go-changed <changed-go-files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/tool_payload.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/normalize.go`
- `apps/web/components/task/chat/messages/tool-call-message.tsx`
- `apps/web/src/locales/*/task.json`
- `apps/backend/cmd/mock-agent/scenarios.go`

## Dependencies

None

## Risks

- Copilot's mapping needs its dialect, so Task 07 adds those three entries using the helper from this task.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/agents/system-design/subagent-provider-lifecycle.md)
- `apps/backend/internal/agentctl/AGENTS.md`, `apps/web/AGENTS.md`
- Captures in `/tmp/acp-pr7/`, or a recapture with `acpdbg`

## Results

Implemented the closed control-operation payload and provider-safe label mapping for Codex and Claude. Added localized labels in all seven supported languages, deterministic mock rows, and unit plus desktop/phone E2E coverage. The browser tests expand the standard collapsed tool group before checking its rows.

Verification passed: ACP and stream race tests; mock-agent tests; the focused web Vitest suite; i18n check and ratchet; web typecheck and lint; changed-file Go lint; docs/spec validation; `git diff --check`; desktop Chromium E2E (3 passed); phone Chromium E2E (3 passed).
