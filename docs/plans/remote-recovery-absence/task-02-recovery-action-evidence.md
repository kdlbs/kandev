---
id: "02-recovery-action-evidence"
title: "Prove recovery actions after remote loss"
status: done
wave: 2
depends_on:
  - "01-settle-remote-absence"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 02: Prove recovery actions after remote loss

## Summary

Prove that guard completion reaches ordinary recovery admission through the
actual lifecycle adapter and public recovery route. Cover a real remote loss
on desktop and phone, and record the supported recovery limits in public docs.

## In scope

- Add `internal/backendapp/remote_recovery_absence_integration_test.go`.
  Compose the actual lifecycle manager, adapter, task repository, and
  orchestrator. A narrow provider fake can supply exact absence evidence.
  the guard and recovery service cannot be mocked away.
- `TestRemoteRecoveryAbsencePublicRecoveryActions` seeds an uncertain prompt
  and a separate uncertain queue dispatch. Exercise Resume, saved-context
  continuation, and Retry connection through `session.recover`. Stop uses the
  public `session.stop` route.
  Assert each action's real success or evidence-based refusal. Missing
  native or journal evidence must not become success merely because the
  startup guard finished.
- Check ordinary prompt admission remains blocked by the independent recovery
  cause. Keep original messages, payload hashes, stream/generation bindings,
  queue claim identities, and unknown outcomes intact.
- Retain unknown/live, archived, Office, automation, and changed-owner controls.
  A valid recovery request cannot bypass their own admission.
- Run the remote-loss scenario in desktop and phone contexts under
  `containers`. Capture the original execution
  identity, stop and join the test-owned backend, stop the exact task container,
  and only then copy and seed its closed journal and disposable database.
  Restart the backend while the container remains stopped.
- Use `testPage` and `SessionPage` to operate the visible Retry connection
  action. Arm the gateway `session.recover` response wait before the click or
  tap. Assert that the persisted executor row is stopped with the original
  execution ID, then await the visible delivery-recovery result with the normal
  UI assertion timeout. Before an explicit recovery action, verify the
  independent uncertain-delivery block and queue claim remain intact.
- Send public `session.recover` Resume and assert that it reaches recovery
  admission instead of returning `session_recovery_in_progress`. Preserve the
  original conversation, uncertain submission, and no-replay behavior. Allow
  the resumed agent-boot audit message. Resume may move the parked queue claim
  back to `queued_messages`; assert the same item remains pending after reload
  without dispatch.
- Retain the existing successful native-recovery desktop and phone scenarios.
  Assert a distinct later instruction and its accepted response, with the
  original interrupted prompt still represented exactly once.
- Add a short explanation in `docs/public/sessions-and-review.md` about
  definitive absence versus unreachable ownership and missing saved state.
  Keep existing labels and automatic restore-only behavior.

## Out of scope

New rendered controls, copy in the application, layout, navigation, endpoint
contracts, feature flags, automatic prompt continuation, or broader provider
death proof. If a new interface contract is necessary, update the design package
before changing that contract.

## Acceptance

1. After definitive absence, public recovery reaches its actual admission
   checks. The desktop and phone regressions assert the stopped executor row
   with the original execution identity, the visible blocked Retry result, and
   a successful public Resume response instead of
   `session_recovery_in_progress`. The uncertain submission remains uncertain.
2. Before explicit Resume, the independent block and queue dispatch claim are
   unchanged. Resume may transfer the claim into the same pending queue item;
   the test verifies that item survives reload without dispatch and that the
   original prompt is not replayed. Existing successful native restoration
   accepts one distinct new instruction without old-prompt replay.
3. Public guidance describes the tested limits. Every command and observed
   result is recorded here and in the plan before status becomes done.

## Verification

Run from the repository root. Install dependencies once before package commands
in a fresh worktree. Browser commands remain sequential and headless.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -trimpath -race -tags fts5 ./internal/backendapp -run '^TestRemoteRecoveryAbsence' -count=1)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -trimpath -race -tags fts5 ./internal/orchestrator ./internal/orchestrator/handlers -run 'TestStartupReconciliationPreservesOnlyExactPendingRemoteOwner|TestSilentRestoreSourceDeathProof|TestSessionRecoveryGuard|TestWsRecoverSession' -count=1)
bash scripts/lint-go-changed apps/backend/internal/backendapp/remote_recovery_absence_integration_test.go
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint e2e/tests/remote-docker/remote-docker-recovery.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/remote-docker/remote-docker-recovery.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/session/durable-reattachment.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-durable-reattachment.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The container selection must discover and execute both new viewport cases.
Docker-unavailable skips are not a passing result. A missing native conversation
is a valid blocked product outcome, not a test or infrastructure skip.
Use the repository PR-documentation coverage preflight for the actual final
diff, including both changed work orders and all referenced contracts.

