---
id: "03-delivery-failure-recovery"
title: "Observe writer failure and preserve recovery evidence"
status: complete
wave: 3
depends_on:
  - 02-all-tool-preparation
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-001
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-003
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.1
  - AC-PLATFORM-BOUNDED-TOOL-DATA-003.1
  - AC-PLATFORM-BOUNDED-TOOL-DATA-003.2
  - AC-PLATFORM-BOUNDED-TOOL-DATA-003.3
  - AC-PLATFORM-BOUNDED-TOOL-DATA-004.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.25
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.26
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
system_design:
  - ../../specs/platform/system-design/bounded-tool-delivery.md
  - ../../specs/platform/system-design/durable-delivery-failure.md
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 03: Observe writer failure and preserve recovery evidence

## Summary

Make writer failure observable without output delivery, browser focus, or another instruction.
Preserve the actual native terminal when storage permits and retain evidence-based recovery when it does not.

## In scope

- Capture an immutable, bounded failure snapshot at the failed write boundary.
- Expose it through existing authenticated status JSON, independently of writer/journal progress.
- Distinguish oversized envelopes from disk, corruption, unavailable-store, and writer errors.
- Keep the native-event consumer through bounded cancellation drain and actual terminal persistence attempts.
- Add an owned lifecycle health monitor with two-second probes, a two-second request deadline, and no overlapping requests.
- Reuse exact session recovery fences, native restoration/retirement, and direct/queued admission.
- Preserve recovery across reload/restart, actual outcomes, independent fences, and old-peer limitations.
- Extend existing bounded metric categories and structured diagnostics without raw output.
- Verify current desktop/phone recovery controls using isolated end-to-end fixtures.
- Reuse existing recovery copy, layout, and actions without new markup or localization.
- Update public recovery guidance and package results after implementation.

## Out of scope

New SQL tables, coarse session enums, admission locks, provider retries, fabricated terminals, prompt replay, and forced runtime replacement.
New output notices, preview widgets, UI labels, locale keys, and recovery layouts are excluded.
Historical live sessions remain untouched.

## Acceptance

1. A failed ordinary append leaves status and Stop responsive and drains the actual native cancellation/terminal.
   A committed terminal settles the real outcome. Failed terminal persistence retains uncertainty and accepted records.
2. Without another stream event or user action, a correlated reachable failure produces persisted recovery within ten seconds.
   Healthy silence remains running. Old replies, changed prompts, unavailable evidence, and sibling sessions retain their independent state.
3. Retry/Resume/Stop reuse existing proof-based recovery without dispatching the old instruction or clearing unrelated fences.
   After valid recovery, a distinct new instruction is admitted once and a usable writer is reestablished.
   Desktop and phone retain their existing recovery controls, layout, copy, and draft behavior.

## TDD regression

Create `TestDeliveryFailureDrainsActualTerminal`, `TestDeliveryFailureTerminalWritePreservesUnknown`, and `TestDeliveryHealthStatusBypassesFailedWriter`.
Inject a genuine append failure into the real process path, followed by the adapter's actual terminal.
The current early return must fail the drain regression.

Create fake-time `TestDeliveryHealthFailureWithoutOutput` and `TestDeliveryHealthHealthySilence`.
Cover failure during native async output as well as an outstanding prompt.
Use `TestDeliveryHealthStaleOwnerAndSiblingIsolation` for delayed status, successor prompts, concurrent Stop, and replacement.

Create `TestDeliveryHealthRecoveryDoesNotResend` and `TestDeliveryHealthRecoveryKeepsIndependentFence` through real admission/reconciliation boundaries.
Include a healthy active record beside failed history. One eligible record must not override another unresolved owner.
Interrupt terminal projection, native retirement, and SQL fence resolution, then repeat recovery and restart.

## Existing-interface regression coverage

Use `SessionRecoveryCard` and the current interrupted-delivery UI as shipped.
No ASCII UI preview is required because this package changes no rendered layout or control.
Use existing desktop/mobile durable-stream recovery tests as the interaction patterns.

