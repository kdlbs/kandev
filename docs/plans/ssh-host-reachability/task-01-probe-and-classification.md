---
id: "01-probe-and-classification"
title: "Single-host SSH probe and failure classification"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-001.4
  - AC-EXECUTORS-SSH-REACHABILITY-001.5
  - AC-EXECUTORS-SSH-REACHABILITY-001.6
  - AC-EXECUTORS-SSH-REACHABILITY-001.7
  - AC-EXECUTORS-SSH-REACHABILITY-001.8
  - AC-EXECUTORS-SSH-REACHABILITY-001.27
system_design:
  - ../../specs/executors/system-design/ssh-reachability.md
---

# Task 01: Single-Host SSH Probe and Failure Classification

## Summary

Export from `lifecycle` a probe that opens an authenticated SSH transport to
one resolved target using its pinned fingerprint, closes it without running a
remote command, and returns a typed outcome. Classify every failure into
exactly one reason from the closed set, walking a total order so a deadline is
never reported as a network failure and a rejected host key is never reported
as an authentication failure. Nothing calls this yet.

## In scope

- New `executor_ssh_reachability_probe.go` exporting the probe result type,
  the reason type with its six constants, `ProbeSSHHost(ctx, target, timeout)`,
  and `ClassifyDialError(err)`. The name is deliberately about the *dial*, not
  the probe: task 05 classifies a launch's own dial error with the same
  function, and a launch is not a probe.
- The probe wraps its own `context.WithTimeout` at the supplied deadline, dials
  through the existing `DialSSH` with `PinnedFingerprint` populated, and closes
  the client on every returning path so an abandoned probe leaks no connection.
- Classification order `config`, `timeout`, `host_key`, `auth`, `network`,
  `unknown`, first match wins. `host_key` keys off the existing
  `errHostKeyMismatch` rather than an upstream error string. `timeout` unwraps
  to `context.DeadlineExceeded` or a `net.Error` timeout. `context.Canceled` is
  **not** a `timeout` and not any other reason: the outcome reports "cancelled,
  no observation" so the caller discards it rather than writing a failure. A
  classifier that folds both context errors together makes every graceful
  shutdown look like a host outage.
- Move `sshTargetFromExecutorConfig` out of `internal/ssh/handlers.go` into
  `lifecycle` as exported `SSHTargetFromExecutorConfig(map[string]string)`,
  and delegate the handler to it. A missing host or a missing
  `ssh_host_fingerprint` fails resolution and classifies as `config` without a
  dial.
- A bastion (`ProxyJump`) host-key rejection classifies as `host_key`, not
  `network`. It arrives as `ssh: bastion dial: …` rather than
  `errHostKeyMismatch`, because only the target hop is fingerprint-pinned; the
  reason names what failed, not which hop. Without this an interception on the
  jump path waits out `failureThreshold` under the wrong reason. See the engine
  design's *Security* section.
- The stored message is the dial error text only. Key material and agent-socket
  contents must not reach it.

## Out of scope

- Any persistence, poller, ticker, hysteresis, or consecutive-failure counter.
- Any caller. `SSHExecutor.HealthCheck` still returns `nil` and
  `HealthCheckAll` is untouched.
- Any change to `POST /api/v1/ssh/test`, which keeps dialing unpinned so a
  first-time user can observe and trust a fingerprint.
- Re-pinning, repairing, or rewriting `executors.config`.

## Acceptance

- A probe against a fake SSH server whose host key matches the pinned
  fingerprint returns a success outcome, opens no session channel, and leaves
  no open client.
- A probe whose presented host key differs from the pinned fingerprint returns
  reason `host_key`, and the target's stored pinned fingerprint is unchanged
  afterwards.
- The classification table returns exactly one reason per constructed error,
  including the two ordering cases: an expired deadline over a dial error, and
  a host-key mismatch over an authentication failure.

## Verification

Start with the ordering table as a failing test — construct an error that is
both a deadline expiry and a dial failure, assert `timeout`, and confirm it
fails before the classification function exists. Then run:

```bash
# From apps/backend:
go test -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'Reachability|ClassifyDialError|ProbeSSHHost'
go test -tags fts5 -race ./internal/ssh/ -run 'SSHTarget|ResolveSSHTarget'
make lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reachability_probe.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reachability_probe_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection.go`
- `apps/backend/internal/ssh/handlers.go`
- `apps/backend/internal/ssh/handlers_test.go`

## Dependencies

None.

## Risks

- The `lifecycle` package is guarded by a `goleak` `TestMain`; a probe that
  closes its client on the success path but not on the classification path
  will surface there rather than in the probe's own assertions. Close in a
  `defer` on the dial's returning path, not at each `return`.
- `golang.org/x/crypto/ssh` error shapes are not a stable API. Confining the
  `host_key` discriminator to Kandev's own `errHostKeyMismatch` keeps a library
  upgrade from turning a security event into `auth`.

## Parallelism

`parallel-safe` with task 02 — disjoint packages, no shared schema or
generated contract.

## Inputs

- System design, sections *The probe* and *Security*.
- `executor_ssh_connection.go` for `SSHTarget`, `DialSSH`, `dialDirect`,
  `buildClientConfig`, and `errHostKeyMismatch`.
- `ssh_fake_server_test.go` for the in-process SSH server harness.
- `internal/ssh/handlers.go` for the projection being lifted.

## Results

Pending.
