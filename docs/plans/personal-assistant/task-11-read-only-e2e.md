---
id: "11-read-only-e2e"
title: "Read-only experiments and rollout evidence"
status: pending
wave: 9
depends_on: ["08-assistant-ui","09-workflow-improvements","10-workspace-grants"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-001
  - REQ-ORCHESTRATION-ASSISTANT-002
  - REQ-ORCHESTRATION-ASSISTANT-003
  - REQ-ORCHESTRATION-ASSISTANT-004
  - REQ-ORCHESTRATION-ASSISTANT-005
  - REQ-ORCHESTRATION-ASSISTANT-006
  - REQ-ORCHESTRATION-ASSISTANT-007
  - REQ-ORCHESTRATION-ASSISTANT-008
  - REQ-ORCHESTRATION-ASSISTANT-009
  - REQ-ORCHESTRATION-ASSISTANT-010
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-001.1
  - AC-ORCHESTRATION-ASSISTANT-001.2
  - AC-ORCHESTRATION-ASSISTANT-001.3
  - AC-ORCHESTRATION-ASSISTANT-001.4
  - AC-ORCHESTRATION-ASSISTANT-002.1
  - AC-ORCHESTRATION-ASSISTANT-002.2
  - AC-ORCHESTRATION-ASSISTANT-002.3
  - AC-ORCHESTRATION-ASSISTANT-002.4
  - AC-ORCHESTRATION-ASSISTANT-003.1
  - AC-ORCHESTRATION-ASSISTANT-003.2
  - AC-ORCHESTRATION-ASSISTANT-003.3
  - AC-ORCHESTRATION-ASSISTANT-003.4
  - AC-ORCHESTRATION-ASSISTANT-004.1
  - AC-ORCHESTRATION-ASSISTANT-004.2
  - AC-ORCHESTRATION-ASSISTANT-004.3
  - AC-ORCHESTRATION-ASSISTANT-004.4
  - AC-ORCHESTRATION-ASSISTANT-005.1
  - AC-ORCHESTRATION-ASSISTANT-005.2
  - AC-ORCHESTRATION-ASSISTANT-005.3
  - AC-ORCHESTRATION-ASSISTANT-005.4
  - AC-ORCHESTRATION-ASSISTANT-006.1
  - AC-ORCHESTRATION-ASSISTANT-006.2
  - AC-ORCHESTRATION-ASSISTANT-006.3
  - AC-ORCHESTRATION-ASSISTANT-006.4
  - AC-ORCHESTRATION-ASSISTANT-007.1
  - AC-ORCHESTRATION-ASSISTANT-007.2
  - AC-ORCHESTRATION-ASSISTANT-007.3
  - AC-ORCHESTRATION-ASSISTANT-007.4
  - AC-ORCHESTRATION-ASSISTANT-008.1
  - AC-ORCHESTRATION-ASSISTANT-008.2
  - AC-ORCHESTRATION-ASSISTANT-008.3
  - AC-ORCHESTRATION-ASSISTANT-009.1
  - AC-ORCHESTRATION-ASSISTANT-009.2
  - AC-ORCHESTRATION-ASSISTANT-009.3
  - AC-ORCHESTRATION-ASSISTANT-009.4
  - AC-ORCHESTRATION-ASSISTANT-010.1
  - AC-ORCHESTRATION-ASSISTANT-010.2
  - AC-ORCHESTRATION-ASSISTANT-010.3
  - AC-ORCHESTRATION-ASSISTANT-010.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 11: Read-only experiments and rollout evidence

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S01–S21, and [plan](plan.md), E2E tests and experiments. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. Existing and new orchestration suites pass together and repeated in one worker with test-owned cleanup; desktop/mobile share the same conversation and action state.
2. Read-only fixtures cover lookup, two-agent context, input, permission, revocation, restart/duplicate delivery and improvement candidates with zero unauthorized mutations.
3. Evidence clearly separates deterministic mock/API coverage from an isolated live-provider trial, records unsupported paths honestly, and leaves production disabled/unchanged.

## Likely files

- apps/backend/internal/backendapp/e2e_reset.go, e2e_reset_test.go
- apps/backend/cmd/mock-agent (minimal deterministic typed-tool scenarios only if needed)
- apps/web/e2e/tests/orchestration/personal-assistant.spec.ts (new)
- apps/web/e2e/tests/orchestration/personal-assistant-attention.spec.ts (new)
- apps/web/e2e/tests/orchestration/personal-assistant-capabilities.spec.ts (new)
- apps/web/e2e/tests/orchestration/mobile-personal-assistant.spec.ts (new)
- apps/web/e2e/tests/orchestration/workspace-orchestrators.spec.ts, automation-orchestrator.spec.ts
- docs/public/orchestration-personas.md; docs/plans/personal-assistant/experiments.md

## Implementation sequence

Use the e2e skill/managed runner. Repair reset by explicit fixture ownership, including persona registration/memory/attention/grants and runs; do not globally rewrite execution profiles or loosen assertions. Inject dropped acknowledgements and typed pending requests, not arbitrary sleeps. Snapshot fixture task/repository/config baselines and mutation counters. For the real-provider trial use only the proven constrained profile with synthetic data; never use the user's actual vault or accounts as a safety test.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -tags fts5 -count=1 ./internal/backendapp -run 'Test.*E2E.*Reset|TestAssistantReadOnly')
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium -- e2e/tests/orchestration --retries=0 --repeat-each=2
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome -- e2e/tests/orchestration/mobile-personal-assistant.spec.ts --retries=0
```

## Dependencies and risks

Dependencies: `08-assistant-ui`, `09-workflow-improvements`, `10-workspace-grants`. Execute in the primary session unless the user explicitly authorizes subagents.

The v0.94.0 rebase repaired the old combined coordinator/Automation fixture-isolation failure; both specs now pass together. New assistant fixtures must preserve that cleanup boundary. Passing a standalone test or a mock reply is insufficient. Pause live-provider trials if native tools cannot be restricted; do not compensate with instructions alone.

## Output

Reproducible deterministic and appropriately scoped live-trial evidence, public usage notes and complete teardown records.

## Detailed execution checklist

1. Freeze a clean candidate that includes the assistant rollout gate and completed
   tasks 03–10. Confirm work-order results name executed tests and no inspect path
   is mislabeled supported. Map every assistant AC to a unit/integration/browser
   result; existing completed 01/02 remain regression scope.
2. Extend fixture-owned cleanup for bindings, retained owners, memory/context,
   attention/outbox, grants, objectives/operations and Automation runs. Preserve
   shared service fixtures and other test-owned records. Run all orchestration
   specs together and repeat, with retries disabled.
3. Drive typed scenarios through the mock provider: direct answer without card,
   task creation and acceptance evidence, queued newer intent, lost acknowledgement,
   native question/permission, attention reconciliation after dropped events,
   locked capability, memory forget and linked-workspace revocation. Assert task/
   repository/config mutation counters for inspect and denial cases.
4. Exercise desktop/mobile assistant navigation and the existing central workspace
   view together. Include page two, stale workspace/binding responses, reconnect,
   pending older session, feature-off direct route, review-versus-done and native
   resolution convergence. Use event predicates/fake clocks, not arbitrary sleeps.
5. Capture screenshots and short silent video in a clean fictional workspace with
   generic requests and scripted responses. Record source SHA, fixture seed, asset
   hashes and review every frame for private prompts, credentials, identifiers,
   paths and unrelated windows. Do not reuse live pilot data.
6. If a real provider/executor has proven enforcement, run a separately isolated
   synthetic read-only trial with restricted credentials and mutation counters.
   No real vault/account secrets or production resources are test inputs. If
   support is absent, record unsupported and stop before launch; deterministic
   tests still run, but no live-provider pass is claimed.
7. Record commands/counts, expected failures/denials, teardown inventory, platform/
   provider support matrix and remaining limits. Update public docs only for
   delivered behavior. Qualification/rehearsal/live enablement follow delivery
   02–04 afterward, rather than being inferred from this experiment.

## Criterion coverage and artifacts

| Area | Deterministic evidence | Browser/experiment evidence |
| --- | --- | --- |
| 001 Ownership/intent | Intake/owner/objective regression suites | Retry/lost acknowledgement, owner switch, no card for a direct answer |
| 002 Context | Pagination/scoping/validation and queue guard tests | Edit/forget, unavailable descriptor, future handoff refusal |
| 003–004 Capabilities/authority | Directory/effect/revocation matrix | Capability health, rejected write, supported-path inspect counters |
| 005–006 Attention/resolution | Reconciliation/outbox/CAS/cancellation tests | Two pending sessions, native answer, human-only permission, pause versus stop |
| 007 UI | API/hook/revision/localization checks | Desktop and mobile shell, reconnect and navigation |
| 008 Improvements | Threshold/grant/boundary tests | Proposal-only without grant, prepared repair still requires publication instruction |
| 009 Workspace scope | Broker/grant/race tests | Explicit target link and revocation; old coordinator token still denied |
| 010 Rollout | Four-combination flag/admission tests | Assistant-off deep link and coordinator-only operation |

New browser specs are the four files listed above; preserve coordinator-view and
Automation coverage in the same managed invocation. Store safe receipts in
`docs/review/orchestration/`; raw local logs and unreviewed media stay outside Git.

## Scope boundaries and delivery

This is evidence collection, not a live deployment or automatic public PR. A
mock tool reply proves deterministic orchestration/UI behavior only. Missing
provider enforcement or migration evidence remains a release limitation.

## Parallelism

`sequential`

## Results

Pending. Record red/green test evidence, exact commands and counts, relevant artifacts, owned changes and cleanup here; synchronize the plan checkbox only after acceptance is met.