Add `e2e/tests/session/resilient-output-delivery.spec.ts` and its `mobile-` counterpart.
Exercise real backend preparation, journaling, status observation, and recovery through isolated mock-agent fixtures.
Do not seed only the expected UI state.

1. Incident-size search reaches its real terminal and admits a distinct next instruction once.
2. Larger results retain existing truncation representation and correct status without a new widget.
3. Failed delivery activates the existing recovery card without another event or user action.
4. Retry remains state-only, Stop remains reachable, reload retains the draft, and valid recovery never resends the old instruction.
5. An unrelated session remains usable while terminal persistence or owner verification fails.

The lifecycle fake-time test proves the ten-second bound.
Phone tests preserve existing action reachability and containment. No new interaction model or control-sizing sweep is needed.
Any failure injection stays behind existing E2E-only guards and needs a production-denial regression.

## Verification

Run from the repository root:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race -p 1 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite -run 'TestDelivery(Failure|Health)|TestLatePressureEvent|TestDeliveryPressure|Test.*NativeResume|Test.*UnresolvedDelivery' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Retain the affected existing native-recovery and cancellation regressions in the selected run.
Run any changed dialect-sensitive recovery tests with the existing configured PostgreSQL job.
Report unavailable PostgreSQL evidence instead of accepting skipped coverage.
Run changed-file Go lint on this work order's complete changed-file inventory.

Run these existing-interface and isolated E2E checks sequentially from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/session-recovery-card.test.tsx components/task/chat/messages/tool-search-message.test.tsx components/task/chat/messages/tool-execute-message.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/session/resilient-output-delivery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-resilient-output-delivery.spec.ts)
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race -p 1 ./internal/agent/runtime/lifecycle ./internal/agent/handlers -run 'Test.*E2E.*Delivery|TestDeliveryHealth' -count=1)
make -C apps/backend build
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The install is a one-time prerequisite when workspace dependencies are missing.
The managed E2E runner builds fresh assets and preserves the single-worker resource guard.
Public docs describe implemented recovery only after the corresponding checks pass.

Run this exact changed-file Go lint block from the repository root:

```bash
python3 - <<'LINT'
import pathlib, subprocess
tracked = subprocess.check_output(['git', 'diff', '--name-only', '--diff-filter=ACMR', 'HEAD', '--', 'apps/backend'], text=True)
untracked = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '--', 'apps/backend'], text=True)
files = sorted({p for p in (tracked + untracked).splitlines() if p.endswith('.go') and pathlib.Path(p).is_file()})
if files:
    subprocess.run(['bash', 'scripts/lint-go-changed', *files], check=True)
LINT
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`, `delivery_pressure.go`, and new `delivery_failure_test.go`.
- `apps/backend/internal/agentctl/server/api/server.go` (`StatusResponse` and `handleStatus`), plus status/control tests.
- `apps/backend/internal/agentctl/journal/metrics.go` and the existing error classifier.
- `apps/backend/internal/agent/runtime/agentctl/client.go` and status decoding tests.
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`, `delivery_reconciliation.go`, and `delivery_reconciliation_pressure.go`.
- Proposed `delivery_health_monitor.go` and `delivery_health_monitor_test.go` beside lifecycle streams.
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`, `native_resume_readiness.go`, and `session_native_delivery_recovery.go`.
- Existing recovery/block projection and SQL admission test owners.
- `apps/web/e2e/tests/session/resilient-output-delivery.spec.ts` and `mobile-resilient-output-delivery.spec.ts` (new).
- Existing isolated E2E fixture helpers and current recovery-card tests.
- `docs/public/sessions-and-review.md` and package results/status records.

Reuse the current `/api/v1/status` endpoint and authentication scope.

## Dependencies

Task 02 establishes the prepared event and typed permanent-error boundary.
Use the unchanged convergence authority. Do not create an incident-specific bypass.

