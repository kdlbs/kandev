---
id: "08-endpoint-approvals"
title: "Deliver exact HTTP endpoint approvals"
status: pending
wave: 8
depends_on: 
  - "07-command-approvals"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-011
  - REQ-AGENTS-SANDBOX-012
acceptance_criteria:
  - AC-AGENTS-SANDBOX-011.1
  - AC-AGENTS-SANDBOX-011.2
  - AC-AGENTS-SANDBOX-011.3
  - AC-AGENTS-SANDBOX-011.4
  - AC-AGENTS-SANDBOX-011.5
  - AC-AGENTS-SANDBOX-011.6
  - AC-AGENTS-SANDBOX-012.2
  - AC-AGENTS-SANDBOX-012.4
system_design:
  - ../../specs/agents/system-design/sandbox-operation-approvals.md
---

# Task 08: Deliver exact HTTP endpoint approvals

## Summary

Add explicit HTTP route permissions and per-request review with private attempt trust material and no raw-TCP bypass.

## In scope

- Origin/method/path/query canonicalization, route bindings, Nono route compilation, typed service requests, editor/review UI.
- Per-attempt TLS trust artifacts, blocked redirects and protocol upgrades, exact request replay tests.

## Out of scope

- No global CA installation, disabled TLS verification, body-level authorization, wildcard endpoint rules, or unrestricted fallback.

## Acceptance

- An approved exact request succeeds once; changed method/path/query/origin, encoding ambiguity, redirects, and raw TCP escape fail.
- Shared proxy labels and PID zero cannot authorize another attempt or route; callback and CA-key isolation pass real tests.
- Desktop and phone disclose HTTP scope and body limitation, and preserve explicit deny/expiry behavior.

## UI preview

Native structured preview per the task instruction. [Full composition and states](plan.md#ui-previews).

### UI-03: Typed policy and approval

Entry: executor policy editor or selected session approval. AC-AGENTS-SANDBOX-011.1 through .7.

| Order | Desktop dialog | Phone full-height next view |
| --- | --- | --- |
| Policy editor | Static access / Ask for approval groups | Same groups in one column; typed editor as next view |
| Review header | Operation type / attempt / deadline | Back / operation type, followed by attempt and deadline |
| Scope | TCP host:port, or command/caller/resources, or HTTP origin/method/path | Exact fields wrap; long values expand in place |
| Explanation | One connection, invocation, or request | Same scope warning above fixed actions |
| Fixed actions | Deny / Allow once | Deny / Allow once, each at least 44px |

Expired requests replace actions with their final outcome. Disconnection disables decisions. Read-only users see the same permitted facts without mutation controls. A stale policy draft retains edits and offers explicit reload. Responsive changes preserve the pending request and draft. Network review states that a connection can carry multiple application requests.

## Verification

Run from the host repository root. New test files/scripts named below are implementation deliverables. A command that runs zero relevant tests or skips real-runtime coverage does not satisfy this order.

```bash
set -e
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test database DSN privately}"
: "${NONO_ACCEPTANCE_BINARY:?Set the verified candidate binary path before runtime tests}"
(cd apps/backend && go test -race ./internal/common/sandbox/... ./internal/agent/sandbox/... ./internal/agent/sandboxruntime/...)
: "${NONO_ACCEPTANCE_BINARY:?Set the verified candidate binary path}"
(cd ../kandev-plugin-nono-main && make validate)
python3 scripts/check-nono-real.py --conformance --record /tmp/nono-endpoint-conformance.json
make build-backend build-web
(cd apps/web && pnpm e2e:run e2e/tests/task/sandbox-approvals.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run e2e/tests/task/mobile-sandbox-approvals.spec.ts --project=mobile-chrome)
```

Also run `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)`. Add targeted Vitest coverage for changed business logic. Use the existing Traditional Chinese generation command. Install workspace dependencies from `apps/` if this worktree lacks them.

## Files likely touched

- `apps/backend/internal/common/sandbox/endpoint_policy.go (new)`
- `apps/backend/internal/agent/sandboxruntime/materialize.go`
- `../kandev-plugin-nono-main/server/endpoint_policy.go (new)`
- `../kandev-plugin-nono-main/server/approval.go`
- `apps/web/components/task/sandbox-approval-review.tsx`
- `scripts/check-nono-real.py`

## Dependencies

07-command-approvals.

## Risks

Certificate pinning and library-specific trust may block interception. Such incompatibility must stop the required route instead of bypassing it.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Pending.
