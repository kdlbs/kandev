---
id: "03-compaction-entries"
title: "Show compaction entries"
status: done
wave: 2
depends_on:
  - "01-repin-sdk"
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-002
  - REQ-AGENTS-ACP-SESSION-EVENTS-004
acceptance_criteria:
  - AC-AGENTS-ACP-SESSION-EVENTS-002.1
  - AC-AGENTS-ACP-SESSION-EVENTS-002.2
  - AC-AGENTS-ACP-SESSION-EVENTS-002.3
  - AC-AGENTS-ACP-SESSION-EVENTS-002.4
  - AC-AGENTS-ACP-SESSION-EVENTS-002.5
  - AC-AGENTS-ACP-SESSION-EVENTS-002.6
  - AC-AGENTS-ACP-SESSION-EVENTS-002.7
  - AC-AGENTS-ACP-SESSION-EVENTS-002.8
  - AC-AGENTS-ACP-SESSION-EVENTS-002.9
  - AC-AGENTS-ACP-SESSION-EVENTS-002.10
  - AC-AGENTS-ACP-SESSION-EVENTS-004.1
  - AC-AGENTS-ACP-SESSION-EVENTS-004.2
  - AC-AGENTS-ACP-SESSION-EVENTS-004.3
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
---

# Task 03: Show compaction entries

## Summary

Advertise `session.compaction` and turn compaction updates into one in-place chat entry.

## In scope

- `ToolKindContextCompaction`, its payload, and display preparation.
- `adapter_session_events.go` conversion, summary accumulation, and load-replay suppression.
- `Session.Compaction` in `clientCapabilitiesForAgent`.
- `ContextCompactionMessage`, renderer and turn-group handling, and seven locale catalogs.
- Mock-agent `compaction` scenario, gated on the advertised capability, and desktop and phone E2E.

## Out of scope

- Notices.
- Changes to the compaction count.

## Acceptance

- Captured Claude and Codex frames produce one entry per identity with the expected outcome.
- A replayed load creates no entry, and the compaction count is unchanged by these events.
- Desktop and phone E2E pass.

## ASCII UI preview

UI-01: Compaction entry. See the [full preview](plan.md#ascii-ui-preview).

```text
[~] Compacting context...

[v] Context compacted                     31.3k -> 2.4k tokens   manual   26.8s  >
    expanded:
    +-------------------------------------------------------------------------+
    | Summary (markdown, bounded preview)                                     |
    +-------------------------------------------------------------------------+

[!] Context compaction failed                                                  >
    expanded: reported error text
```

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/...)
(cd apps/backend && go test -trimpath -count=1 ./internal/agentctl/server/utility/ ./cmd/mock-agent/)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck && pnpm exec vitest run components/task/chat/messages/context-compaction-message.test.tsx && pnpm run i18n:check)
(cd apps && pnpm --filter @kandev/web lint)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/context-compaction.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-context-compaction.spec.ts
bash scripts/lint-go-changed <changed-go-files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/tool_payload.go`
- `apps/backend/internal/agentctl/types/streams/tool_display_prepare.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_events.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_helpers.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/web/components/task/chat/messages/context-compaction-message.tsx`
- `apps/web/components/task/chat/message-renderer.tsx`
- `apps/web/components/task/chat/messages/turn-group-message.tsx`
- `apps/web/components/task/chat/types.ts`
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 01

## Risks

- Claude stops sending its `Compact conversation` tool call once this ships, so the entry is the only record.
- A new tool kind must pass every kind-exhaustive switch and the retention and display paths.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/agents/system-design/acp-session-events.md)
- `apps/backend/internal/agentctl/AGENTS.md`, `apps/web/AGENTS.md`
- Captures in `/tmp/acp-pr7/`, or a recapture with `acpdbg`

## Results

Passed:

- `go test -trimpath -race -count=1 ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/...`
- `go test -trimpath -count=1 ./internal/agentctl/server/utility/... ./cmd/mock-agent/...`
- Final captured-frame conversion tests with race detection, including Claude camelCase metadata and Codex status-only frames
- `make -C apps/backend build`
- Web typecheck, compaction/grouping Vitest tests, i18n checks, and full web lint
- Desktop and mobile context-compaction E2E tests; mobile viewport geometry and touch expansion passed
- Changed-file Go lint, docs/spec validation, and `git diff --check`
