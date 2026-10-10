---
id: "06-hydrate-turns-by-message-window"
title: "Load turn context with each message window"
status: complete
wave: 6
depends_on:
  - "05-bound-route-boot-data"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-JOURNEY-LOADING-002
acceptance_criteria:
  - AC-PLATFORM-JOURNEY-LOADING-002.1
  - AC-PLATFORM-JOURNEY-LOADING-002.2
  - AC-PLATFORM-JOURNEY-LOADING-002.3
  - AC-PLATFORM-JOURNEY-LOADING-002.4
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
---

# Task 06: Load turn context with each message window

## Summary

Boot returns at most 51 turn rows for 50 messages. Client cold/reconnect reads return at most 101 for their existing 100-message window. Adding older turns from 200 to 2,000 does not change the initial set.

## In scope

Add optional include_turns to HTTP/WS message listing and return scoped turns/coverage from one reader snapshot. Reuse the same builder in boot.
Load only turns referenced by each returned message window plus the active turn. Integrate older-page loading, search jumps, live completion, and reconnect repair.
Replace unconditional full-history hydration with per-window coverage. Keep the existing full-turn endpoint and older-backend fallback. Audit all consumers of loadedBySession so partial coverage never claims global completeness. Document the additive request/response option through docs-maintainer.

## Out of scope

Other work orders, live installation mutation, unrelated refactors, pool increases, new runtime flags, and deployment.

## Acceptance

- Boot returns at most 51 turn rows for 50 messages. Client cold/reconnect reads return at most 101 for their existing 100-message window. Adding older turns from 200 to 2,000 does not change the initial set.
- Pagination/search retain correct grouping, duration/usage, and active-turn state. A concurrent completion or late snapshot cannot resurrect stale state.
- Boot hydration avoids an immediate full-turn refetch; legacy clients retain existing behavior and older-server fallback is bounded by scoped readiness.

## ASCII UI preview

[UI-02: full composition and states](plan.md#ascii-ui-preview). Applies to the acceptance IDs in this work order.

```text
UI-02 desktop: [Agent A | Agent B | Agent C] [optional visible split]
               [Visible chat + draft]        [Visible chat + draft]
UI-02 phone:   [Task / session picker]
               [One chat scroll region]
               [Composer + retained draft]
```

Existing controls and scroll ownership remain. Detail demand follows visibility, not mounting or keyboard focus. Preview spacing is illustrative.

## Verification

Use TDD. New test filenames and named methods below are planned deliverables, not existing passing evidence.
Run from the repository root. Install `apps/` dependencies first only if absent. Each command is independently rooted.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/handlers ./internal/backendapp -run 'Test(MessageTurnWindow|.*ListMessages|.*ListTurns|Boot|.*Boot)' -count=1)
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestMessageTurnWindowPostgres$' -count=1 -v)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-turns-hydration.test.ts hooks/domains/session/use-session-turns.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-messages.live-refresh.test.tsx lib/ssr/session-page-state.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/message-turn-window.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-message-turn-window.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/session/use-session-messages.ts hooks/domains/session/use-session-turns-hydration.ts hooks/domains/session/use-session-turns-hydration.test.ts lib/api/domains/session-api.ts lib/state/slices/session/turn-actions.ts lib/state/slices/session/types.ts e2e/tests/session/message-turn-window.spec.ts e2e/tests/session/mobile-message-turn-window.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the web typecheck and targeted ESLint for every changed TS/TSX file after the listed tests. If localized copy changes, run `pnpm run i18n:check` and `pnpm run i18n:ratchet` from `apps/web`. Managed E2E commands build fresh assets and enforce resource limits.

Use a disposable PostgreSQL database with `KANDEV_TEST_POSTGRES_DSN`. Never use the live database. A missing server or skipped test is an incomplete engine gate.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/message_turn_window_test.go (new)`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/requests.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_list_handlers_test.go (existing handler coverage)`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/use-session-turns-hydration.ts`
- `apps/web/hooks/domains/session/use-session-turns-hydration.test.ts`
- `apps/web/lib/api/domains/session-api.ts`
- `apps/web/lib/state/slices/session/turn-actions.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/e2e/tests/session/message-turn-window.spec.ts (new)`
- `apps/web/e2e/tests/session/mobile-message-turn-window.spec.ts (new)`

## Dependencies

[Task 05](task-05-bound-route-boot-data.md)

## Risks

Messages and turns require a coherent observation. A message page can reference old sparse turns; last-N turns is not equivalent. Coverage retention must follow existing message retention.

## Parallelism

`sequential`

## Inputs

- [Plan, contract inventory, and test mapping](plan.md).
- [Journey loading design](../../specs/platform/system-design/journey-data-loading.md).
- [Measured baseline](evidence.md) and `evidence/` artifacts.
- Read the owned source and nearby tests before the first edit. Preserve existing user changes.

## Results

Implemented scoped turns and coverage in HTTP/WS message windows and boot. The
50-message repository window remains at 51 turns after unrelated history grows
from 200 to 2,000 turns. PostgreSQL 18 parity passed on a disposable database.
Client coverage, legacy fallback, pagination, stale response, live completion,
and reconnect tests passed. Desktop and phone E2E preserve complete history
access with a 50-message boot and 100-message client window. The core recovery
callback uses that window without a separate full-turn fetch. Public WebSocket
documentation describes the additive option. See [implementation evidence](implementation-evidence.md#task-06-message-window-turn-context)
for commands, fixture corrections, and results.

### Review correction

Window reconciliation clears markers whose merged row proves completion or retirement. A null active-turn response is authoritative only for its captured request-start identity, epoch, and freshness. Late responses preserve newer WebSocket turns. Initial fetch, gap repair, pagination, route enrichment, and hydration regressions cover both directions of the race. See the [review correction results](implementation-evidence.md#review-remediation).
