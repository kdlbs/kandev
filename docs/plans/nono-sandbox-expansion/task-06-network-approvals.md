---
id: "06-network-approvals"
title: "Deliver exact connection approvals and policy envelopes"
status: in_progress
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

Implementation in progress. Preliminary pinned-runtime probes on 2026-11-03 confirmed native CONNECT callbacks contain the network operation, exact port, PID zero, request ID, and runtime session ID. Two sequential allowed connections generated distinct callbacks. A different port and the callback endpoint were denied.

The default kernel TCP-port restriction allowed a direct connection to an owned server at `127.0.0.2` on the proxy port, without a callback. Repeating the probe with `--isolate net` blocked that bypass while preserving approved CONNECT traffic. Native v2 mode must require network namespace isolation and refuse startup when it is unavailable. No all-port loopback option or external-enforcement fallback is permitted. The v1 relay path is unchanged. The full DNS/alias/IPv6, concurrency, MCP, operator, and packaged acceptance matrix is still pending.

Temporary owned evidence: `/tmp/nono-task06-network-probe.py`, `/tmp/nono-task06-network-probe-result.json`, and `/tmp/nono-task06-network-netns-result.json`. These probes do not prove host integration or authorize capability activation.

The same owned alternate-address probe also reached the unapproved destination through the legacy relay mode when isolation was absent. Its positive controls established two allowed CONNECT tunnels through the relay; network isolation blocked the direct bypass while retaining those tunnels. Require namespace isolation for new Nono launches with any proxy access, including the legacy relay and logical MCP. This hardens the existing path; it does not replace it with native approvals or migrate saved policy revisions. Filesystem-only blocked-network policies do not need a proxy namespace. Add permanent compiled-policy and real-runtime regressions before changing the compiler.

Host rebase checkpoint `c87e576f6018` and public plugin SDK pin `f35f5aa87` are pushed to their existing draft PRs. The independent SDK consumer, plugin validation/package verification, and rebuilt packaged host/runtime tests pass. No merge or release is authorized.

### Network isolation slice (2026-11-03)

The provider compiler now requests `linux.isolate.network` whenever a new launch has a TCP or logical MCP grant. It leaves unrestricted namespace loopback disabled and retains the existing upstream relay. Blocked-network filesystem policies do not request the proxy namespace. Saved policy revisions and captured attempts are unchanged.

RED: `TestCompileRequiresNetworkNamespaceForProxyAccess` failed for TCP and MCP. `TestPinnedRuntimeEnforcesProxyDestinations` failed because an owned `127.0.0.2` listener on the proxy port received one unapproved child request. The fixture verifies host reachability before releasing the child, exchanges only the public proxy port, and checks zero forbidden requests after the process exits.

GREEN: both regressions pass after the compiler change, including permitted proxy work and ordinary filesystem/network denials. `TestPinnedRuntimeRefusesProxyWhenNamespacesAreUnavailable` installs a seccomp filter only in the owned test process tree. It denies `unshare`, verifies that the proxy launch fails before the child marker exists, and pairs this with a successful blocked-network control. It changes no host namespace settings.

Final checks: `NONO_ACCEPTANCE_BINARY=/tmp/nono-expansion-runtime-081/nono make validate verify-package` passed (full provider race tests, 13.0s, runtime packaging and release negatives). The independent public SDK consumer passed. Host common/policy race suites passed with SQLite/PostgreSQL. The rebuilt packaged runtime suite passed uncached with `go test -trimpath -race -p=4 ./internal/agent/sandboxruntime -count=1` (78.7s), including approval, private MCP, retained state, report, and cleanup checks. A cached run was not used as package acceptance evidence. Public documentation checks passed (63 tests, 54 published pages).

This slice is complete. Task 06 remains in progress: schema 2, explicit migration, approval envelopes, service actors, native TCP decisions, desktop/phone review, and the full DNS/alias/IPv6/concurrency/MCP conformance matrix remain pending. Current-pair managed acceptance remains Task 10.