## Risks

- The only event consumer currently exits after the failed append.
- A fresh capacity read cannot establish that a failed writer became usable.
- Late health reports can target a successor unless identity is captured and revalidated.
- A real disk failure can also prevent terminal persistence.
- A live process or cancellation request is insufficient evidence for a terminal outcome.

## Parallelism

`sequential`

## Inputs

- [Delivery failure design](../../specs/platform/system-design/durable-delivery-failure.md).
- [Recovery convergence](../../specs/platform/system-design/session-recovery-convergence.md).
- Existing process pressure/Stop tests, `StreamManager` ownership patterns, and native retirement/admission matrix tests.

## Results

Implemented the immutable delivery-failure snapshot, bounded post-failure cancellation drain with actual-terminal commit, lifecycle health monitor probing the existing status endpoint every two seconds, and reuse of the existing recovery fences and surfaces.

Validation passed:

- `(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race -p 1 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite -run 'TestDelivery(Failure|Health)|TestLatePressureEvent|TestDeliveryPressure|Test.*NativeResume|Test.*UnresolvedDelivery' -count=1)`
- `(cd apps/web && pnpm exec vitest run components/task/chat/session-recovery-card.test.tsx components/task/chat/messages/tool-search-message.test.tsx components/task/chat/messages/tool-execute-message.test.tsx)` passed 33 tests.
- `(cd apps/web && pnpm e2e:run --host --project chromium tests/session/resilient-output-delivery.spec.ts)` passed 4 tests, including a real oversized writer failure that recovered the writer, retained a draft across reload, admitted one distinct next instruction, and an unresolved failure that surfaced the existing recovery card without resending the prompt.
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-resilient-output-delivery.spec.ts)` passed 2 tests with real writer-failure controls, no resend, Stop reachability, and no document overflow.
- `make -C apps/backend build` passed.
- `node --test scripts/validate-public-docs.test.mjs` passed 63 tests; `node scripts/validate-public-docs.mjs` validated 54 pages.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.

The mock-agent `large-search`, `large-search-overflow`, `oversized-output`, and `oversized-output-hold` scenarios exercise the real adapter and forwarder. The failure tests do not seed only the desired UI state: a genuine encoded-event rejection drives health observation, persisted recovery, Retry, Stop, and no-resend checks.

Changed-file Go lint reported 0 issues.

Additional review-remediation regressions passed:

- `TestDeliveryHealthMonitorSurvivesSuccessfulStreamSetup` proves the monitor survives a successful real stream setup, keeps probing, and is cancelled by shutdown.
- `TestDeliveryHealthMonitorPublishesRecoveryAfterRealTerminal` and `TestDeliveryFailedWriterNeverReportsRecovered` use the actual reconciler and prove a live process or open socket cannot clear a sticky writer failure, while a real terminal or cleared failure publishes recovery.
- `TestDeliveryWriterFailureUncertainPhasePersistsRecoveryBlock` proves the actionable uncertain phase persists an `agent_delivery` recovery block without settling the coarse session state.
- `TestDeliveryFailureDrainsActualTerminal` now dispatches a distinct second instruction through the same consumer after the cancellation terminal and verifies both submissions settle exactly once.

`TestDeliveryHealthRecoveryWithinTenSeconds` advances fake probe time and asserts the real reconciliation starts within the shared ten-second window after the failure becomes observable.

Review-fixup additions:

- `TestDeliveryFailureTimeoutSuppressesFailedOutput` proves ordinary output from the failed submission stays dropped after the drain timeout and that only its terminal or a different submission's committed event releases the sticky failure.
- `TestDeliveryHealthStateOnlyFailureUsesReconciler` covers retained peers that report `storage_error` without a failure snapshot and verifies the existing reconciler publishes the uncertain phase.
- Desktop and phone E2E were rerun after the merge; Retry, Stop, draft retention, no-resend, and distinct-next-instruction behavior remain green.
