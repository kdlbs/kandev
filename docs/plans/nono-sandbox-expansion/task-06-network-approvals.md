---
id: "06-network-approvals"
title: "Deliver exact connection approvals and policy envelopes"
status: pending
wave: 6
depends_on: 
  - "05-audit-verification"
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
  - AC-AGENTS-SANDBOX-011.7
  - AC-AGENTS-SANDBOX-012.2
  - AC-AGENTS-SANDBOX-012.4
system_design:
  - ../../specs/agents/system-design/sandbox-operation-approvals.md
---

# Task 06: Deliver exact connection approvals and policy envelopes

## Summary

Introduce policy schema 2 and deliver exact TCP approval through the attempt-owned runtime-service channel.

## In scope

- Typed policy core and version-preserving migration, explicit approval envelopes, actor validation, network preflight, editor/review UI.
- Nono native approval-proxy mode with strict empty allowlist, static-policy evaluation, and one-connection operator decisions.

## Out of scope

- No automatic replacement of the v1 upstream-relay path, no arbitrary loopback allowance, no command/HTTP approval activation.

## Acceptance

- A static exact destination works; an envelope-only destination needs one decision per connection; forbidden same-host ports remain blocked.
- PID-zero service requests bind to the owned channel without weakening child checks; forged, expired, concurrent, or foreign requests fail correctly.
- Real runtime and desktop/phone tests prove decisions, delivery states, MCP isolation, and unchanged saved revisions.

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
python3 scripts/check-nono-real.py --conformance --record /tmp/nono-network-conformance.json
make build-backend build-web
(cd apps/web && pnpm e2e:run e2e/tests/task/sandbox-approvals.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run e2e/tests/task/mobile-sandbox-approvals.spec.ts --project=mobile-chrome)
```

Also run `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)`. Add targeted Vitest coverage for changed business logic. Use the existing Traditional Chinese generation command. Install workspace dependencies from `apps/` if this worktree lacks them.

## Files likely touched

- `apps/backend/internal/common/sandbox/policy.go`
- `apps/backend/internal/agent/sandbox/approval_validation.go`
- `apps/backend/internal/agent/sandbox/approval_channel.go`
- `apps/backend/internal/agent/sandboxruntime/prepare.go`
- `apps/web/components/settings/sandbox-policy-fields.tsx`
- `apps/web/components/task/sandbox-approval-review.tsx`
- `../kandev-plugin-nono-main/server/network.go`
- `../kandev-plugin-nono-main/server/approval.go`
- `scripts/check-nono-real.py`

## Dependencies

05-audit-verification.

## Risks

Plain allow_domain strips ports and upstream CONNECT bypasses the new approval backend. The native path needs real same-host/different-port and callback-isolation proof.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Pending implementation. Preliminary pinned-runtime probes on 2026-11-03 confirmed native CONNECT callbacks contain the network operation, exact port, PID zero, request ID, and runtime session ID. Two sequential allowed connections generated distinct callbacks. A different port and the callback endpoint were denied.

The default kernel TCP-port restriction allowed a direct connection to an owned server at `127.0.0.2` on the proxy port, without a callback. Repeating the probe with `--isolate net` blocked that bypass while preserving approved CONNECT traffic. Native v2 mode must require network namespace isolation and refuse startup when it is unavailable. No all-port loopback option or external-enforcement fallback is permitted. The v1 relay path is unchanged. The full DNS/alias/IPv6, concurrency, MCP, operator, and packaged acceptance matrix is still pending.

Temporary owned evidence: `/tmp/nono-task06-network-probe.py`, `/tmp/nono-task06-network-probe-result.json`, and `/tmp/nono-task06-network-netns-result.json`. These probes do not prove host integration or authorize capability activation.
