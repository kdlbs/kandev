---
id: "01-contain-proxy-credentials"
title: "Contain consumed control credentials at executor proxies"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PROXY-CREDENTIAL-001
acceptance_criteria:
  - AC-EXECUTORS-PROXY-CREDENTIAL-001.1
  - AC-EXECUTORS-PROXY-CREDENTIAL-001.2
  - AC-EXECUTORS-PROXY-CREDENTIAL-001.3
  - AC-EXECUTORS-PROXY-CREDENTIAL-001.4
  - AC-EXECUTORS-PROXY-CREDENTIAL-001.5
system_design:
  - ../../specs/executors/system-design/proxy-control-credential-containment.md
---

# Task 01: Contain Proxy Control Credentials

## Summary

Authenticated executor previews must not receive the agentctl control credential.
Preserve admission, credential fencing, and disabled-auth app-header behavior.

## In scope

Trusted consumption marking in accepted static/dynamic auth branches and the
shared outbound request projection used by port/editor proxies. Real HTTP,
stream and upgrade integration; preserve gateway upstream authentication tests.

## Out of scope

Changing the nested app interlock, global Authorization stripping, new original
application-auth transport, provider lease policies, or main/active-agentctl restart.

## Acceptance

1. Real authenticated proxy plus real destination settings interlock succeeds
   with a valid SPA header, matching direct behavior; app sees no control auth.
   Static and rotating control credentials use the same containment boundary.
2. Invalid/revoked control credentials are refused and streams retain captured
   generation fencing. Disabled auth preserves Basic and Bearer app headers.
3. HTTP body/method/query/Host/cookies/interlock and app WebSocket negotiation
   are preserved. Port and editor proxy projections agree; no credentials logged.

## Regression sequence

Create `TestPortProxy_ConsumesControlCredentialBeforeApplication` in
`port_proxy_test.go` using real `instanceAuth`, `handlePortProxy`, and the real
`httpmw.InterimSettingsInterlock` at the destination. Expect success and absent
Authorization; verify RED returns the reproduced 403 before touching production.
Add accepted rotating control credential, bad credentials with zero app requests,
auth-disabled app Basic/Bearer, request-body, stream and upgrade cases. Exercise
`credentialScopedRequest` through both proxy handlers. Do not substitute a fake
agentctl that strips the token before the real code runs.

## Files likely touched

- `apps/backend/internal/agentctl/server/api/auth.go` and `auth_test.go`.
- `apps/backend/internal/agentctl/server/api/credential_rotation.go` and tests.
- `apps/backend/internal/agentctl/server/api/credential_fencing.go` and a focused test.
- `apps/backend/internal/agentctl/server/api/port_proxy_test.go`, `vscode_proxy_test.go`.
- Proxy handlers only if necessary for shared projection wiring.
- Scoped `apps/backend/internal/agentctl/server/api/AGENTS.md` for the invariant.
- Public forwarding guidance if it describes the repaired boundary.

## Verification

Run from repository root; do not alter main :9998:

```bash
(cd apps/backend && go test ./internal/agentctl/server/api -run 'Auth|Credential|PortProxy|VSCode' -count=1)
(cd apps/backend && go test -race ./internal/agentctl/server/api -run 'Credential|PortProxy|VSCode' -count=1)
(cd apps/backend && go test ./internal/common/httpmw ./internal/gateway/websocket -count=1)
(cd apps/backend && make lint)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/session/port-forward-dialog.spec.ts --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Builds happen only through owned artifact paths. Record every result and any
unavailable container/remote-provider coverage separately. Use a fresh owned
agentctl proxy for live verification; do not claim the existing :38383 forwarder
changed when it still uses the main runtime's old component.

## Dependencies

None. Primary session only; sequential, no delegation.

## Risks

Consumption marking must cover static and rotating branches, including both
accepted instance-auth paths. Clone headers without changing the original control
request or repeating a credential-source lookup. Preserve WebSocket fencing.

## Inputs

Executor requirement/design and the linked ADR. Temporary reproduction evidence
under `/tmp/kandev-pr3293-test-runtime/port-proxy-followup-diagnostic.log`.

## Results

Implementation and scoped verification complete. The permanent real-interlock
regression failed RED with static and rotating proxy 403, then passed after
trusted consumption marking and outbound header cloning. Both proxy handlers
preserve disabled-auth Basic/Bearer, reject invalid/revoked requests before the
app, retain HTTP data, and contain credentials on streaming/WebSocket paths.
Final focused auth/credential/proxy suite passed under `-race` (4.496s).

Fresh owned production-API proxy at :48595 passed a real browser selection save
(200), reload persistence, zero local errors/reload banners, and restoration
(200) to baseline IDs. The first reload attempt timed out while the UI loaded;
its saved choice was independently observed and restored. A fresh retry passed
all browser assertions. Evidence is in `repair-owned-proxy-live-retry.log`.

Gateway/interlock command ran twice; the gateway's unrelated
`TestStartUserShellProcessExecutesPersistedTerminalCommand` could not produce
its local shell marker, including with an isolated HOME. It does not exercise
agentctl forwarding. The remaining gateway suite and interlock tests initially passed
with that test excluded. A clean shell check then passed the previously failing
case. Finally the full gateway (4.164s) and interlock (0.018s) suites passed
without exclusions, using isolated HOME/config, Bash, cleared startup/BASH_ENV/
ZDOTDIR overrides, and the owned cache. This resolved the ambient-shell fixture
failure without production or test changes outside this repair.

The first lint/build failed because shared Go cache files vanished mid-run.
An owned cache contains the reruns. Full backend lint reports zero issues;
all 14 port-forward browser tests passed (one worker, 5.4m).
Cross-platform helpers built before the original backend link failure. The
redundant cold cross-platform rebuild was stopped; native `make build-dev`,
web `build:e2e`, and fixture packaging prepare the host runner. This is a host
preview, without new container/remote-provider runtime coverage.
