---
id: "06-default-on-title-preference"
title: "Default-on title preference"
status: done
wave: 6
depends_on: ["05-single-owner-title-handoff"]
plan: "plan.md"
spec: "../../specs/tasks/agent-generated-titles.md"
---

# Task 06: Default-on title preference

## Acceptance

- New backend user settings and stored JSON that omits `agent_generated_task_titles` resolve to `true`.
  An explicit stored or patched `false` remains false, and an omitted PATCH never changes the current
  value.
- Frontend default state, boot/SSR mapping, and partial WebSocket updates use the same default-on and
  preserve-current semantics; an older response that omits the field does not disable the feature.
- Settings UI remains self-explanatory and mobile-equivalent. No new layout is introduced; the shipped
  task/subtask prompt-first surfaces simply become the default.
- Shared browser-test setup explicitly saves `false` where unrelated tests require manual title inputs,
  while dedicated title tests cover enabled-by-default and explicit opt-out behavior.

## Verification

```bash
cd apps/backend && go test ./internal/user/store ./internal/user/service ./internal/user/dto ./internal/backendapp -run 'Test.*(AgentGeneratedTaskTitles|UserSettings)'
cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts components/settings/agent-generated-task-title-settings.test.tsx components/task-create-dialog-setup.test.ts
```

## Files likely touched

- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- related user service/DTO/boot-state tests if their default assertions need expansion
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- focused settings/default-state tests
- `apps/web/e2e/fixtures/test-base.ts`

## Dependencies

Task 05 establishes the exact single-session behavior that becomes the default experience.

## Parallelism

Sequential in the primary conversation. Keeping this after Task 05 prevents enabling incomplete
multi-session behavior by default in an intermediate commit.

## Inputs

- Spec **User settings**, default/opt-out scenarios, and desktop/mobile parity requirements.
- ADR-2026-08-02 missing-versus-explicit preference decision.
- Existing pointer-backed JSON decoder and shared frontend settings mapper.

## Risks

- A hardcoded frontend fallback can overwrite explicit false during partial WS updates; use current
  state for omitted event fields and the new default only when creating initial state.
- Many E2E tests intentionally fill title inputs. Keep their common fixture explicitly opted out rather
  than rewriting unrelated scenarios around prompt-first creation.

## Result

Missing backend settings, initial frontend state, boot/SSR hydration, and partial WebSocket updates
now default to enabled while preserving an explicit `false`. The shared browser fixture explicitly
opts out so existing manual-title scenarios remain deterministic.

The affected Vitest files passed (50 tests), backend user-store tests passed, and the web lint check
passed. The broad web typecheck command is mis-scoped in this worktree and reports pre-existing
repository alias/type errors; it did not identify a regression in the changed settings files.
