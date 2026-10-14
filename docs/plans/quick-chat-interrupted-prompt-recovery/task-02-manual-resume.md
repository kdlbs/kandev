---
id: "02-manual-resume"
title: "Existing Resume recovery"
status: done
wave: 2
depends_on:
  - "01-workspace-access"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Existing Resume recovery

## Summary and scope

Route the existing manual Resume hook through `session.recover` action `resume`, returning
its launch-shaped response. Preserve automatic launch semantics, guarded state updates,
native tokens and no-resend behavior. No new controls, confirmations, schema or recovery actions.

## Acceptance

1. Manual Resume succeeds through explicit recovery; automatic open/focus still uses launch.
2. Native resume preserves native identity and unknown history without sending the old prompt;
   failed recovery keeps its block. Existing accepted-work/duplicate-ID behavior still passes.
3. Quick Chat resume and a distinct follow-up instruction pass on desktop/phone after reload,
   including a repository-free workspace; stale replies cannot affect another session.

## Verification

From repository root, sequentially:

```bash
pnpm --dir apps install --frozen-lockfile
(cd apps/web && pnpm exec vitest run lib/services/session-recovery-service.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption-manual-recovery.test.ts hooks/domains/session/use-session-recovery-actions.test.ts)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -count=1)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/session/use-session-resumption.ts lib/services/session-recovery-service.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/quick-chat-resume-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-quick-chat-resume-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/session/use-session-resumption.ts` and related tests
- `apps/web/lib/services/session-recovery-service.ts` and its test
- `apps/backend/internal/orchestrator/session_native_delivery_recovery_test.go`
- `apps/web/e2e/tests/chat/quick-chat-resume-recovery-helpers.ts`
- `apps/web/e2e/tests/chat/quick-chat-resume-recovery.spec.ts`
- `apps/web/e2e/tests/chat/mobile-quick-chat-resume-recovery.spec.ts`

## Dependencies and risks

Task 01. Preserve archive checks and existing request-identity rollback. Do not override
live durable recovery or invoke history continuation automatically.

## Parallelism

`sequential`

## Inputs

[Design](../../specs/platform/system-design/interrupted-prompt-recovery.md), manual resume.

## Results

- Hook regression failed before implementation because manual Resume used ordinary launch.
- Focused Vitest: 4 files, 91 tests passed. Service-only verification after test cleanup: 18 tests passed.
- Full orchestrator suite with `go test -trimpath -race ./internal/orchestrator -count=1`: passed (149.448s).
- Frontend typecheck and ESLint with zero warnings across changed web files: passed.
- Managed Chromium E2E: 1 passed. Managed mobile Chrome E2E: 1 passed.
  Both restore an uncertain repository-free workspace, reload, use existing Resume, preserve
  native identity and the unknown submission, and send a distinct follow-up without replay.
- The fixture creates a separate unknown submission because retained terminal evidence correctly
  settles a completed submission during durable replay. Wait for the recovery response before
  asserting block resolution; the waiting session state is published earlier.
- Documentation catalog/specification lint, 36 spec-linter tests, and `git diff --check`: passed.
- No public documentation change is needed: existing Resume semantics and controls are retained.
- No screenshots are required: request routing and admission changed; rendered markup is unchanged.
- PR publication is tracked in the task handoff, outside this source verification record.
