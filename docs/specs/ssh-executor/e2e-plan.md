# SSH Executor — E2E Test Plan

This document tracks the Playwright e2e coverage for the SSH executor. Lives next to `spec.md` so future contributors don't have to reverse-engineer "what's tested" from the test directory.

## Approach

**Real SSH server, no mocks.** Every SSH e2e test runs against a real `sshd` container the test brings up itself. This keeps test code identical to the production code path (SFTP upload, SSH handshake, port forward, agentctl launch) and lets us trust green tests as evidence the feature works.

For scenarios that need fault injection (host-key rotation, dropped connections, dead agentctl), we operate on the container itself — regenerate keys, drop traffic via iptables, kill processes by pid — rather than mocking those at the backend layer.

## Container project (formerly `docker`)

SSH tests live in the same Playwright project as the existing Docker e2e tests, now renamed from `docker` → `containers`. The project gates on Docker being available on the host and skips cleanly when it isn't. See `apps/web/e2e/README.md` for how to run it.

Env flag: `KANDEV_E2E_CONTAINERS=1`. The legacy `KANDEV_E2E_DOCKER=1` is honored as a deprecated alias for one release.

```
apps/web/e2e/
├── fixtures/
│   ├── docker-test-base.ts          (existing, untouched)
│   └── ssh-test-base.ts             (new)
├── helpers/
│   ├── docker.ts                    (existing)
│   ├── docker-probe.ts              (existing)
│   ├── ssh.ts                       (new — keygen, container lifecycle, rekey, drop-traffic)
│   ├── ssh-image.ts                 (new — buildE2ESSHImage)
│   └── ssh-bastion.ts               (new — 2-container network for ProxyJump)
├── pages/
│   └── SSHSettingsPage.ts           (new)
└── tests/
    ├── docker/...                   (unchanged)
    └── ssh/                         (new — every SSH spec lands here)
```

## sshd image

`kandev-sshd:e2e`, built once per CI run. Alpine + openssh-server + openssh-sftp-server + git + bash + sudo + iptables + a pre-baked `mock-agent` binary at `/usr/local/bin/mock-agent`. The image does **not** pre-bake `agentctl` — every test exercises the SFTP upload path the production code uses.

The image entrypoint:
1. Generates a fresh host key on first start.
2. Reads the worker's public key from a bind-mounted file into `/home/kandev/.ssh/authorized_keys`.
3. Runs `sshd -D -e`.

## Spec files & coverage

Test naming follows the same concern-based grouping the existing `docker/*.spec.ts` files use.

| File | What it covers |
|---|---|
| `ssh/connection-form.spec.ts` | A1–A7: form rendering, field gating, identity-source toggle, default port |
| `ssh/test-result.spec.ts` | B1–B7: successful + failed Test Connection paths, step badges, fingerprint surfacing, "cached / will upload" |
| `ssh/trust-gate.spec.ts` | C1–C7: Save disabled until trust ticked; edits to host/port/user/identity reset result + trust; fingerprint-change amber warning |
| `ssh/executor-crud.spec.ts` | D1–D7: SSH executor CRUD, listing with right icon/label, profile `workdir_root` round-trip, edit-with-live-sessions modal, delete-with-live-sessions warning |
| `ssh/sessions-card.spec.ts` | E1–E6: empty state, row rendering, manual refresh, status badges, truncation, polling pickup |
| `ssh/test-endpoint.spec.ts` | F1–F6: HTTP contract for `POST /api/v1/ssh/test` + WS parity |
| `ssh/sessions-endpoint.spec.ts` | G1–G5: HTTP contract for `GET /api/v1/ssh/executors/:id/sessions` + WS parity |
| `ssh/launch-task.spec.ts` | H1–H8: end-to-end task launch on real sshd, agentctl upload + sha256 cache hit on second launch, per-task / per-session dir layout, cleanup on stop |
| `ssh/concurrency.spec.ts` | I1–I4: two sessions on same task share workdir, two tasks share connection, single SSH conn for same host, keepalive eviction + reconnect |
| `ssh/recovery.spec.ts` | J1–J4: backend restart with live session reconnects; dead remote agentctl handled; two surviving sessions reattach; persisted metadata keys present |
| `ssh/hostkey-rotation.spec.ts` | K1–K4: simulate rekey of the container; mismatch surfaces verbatim; re-test + re-trust flow restores function |
| `ssh/auth-methods.spec.ts` | L1–L5: file + valid key, file + missing key, file + passphrase-protected (error), agent + agent running, agent without SSH_AUTH_SOCK |
| `ssh/config-inheritance.spec.ts` | M1–M6: `~/.ssh/config` alias resolution, form-overrides-config precedence, unknown alias fallback |
| `ssh/proxy-jump.spec.ts` | N1–N3: direct connect, single bastion ProxyJump (2-container network), chained ProxyJump explicit failure |
| `ssh/workdir-per-profile.spec.ts` | P1–P4: different profiles → different remote workdirs; default workdir; profile switch does not move existing task dirs |
| `ssh/error-surfacing.spec.ts` | Q1–Q5: TCP refused, auth failed, permission denied (mkdir), ProxyJump unreachable, backend 5xx |
| `ssh/persistence.spec.ts` | R1–R4: fingerprint persists across reload, incomplete test does not leak state, mobile project responsive, executor icon/label correct in lists |