## Files likely touched

- `apps/backend/internal/backendapp/remote_recovery_absence_integration_test.go` (new)
- `apps/web/e2e/tests/remote-docker/remote-docker-recovery.spec.ts`
- `docs/public/sessions-and-review.md`
- `docs/plans/remote-recovery-absence/plan.md`
- Both work orders in this package for status and actual results.

Existing recovery helpers and fixtures are inputs. Prefer a local helper inside
the spec for a scenario used only there. Extend shared fixture contracts only
when an existing scenario requires that change.

## Dependencies

Task 01. This task validates its integrated behavior and does not run in parallel.

## Risks

A dead port supplies unknown ownership, not definitive absence. Browser tests
must prove the intended provider result. The remote container has its own
retained state. Successful native restoration is conditional and cannot be
assumed after resource loss. Cleanup must affect only test-owned resources.

## Parallelism

`sequential`

## Inputs

- [Plan API and E2E contract](plan.md#e2e-tests).
- [Definitive remote absence design](../../specs/platform/system-design/durable-restart-reconciliation.md#definitive-remote-absence).
- [Reattachment design](../../specs/platform/system-design/durable-agent-reattachment.md),
  recovery without an execution and persisted recovery presentation.
- [Restart requirements](../../specs/platform/requirements/durable-restart-reconciliation.md).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md), REQ-006.
- `apps/backend/internal/backendapp/adapters.go` and
  `apps/backend/internal/orchestrator/handlers/handlers.go`.
- Existing `remote-docker-test-base.ts`, `backend.ts`, and
  `tests/remote-docker/remote-docker-recovery.spec.ts`.
- Existing desktop and mobile durable-reattachment tests and
  `helpers/restart-reconciliation.ts`.
- `/e2e`, `/mobile-parity`, `/docs-maintainer`, and scoped web/backend guidance.

## Results

Done. `TestRemoteRecoveryAbsencePublicRecoveryActions` passed under the race
detector for Resume, saved-context continuation, Retry connection, and Stop
through the registered websocket handlers. It composes the lifecycle manager
and adapter with real task, executor, environment, submission, recovery-block,
transcript, and queue repositories. Resume and continuation reach the fake
Remote Docker creation boundary, which refuses resource creation. Retry remains
unavailable without a retained runtime. The original uncertain delivery,
recovery block, transcript, and queue claim remain unchanged.

The prescribed orchestrator and handler race controls passed. Changed Go files
reported zero lint issues, and `make -C apps/backend build` passed for all
configured targets. The Darwin builds reported only the expected warning that
no codesigning tool was available. Web typecheck passed with an 8 GB Node heap,
and ESLint passed for the changed E2E fixtures, page object, and spec.

The `containers` project passed all four Remote Docker recovery cases: both
existing restoration/lock cases and the new desktop and phone exact-container
loss cases. The linked checkout's `.git` pointer cannot be resolved from the
runner container, so the same `containers` Playwright project ran in host mode
with the managed runner rebuilding its required assets; Docker was available
and both new viewport cases executed. Existing native reattachment passed on
desktop (2 tests) and phone (1 test). The loss fixture captured the execution identity, joined the owned
backend shutdown, stopped the exact container, then copied and seeded the
closed journal and database before restarting the backend. Both viewport cases
observed the stopped executor row with its original execution ID, awaited the
visible blocked Retry result, and confirmed the block and queue claim remained
intact before explicit Resume. The public Resume request returned successfully
instead of the startup guard error. The original conversation remained intact
apart from one resumed agent-boot audit event, and the submission remained
`interrupted_unknown`. Explicit Resume moved the queue claim into the same
pending `queued_messages` item; that item persisted after reload without
dispatch. The phone case checked the 44px action target and horizontal
overflow.

Public-doc tests passed (63), the published-doc validator passed (54 pages),
the document catalog validated (425 decisions and 1878 specifications), all
specifications passed lint, and `git diff --check` passed. The actual-diff
documentation coverage preflight reports `covered` with no errors for both
work orders and their referenced contracts.

After review findings, `TestDeferredRemoteRecoveryRetriesUntilConfirmedAbsence`
and `TestRemoteRecoveryAbsenceRespectsStopAndShutdownFences` passed 20
repetitions under the Go race detector. Both use an unactivated startup reporter
to keep background polling gated while tests drive controlled polls; no sleeps
are used.

After the review changes, the managed host command above rebuilt the E2E
artifacts and passed all four selected container tests, including both desktop
and phone confirmed-absence cases. Each Retry case awaited its correlated
`session.recover` response before checking the visible result with the default
assertion timeout. The runtime row retained the original execution identity,
and explicit Resume passed through recovery admission while preserving the
uncertain submission and pending queue item.
