---
id: "09-workflow-improvements"
title: "Supervised workflow improvement loop"
status: done
wave: 7
depends_on: ["05-tool-authority","06-attention","07-input-resolution","08-assistant-ui"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-008
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-008.1
  - AC-ORCHESTRATION-ASSISTANT-008.2
  - AC-ORCHESTRATION-ASSISTANT-008.3
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 09: Supervised workflow improvement loop

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S16, S17, and [plan](plan.md), Backend 7. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. Redacted incidents distinguish actual gate origins and generate one candidate only after three matching incidents across two tasks in seven days.
2. Without a maintenance grant the assistant only reports/investigates; with it, one isolated normal task can produce a narrow fix, positive/negative tests and local commit receipt.
3. The maintenance path cannot self-approve, broaden permission policy, push/PR/deploy/restart, or count suppressed prompts as a successful fix.

## Likely files

- apps/backend/internal/orchestration/models/friction.go (new)
- apps/backend/internal/orchestration/repository/sqlite/friction.go (new)
- apps/backend/internal/orchestration/runtime/friction.go, friction_test.go (new)
- apps/backend/internal/backendapp/adapters_assistant_attention.go (safe origin adapter)
- apps/web/app/assistant/attention-card.tsx (candidate presentation)
- docs/public/orchestration-personas.md (maintenance scope and evidence)

## Implementation sequence

Create synthetic provider/local/configuration/authentication incidents and test normalization, redaction and deduplication. Reuse objective/routing/operation mechanisms for repair tasks; record candidate -> investigation -> proposed -> verified/local-commit -> observed resolution. Require positive and negative permission cases so a blanket bypass fails acceptance. External classifier internals remain unknown unless authoritative evidence exists.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp -run 'TestAssistant(Friction|Improvement|Maintenance)')
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test app/assistant/attention-card.test.tsx)
```

## Dependencies and risks

Dependencies: `05-tool-authority`, `06-attention`, `07-input-resolution`, `08-assistant-ui`. Execute in the primary session unless the user explicitly authorizes subagents.

Repeated denials can be correct. Do not learn an allow rule from frequency or from untrusted error text. Candidate evidence must not retain credentials or unrelated private transcripts.

## Output

A bounded, auditable repair proposal/commit loop, not a permission bypass.

## Detailed implementation checklist

1. Add bounded redacted friction observations with workspace/account scope,
   operation family, normalized reason, origin/version and safe source IDs.
   Classify missing capability, expired authentication, denied authority, policy
   boundary, transient provider failure and ordinary task defect separately.
2. Aggregate by stable fingerprint over a rolling seven-day window. Require at
   least three occurrences across two distinct tasks; one open candidate per
   fingerprint. Deduplicate repeated delivery and use an injectable clock.
3. Provide read-only investigation and an owner-visible proposal with evidence,
   intended change boundaries and expected validation. Without a current explicit
   workflow-maintenance grant, stop at the proposal and create no repair task.
4. A qualifying grant may create an isolated normal task to prepare a patch/tests
   and local commit. Bind the task/operation to owner, scope, candidate/grant
   revision and allowed files/actions. Recheck changed-file and effect boundaries
   before applying/committing; workspace tools remain subject to native authority.
5. Reject changes to broad allowlists, provider/tool enforcement or self-approval.
   A grant never authorizes push, PR, deployment or restart. Revocation stops future
   dispatch; already-performed external effects retain an honest receipt.
6. Track proposed/investigating/prepared/rejected/resolved/unknown states. Resolve
   a candidate using subsequent successful affected workflows and linked evidence,
   not simply fewer permission requests or a model declaration of success.
7. Add persistence, retention and SQLite/PostgreSQL conformance for new rows;
   owner-scoped UI summary and docs reuse existing assistant panels/task links.

## Detailed evidence map

| Criterion | Planned test | Required edge cases |
| --- | --- | --- |
| AC-ORCHESTRATION-ASSISTANT-008.1 | `TestAssistantFrictionThresholdAndRedaction` | Below/exact threshold, two tasks, window expiry, duplicate event, canary text |
| AC-ORCHESTRATION-ASSISTANT-008.2 | `TestAssistantMaintenanceGrantDispatch` | No grant, stale/revoked grant, allowed isolated task, stable receipt |
| AC-ORCHESTRATION-ASSISTANT-008.3 | `TestAssistantMaintenanceBoundary` | Forbidden file/effect, self-approval, push/PR/deploy/restart denial, no false resolution |

Tests assert actual task/mutation counters, not just generated suggestions. Use
fake task creation/effect ports and deterministic clocks; exercise a single
synthetic browser proposal/review flow in task 11.

## Scope boundaries and delivery

This is a supervised repair workflow, not permission-policy optimization. Existing
user-facing permission behavior is not relaxed. If safe changed-file enforcement
cannot be established, offer the proposal only and document unsupported repair.

## Parallelism

`sequential`

## Results

Complete. The foundation includes typed native friction and original event
times, deduplicated threshold aggregation, human-only revisioned grants,
maintenance objectives and one native review task, private file CAS, qualified
offline container checks, and validated local commit receipts. General task
dispatch and ordinary objective delegation cannot bypass the maintenance path.
The executor decision is recorded in
[ADR-2026-09-18](../../decisions/2026-09-18-closed-maintenance-preparation.md).

Behavioral red/green checks cover both SQL engines, actual isolated Git/Docker
operations, concurrent file CAS, revoked grants before effects, foreign owners,
unknown gate provenance, source-time windowing and old-workspace visibility.
Detailed final checkpoint verification is recorded below after checks complete.

The proposal/grant/review UI, named broker tools, file/artifact inspection, human
closure with subsequent native success, recurrence after closure and retention/
recovery integration are complete. Desktop and phone browser flows exercise
synthetic proposal evidence, the explicit grant form and persisted human rejection
without creating a repair task. Cross-feature combined coverage remains task 11.

### Foundation checkpoint verification, 2026-09-18

From `apps/backend`, with the repository toolchain, an owned PostgreSQL fixture
and `KANDEV_TEST_MAINTENANCE_IMAGE` set to an installed immutable Linux image:

```sh
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestration/... ./internal/persistence/storeconformance
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/persistence/storeconformance -run 'TestAssistant(Friction|Maintenance)'
GIN_MODE=release go test -tags fts5 -count=1 ./internal/persistence/storeconformance -run 'TestPreviousStableUpgrade|TestUpgradeFixtureManifest'
GIN_MODE=release go test -tags fts5 -count=1 ./internal/backendapp ./internal/task/service -run 'Test(Assistant|WorkspaceTask|Contribution|CreateTask.*Contribution)'
go run ./cmd/sqlguard ./internal
golangci-lint run --new-from-rev=upstream/release-v0.94.0
```

All passed. The full store suite passed in 117.2 seconds; the final open-candidate
index received a subsequent both-engine friction/grant race check (5.9 seconds)
and previous-stable upgrade check (19.1 seconds). The real sandbox package passed
under the race detector in 3.8 seconds, including container isolation, stale tree
rejection and unchanged source checkout. Runtime race checks passed in 21.5
seconds. SQL guard, full specification lint and diff whitespace checks passed.
No live service or real prompt history was used. Test repair containers are
removed; the owned database fixture remains available for the next SQL slice.

### Review completion verification, 2026-09-18

Added human closure and review receipts, independently scoped recurrence evidence,
bounded artifact/file reads, complete patch download, native resource/result
pickers, five closed broker tools, durable unknown recovery and 30-day retention.
Patching invalidates checks. Old account events cannot be regrouped, and native
success under a changed account configuration cannot resolve an older candidate.
The localized Details view includes the exact grant, revocation, validation,
artifact and verified recovery controls.

Behavioral red/green checks demonstrated old/new incident mixing on both engines,
old profile-event regrouping, stale checks after patch, misuse of human review
as general delegation authority and resolution under a changed account. All now
pass. Artifact tests exercise actual private Git/Docker operations and apply the
returned patch with `git apply --check`; bounded subprocess output cannot silently
truncate evidence. Native recovery tests use persisted messages, sessions and gates.

Verification with the repository toolchain and owned PostgreSQL/image fixtures:

```sh
# apps/backend
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestration/... ./internal/persistence/storeconformance
GIN_MODE=release go test -tags fts5 -count=1 ./internal/backendapp ./cmd/agentctl -run 'TestAssistant|TestKandevAssistant'
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestration/runtime -run 'TestAssistant(Improvement|Maintenance|Friction|FeatureGate)'
go run ./cmd/sqlguard ./internal
# apps/web
pnpm test app/assistant/maintenance-grant-form.test.tsx app/assistant/maintenance-review.test.tsx hooks/domains/orchestration/use-maintenance-mutations.test.ts
pnpm run typecheck
pnpm e2e:run --host --shards 1 --project chromium tests/orchestration/assistant-maintenance.spec.ts -- --retries=0
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/orchestration/mobile-assistant-maintenance.spec.ts -- --retries=0
```

Full Orchestration race and store conformance passed, including both-engine fresh,
replay and previous-stable upgrades (store suite 134.6 seconds; runtime 19.1 seconds;
actual maintenance sandbox 3.1 seconds). Native adapter/broker checks passed.
The final account-resolution guard received its focused runtime race check.
All five new UI tests passed; typecheck, scoped lint, SQL guard, specification,
architecture and harness validation passed. Desktop and phone each passed one
test with retries disabled (6.9 and 4.4 seconds). The initial desktop run exposed
a fixture selector that skipped the Details tab; the test now waits for its
accessible role on both layouts. No product response was mocked and no real
prompt or live service was used. Real-provider qualification remains task 11.
