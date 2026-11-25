---
id: "04-provider-notices"
title: "Show provider notices"
status: blocked
wave: 3
depends_on:
  - "03-compaction-entries"
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-003
  - REQ-AGENTS-ACP-SESSION-EVENTS-004
acceptance_criteria:
  - AC-AGENTS-ACP-SESSION-EVENTS-003.1
  - AC-AGENTS-ACP-SESSION-EVENTS-003.2
  - AC-AGENTS-ACP-SESSION-EVENTS-003.3
  - AC-AGENTS-ACP-SESSION-EVENTS-003.4
  - AC-AGENTS-ACP-SESSION-EVENTS-003.5
  - AC-AGENTS-ACP-SESSION-EVENTS-004.1
  - AC-AGENTS-ACP-SESSION-EVENTS-004.2
  - AC-AGENTS-ACP-SESSION-EVENTS-004.3
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
---

# Task 04: Show provider notices

## Summary

Advertise `session.notices` and show each notice as a status row that never changes session state.

## In scope

- `EventTypeSessionNotice` from the adapter through lifecycle to `handleSessionNoticeEvent`.
- `Session.Notices` in `clientCapabilitiesForAgent`.
- Retention-proof classification and failure-path exclusion for `notice: true`.
- Mock-agent `notice` scenario and E2E.

## Out of scope

- Toasts or badges.
- Deduplicating by notice text.

## Acceptance

- An error-severity notice leaves session state and turn unchanged.
- A notice delivered before a reconnect produces one row. If the durable cursor cannot prove this, stop and report the gap.

## ASCII UI preview

UI-02: Notice rows. See the [full preview](plan.md#ascii-ui-preview).

```text
(i) Info title
/!\ Rate limit approaching                                                     >
    expanded: Retry later
(x) Error title
```

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/...)
(cd apps/backend && go test -trimpath -count=1 ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/... ./internal/task/repository/sqlite/...)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/status-message.test.tsx lib/state/slices/session/running-notice-activity.test.ts)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/session-notice.spec.ts
bash scripts/lint-go-changed <changed-go-files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_events.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_helpers.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_proof.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/web/components/task/chat/messages/status-message.tsx`

## Dependencies

Task 03

## Risks

- Claude's bold transcript fallback disappears, so a dropped event loses the notice.
- A status row must not count as agent activity for a running notice.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/agents/system-design/acp-session-events.md)
- `apps/backend/internal/agentctl/AGENTS.md`, `apps/web/AGENTS.md`
- Captures in `/tmp/acp-pr7/`, or a recapture with `acpdbg`

## Results

Blocked at the durable-cursor acceptance condition. A notice would be published
to the orchestrator before the generic inbox projector commits its cursor. The
status row is persisted separately, so a crash between those operations can
either replay and duplicate the row or advance the cursor before the row is
written and lose it. The lifecycle payload also does not expose delivery stream
and sequence identity for an idempotent row key. The canonical projection path
does write message content and the cursor in one transaction, but it does not
support status rows. Adding the notice to the retention allowlist alone would
not prove that a row exists before its payload is pruned.

The required implementation boundary is atomic status-row persistence with the
inbox cursor, using durable event identity, plus retention proof that the row
exists. Resume this work after that boundary is designed and available. No
production changes were made for Task 04.
