---
id: "03-recovery-experience"
title: "Present and prove startup recovery"
status: complete
wave: 3
depends_on:
  - "02-bounded-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.8
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 03: Present and prove startup recovery

## Summary

Expose automatic retry through existing startup progress and accurate final recovery cards.
Prove real startup recovery on desktop, phone, local Docker, and SSH.

## In scope

- Existing boot progress and failure-card integration for managed_runtime_startup.
- All seven locales, truthful attempt/cause wording, one final Retry runtime action.
- Deterministic subprocess E2E fixtures, mixed burst tests, and public recovery guidance.

## Out of scope

- New pages, dialogs, launch settings, live registry tests, or provider credentials.

## Acceptance

1. Retry progress appears while the original session stays in startup. Success has no failure card and delivers one prompt.
2. Exhaustion yields one correctly classified card. Early exit is not labeled an npm error; failed cleanup does not claim a second attempt ran.
3. Desktop/phone and supported executor tests prove recovery, cancellation, identity, and preservation of a live sibling's shared files.

## UI-01: Automatic startup retry

Follow the [combined preview](plan.md#ui-01-automatic-startup-retry).

```text
Pending, desktop/phone: [ Retrying agent startup (attempt 2 of 2) ... ]
Final, desktop: [ Cause and retry result ] [ Retry runtime ] [ Details > ]
Final, phone:   [ Cause and retry result ]
                [ Retry runtime         ]
                [ Details >             ]
```

Use the current boot/status area and recovery card. Retain the existing chat scroll owner and phone safe-area spacing.
Keep phone actions at least 44px and desktop controls at their existing density. No new navigation or overlay.
Criteria: 004.5 and 004.8.

## Tests and fixtures

Add backend `TestManagedStartupProgress` metadata tests and focused recovery model/component tests before wiring new copy.
Use `npm_transient`, `early_exit`, `cleanup_failed`, and actual attempt counts from typed metadata; do not parse diagnostic prose in React.
Known final policy/auth failures keep their existing specialized recovery presentation.

Extend `e2e/fixtures/managed-runtime-npx.sh` with explicit modes and per-launch atomic attempt accounting.
Keep shared cache/prefix identity, a live sibling sentinel, and distinct launch IDs.
Remove the fixture's artificial rule requiring a stale marker to be deleted before online retry.
Add desktop `tests/session/managed-runtime-startup-retry.spec.ts` and phone `mobile-managed-runtime-startup-retry.spec.ts`.
Cover mixed concurrent outcomes, silent first exit, transient npm error, repeated failure, permanent refusal, and cancellation during backoff.
Assert server launch counters, session IDs, prompt counts, visible progress, and final cards.

Extend existing Docker/SSH recovery specs and host-utility recovery coverage for non-destructive retries.
Update public `agents-and-profiles.md` recovery and troubleshooting sections together; remove automatic tree-deletion claims.
Keep exact-version selection, registry preservation, supported executor limits, and explicit maintenance guidance.
Do not advertise a confirmed npm race.

## Verification

Run sequentially from the repository root. Install dependencies once if missing.

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestManagedStartupProgress' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/managed-runtime-startup-recovery.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
make build-backend
make build-web
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/managed-runtime-startup-retry.spec.ts tests/session/managed-runtime-npm-recovery.spec.ts tests/settings/host-utility-managed-runtime-recovery.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-managed-runtime-startup-retry.spec.ts tests/session/mobile-managed-runtime-npm-recovery.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --shards 1 --project containers tests/docker/managed-runtime-npm-recovery.spec.ts tests/ssh/managed-runtime-npm-recovery.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the new component test filename above. If fixture placement changes, update commands before running them.
Do not mark container support verified from a skipped Docker/SSH run. Record the blocker and leave that validation pending.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`, `manager_startup.go`, and focused progress tests
- `apps/web/components/task/chat/session-recovery-model.ts`
- `apps/web/components/task/chat/session-bootstrap-recovery-model.ts` and its consumers where boot metadata is rendered
- `apps/web/components/task/chat/managed-runtime-startup-recovery.test.tsx` (new)
- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/simple/components/run-error-entry.tsx` and `managed-runtime-npm-run-error.tsx`
- `apps/web/lib/active-session-recovery.ts` if failure-kind mapping requires it
- `apps/web/src/locales/*/chat.json` and generated locale artifacts
- `apps/web/e2e/fixtures/managed-runtime-npx.sh`
- New desktop/phone startup-retry specs and existing managed-runtime recovery specs named above
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 02.

## Risks

Seeded failure cards cannot prove process retry. A shared fixture counter can hide cross-session interference.
Any final specialized failure must keep its correct action instead of being flattened into a generic startup card.

## Parallelism

`sequential`

## Inputs

- [Design: Presentation and diagnostics](../../specs/agents/system-design/managed-npm-runtime-recovery.md#presentation-and-diagnostics).
- Existing managed npm desktop/mobile recovery cards, executor fixtures, `/e2e`, `/mobile-parity`, and `/docs-maintainer`.

## Results

Completed. Startup progress and final recovery presentation use the typed attempt, cause, and cleanup evidence from the original session. Automatic retries preserve the session and shared runtime tree; exhausted recovery shows one actionable card. Public guidance describes supported behavior without claiming a confirmed npm race.

Verification passed:

- `go test ./internal/agent/runtime/lifecycle -run 'TestManagedStartupProgress' -count=1`.
- `pnpm exec vitest run components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/managed-runtime-startup-recovery.test.tsx`: 16 tests passed.
- `pnpm run typecheck` and `pnpm run i18n:check`.
- Backend and web builds; the host E2E runner also rebuilt both.
- Desktop managed-runtime E2E group: 7 passed.
- Mobile managed-runtime E2E group: 4 passed. The startup-progress case opens the task before waiting for completion so the transient progress row remains observable.
- Docker and SSH managed-runtime E2E group with `KANDEV_E2E_CONTAINERS=1`: 2 passed on their real executors, no skips.
- Public docs validator tests: 62 passed; 47 public pages validated.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

Issue 4152's original process-exit cause remains unconfirmed. This work adds bounded recovery for the proven eligible startup failure classes; it does not claim to identify that incident's initial exit cause.

### PR review follow-up (2026-10-02)

The review pass corrected public guidance to require complete, generation-matched evidence for ordinary-exit retry; unclassified npm codes and truncated diagnostics fail closed. UI review changes now preserve structured bootstrap causes for managed-runtime cards and share the card shell between npm and general startup failures. The mobile success E2E confirms the agent reply remains in the same conversation.

Final PR-review validation:

- Desktop startup-retry E2E: 4 passed on full rerun. The preceding run had three passes and one backend-fixture readiness failure during teardown; the cancellation case then passed alone before the full rerun.
- Mobile startup-retry E2E: 2 passed.
- Docker and SSH startup-recovery E2E: 2 passed on the real executors, with no skips.
- The managed-runtime component suite passed 61 tests across the session recovery model, startup recovery card, and action message. After the complexity refactor, the focused startup card suite passed 5 tests; TypeScript typecheck and i18n validation passed.
- Backend `golangci-lint` passed with zero issues. The commit hooks passed documentation, spec, formatting, scoped lint, web lint, i18n, E2E sleep, public-copy, and commit-message checks.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and public-document validation passed.

On 2026-10-03, the first PR CI attempt failed the lifecycle package at its 10-minute test timeout. The exact local race-and-coverage command passed in 105 seconds. A retry passed both Linux backend test shards and every Windows test step, but the Windows job reached its 40-minute limit during Go cache cleanup. A 45-minute retry also passed every Windows test step but reached that limit while cleanup was still running. The Windows job now allows 50 minutes, and its contract test checks that bounded cleanup buffer. A full current-head CI run remains pending.
