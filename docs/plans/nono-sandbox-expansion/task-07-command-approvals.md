---
id: "07-command-approvals"
title: "Deliver confined command invocation approvals"
status: pending
wave: 7
depends_on: 
  - "06-network-approvals"
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
system_design:
  - ../../specs/agents/system-design/sandbox-operation-approvals.md
---

# Task 07: Deliver confined command invocation approvals

## Summary

Add immutable command identity, exact invocation/caller rules, and resource policies through Nono command mediation.

## In scope

- Command canonicalization/subset checks, explicit caller edges, resource policies, compiler and request decoding, typed review/settings.
- Child/shim identity checks and cancellation of brokered descendants.

## Out of scope

- No arbitrary regex or shell-text policy, no writable executable bypass, no claim that all session executables are mediated.

## Acceptance

- The exact invocation can be approved once; wrong argv, caller, executable replacement, direct-path bypass, or extra child resources are denied.
- Repeated and concurrent invocations have separate request identities; cancellation removes owned tool descendants.
- Desktop and phone show exact operation and resources; redaction that prevents safe review blocks approval.

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
python3 scripts/check-nono-real.py --conformance --record /tmp/nono-command-conformance.json
make build-backend build-web
(cd apps/web && pnpm e2e:run e2e/tests/task/sandbox-approvals.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run e2e/tests/task/mobile-sandbox-approvals.spec.ts --project=mobile-chrome)
```

Also run `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)`. Add targeted Vitest coverage for changed business logic. Use the existing Traditional Chinese generation command. Install workspace dependencies from `apps/` if this worktree lacks them.

## Files likely touched

- `apps/backend/internal/common/sandbox/command_policy.go (new)`
- `apps/backend/internal/agent/sandbox/approval_validation.go`
- `../kandev-plugin-nono-main/server/command_policy.go (new)`
- `../kandev-plugin-nono-main/server/approval.go`
- `apps/web/components/settings/sandbox-policy-fields.tsx`
- `apps/web/components/task/sandbox-approval-review.tsx`
- `scripts/check-nono-real.py`

## Dependencies

06-network-approvals.

## Risks

Brokered tools have separate resource policies and requester identities. A parent-session grant must not implicitly authorize the tool.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Pending.
