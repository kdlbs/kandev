---
id: "09-workflow-improvements"
title: "Supervised workflow improvement loop"
status: pending
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

Pending. Record red/green test evidence, exact commands and counts, relevant artifacts, owned changes and cleanup here; synchronize the plan checkbox only after acceptance is met.
