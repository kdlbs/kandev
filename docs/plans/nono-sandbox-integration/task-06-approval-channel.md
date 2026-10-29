---
id: "06-approval-channel"
title: "Connect verified approvals to operator decisions"
status: done
wave: 6
depends_on:
  - "05-nono-provider-lifecycle"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-003
  - REQ-AGENTS-SANDBOX-006
acceptance_criteria:
  - AC-AGENTS-SANDBOX-003.1
  - AC-AGENTS-SANDBOX-003.2
  - AC-AGENTS-SANDBOX-003.3
  - AC-AGENTS-SANDBOX-003.4
  - AC-AGENTS-SANDBOX-003.5
  - AC-AGENTS-SANDBOX-003.6
  - AC-AGENTS-SANDBOX-006.1
  - AC-AGENTS-SANDBOX-006.2
  - AC-AGENTS-SANDBOX-006.3
  - AC-AGENTS-SANDBOX-006.4
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
---

# Task 06: Connect verified approvals to operator decisions

## Summary

Add the core-owned callback channel, durable requests, operator decision API, and permission cards.

## In scope

Protect callback capabilities, validate request identity, enforce ceilings, deny on deadlines, and provide desktop/phone decisions.

## Out of scope

No inspect polling for requests, terminal scraping, agent auto-approval, signed-platform assumption, or implicit policy persistence.

## Acceptance

- Real Nono file requests reach one bound operator card and grant only the exact approved operation.
- Forged, stale, duplicated, late, oversized, disconnected, and cancelled requests cannot grant access.
- The sandbox cannot read the callback secret or contact its listener, and phone decisions preserve desktop semantics.

## UI preview

UI-02: Approval. See the [combined preview](plan.md#ui-preview).
Desktop: operation and policy details above Deny and Allow once. Phone: focused full-height review with fixed actions and one scroll owner.
Preserve loading, error, expired, and read-only states. Use shared domain state and localized labels.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/sandbox/... ./internal/agentctl/server/process -run 'SandboxApproval' -count=1)
pnpm --dir apps/web e2e:run --host --project chromium tests/task/sandbox-approvals.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sandbox-approvals.spec.ts -- --retries=0
python3 scripts/check-nono-real.py --approvals --record docs/plans/nono-sandbox-integration/acceptance.md
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `apps/backend/internal/agent/sandbox/approval.go (new)`
- `apps/backend/internal/agent/sandbox/approval_test.go (new)`
- `apps/backend/internal/agentctl/server/process/`
- `apps/web/components/task/chat/`
- `apps/web/hooks/domains/`
- `apps/web/e2e/tests/task/sandbox-approvals.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-sandbox-approvals.spec.ts (new)`
- `apps/web/src/locales/`

## Dependencies

05-nono-provider-lifecycle

## Risks

Unsigned webhook requests require a protected bearer capability. Failure to prove that boundary blocks approval support.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).

## Results

Implementation is authorized. Component checks are recorded below; full product acceptance is pending.

Approval persistence and private HTTP transport are implemented as components. SQLite and PostgreSQL tests cover operator authority, immutable requests, single delivery, and refusal after callback loss or process stop. HTTP tests cover duplicates, replay, bounds, cancellation, and failed response flush. The operator API exposes bounded approval views without private runtime identities or callback data.

A packaged Nono integration check now exercises a real file request, an operator allow-once response, a second denied read, and unchanged policy content. Extending that test to a permitted loopback network destination exposed Nono's port-stripping behavior. Exact egress enforcement now requires the core relay described in the updated provider design. Product wiring, permission cards, and complete approval acceptance remain open.


The shared approval review is now connected to committed, workspace-scoped live events. It shows the session, captured policy revision and digest, exact operation and target, deadline, and delivery state. Allow once and Deny require an explicit operator action. Decisions preserve saved policies. Reads and mutations are scoped to the authenticated user, session incarnation, attempt, and connection epoch. Duplicate clicks share one request. Late responses after sign-out or operator replacement cannot enable an old action.

A deferred-read regression found that ordinary query invalidation could retain an older empty response. Live events now cancel that read and obtain the current list. Desktop browser testing found that the final request removed its own focus target; closing now returns focus to Execution. Twenty-four focused frontend tests, typecheck, localization, scoped lint, and public-doc validation pass. Desktop and phone browser checks pass for live arrival, explicit allow/deny, focus return, read-only access, expiry, disconnection, long content, 44px touch targets, and phone widths 393, 700, and 767. Both final screenshots were inspected. Browser fixtures prove interface behavior only. Full actual-agent acceptance, denial learning, explicit restart review, and child-state deletion remain open.


## Completed acceptance (2026-10-27)

A real managed ACP Pi session requested a file outside its saved grants while agent auto-approval was enabled. The callback still required the authenticated human operator. Denial produced a report. A later explicit allow-once decision permitted each requested low-level operation without changing policy. Pi performs a MIME probe and the actual read separately, so both requests received separate operator decisions. The packaged-provider regression passed real request/decision handling, callback isolation, MCP, and denial controls. Runtime conformance passed deny, allow, invalid response, and timeout. Existing authority/forgery/replay tests and desktop/phone approval checks remain the evidence for those boundaries.