Roughly 75 cases across 17 files.

## Helpers exposed to specs

- `ssh.ts`
  - `hasSSHContainerSupport(): boolean` — Docker reachable
  - `buildE2ESSHImage()` — idempotent
  - `startSSHServer({ workerIndex }): SSHServerHandle` — generate keypair, start container, return `{ host, port, user, identityFile, hostFingerprint, containerId }`
  - `stopSSHServer(handle)`
  - `regenerateHostKey(handle)` — simulate rekey
  - `dropTrafficToPort22(handle)` / `restoreTraffic(handle)` — simulate connection drop
  - `killRemotePid(handle, pid)` — fault injection for recovery
  - `readRemoteFile(handle, path): string` — assertions on uploaded files (sha256, port files)
- `ssh-bastion.ts`
  - `startBastionAndTarget({ workerIndex }): { bastion, target, network }` — 2 containers in a private network
  - `stopBastionAndTarget(...)`
- `ssh-test-base.ts`
  - `sshTest` Playwright test fixture: extends `dockerTest`-style, pre-seeds workspace/workflow/agent profile, brings up one sshd container, exposes a `seedData.sshTarget` for tests that just need a configured executor.

## Page Object

`SSHSettingsPage` exposes:

- `goto(executorId?)` — navigate to settings; if `executorId` omitted, the "new SSH executor" flow.
- `fillForm({ name, host, port, user, identitySource, identityFile, proxyJump, hostAlias })`
- `clickTestConnection()` / `waitForTestResult()`
- `expectStep(name, status)` — badge assertion per step
- `expectFingerprint(fp)` / `expectFingerprintAny()` — accepts any non-empty fingerprint
- `tickTrust()` / `untickTrust()`
- `clickSave()` / `confirmRunningSessionsModal()` / `cancelRunningSessionsModal()`
- `expectConnectionBadge("trusted" | "unverified")`
- `sessionsTable.expectEmpty()` / `sessionsTable.rowFor(sessionId).expectColumns(...)`

All locators use `data-testid` attributes added to `ssh-settings.tsx` in the same PR.

## API-client helpers

`apps/web/e2e/helpers/api-client.ts` gains:

- `createSSHExecutor({ name, config }): Executor`
- `updateSSHExecutor(id, patch)`
- `listSSHSessions(executorId): SSHSession[]`
- `createSSHExecutorProfile(executorId, { name, workdirRoot })`
- `getExecutorRunning(sessionId)` — for asserting persisted SSH metadata after launch

## Deferred — revisit later

These are intentionally out of scope for this PR. Each one names what unblocks it.

- **arm64 remote host (O1, O2)** — needs `agentctl-linux-arm64` in the build pipeline and `qemu-user-static` available in CI to run an arm64 sshd container. Revisit when the arm64 follow-up lands (see spec.md "Future scope: Linux arm64 agentctl"). Until then, the `requireSupportedArch` gate is covered indirectly by H1 succeeding only when the host is amd64.
- **Chained ProxyJump beyond first hop** — v1 spec says single bastion only. N3 just verifies we *fail* on a chained config rather than silently misroute. A future "chained ProxyJump" feature would add positive cases.
- **macOS / Windows remote** — needs `agentctl-darwin-*` / `agentctl-windows-*` binaries.
- **Reverse-forward of local MCP servers to the remote agent** — not in v1 spec; would need its own test plan when that feature lands.

## Phasing (within this PR)

We don't need formal phases since everything is real-SSH-backed and lands together, but the implementation order is:

1. Plan doc (this file).
2. Rename `docker` project → `containers` + README explanation.
3. SSH harness: image, helpers (`ssh.ts`, `ssh-image.ts`), fixture (`ssh-test-base.ts`).
4. SSH UI test IDs + Page Object.
5. API client helpers.
6. Spec files in roughly the table order.
7. Bastion helper + `proxy-jump.spec.ts` last (heaviest infra).
