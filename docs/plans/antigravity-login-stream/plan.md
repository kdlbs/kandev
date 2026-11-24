---
created: 2026-11-08
status: done
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-015
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
legacy_specs: []
---

# Fix plan: Antigravity login stream startup

## Overview

Repair [issue #5193](https://github.com/kdlbs/kandev/issues/5193) so a successful
agentctl WebSocket connection lets explicit Antigravity sign-in initialize ACP
and authenticate. One sequential work order owns the caller correction and
regression tests. Reuse the existing [requirements](../../specs/agents/requirements/antigravity-acp-agent.md)
and clarify the [sign-in design](../../specs/agents/system-design/antigravity-acp-agent.md#explicit-local-sign-in).
The requirements remain draft because wider native qualification is incomplete;
this package adds no new product behavior or acceptance identifiers.

## Confirmed intent and evidence

The operator requested an isolated local worktree, a fix, and a PR after the
read-only diagnosis. The scope is the confirmed standalone-server sign-in bug.
No material implementation choice requires further clarification.

On macOS arm64 at `88c81ce1ba2e2e56a433dfa984be828b2b7400f8`, installation
succeeded and two sign-in attempts connected their WebSocket, then immediately
deleted the instance. The snapshot reported `setup_failed` with
`antigravity login stream closed before it connected` before OAuth began.
A temporary Go overlay diagnostic reproduced this on 20/20 healthy local
WebSockets, with a preconnected positive control. It was removed after the
issue was published. The new worktree starts from remote main
`b18866572ae445d8c2a76c73cb76f51d30c742c5`; the affected source is unchanged.

`configureAntigravityLoginInstance` sends the return of `Client.StreamUpdates`
to `streamErr` from a goroutine. The client returns `nil` after connecting and
starting its own reader. If `waitForAntigravityStream` entered its select before
the connection completed, it interprets that nil result as stream termination
and fails without rechecking the connected socket. Deferred cleanup deletes
the instance. The prior argv repair in #5052 does not cover this later step.

## Scope

### In scope

- Call the existing stream setup API according to its synchronous contract.
- Cover the real configure/start/stream/initialize/authenticate caller boundary.
- Preserve actionable dial, disconnect, and cancellation failure behavior.
- Keep the original sign-in delivery record linked to this repair.

### Out of scope

- Migrating to `agy acp`, importing CLI credentials, or changing discovery.
- Changing public generic client APIs, redesigning transport behavior, changing
  native bundle pins, auth policy, or UI markup.
- Completing real Google consent or the wider browser/native qualification gates.
- Restarting or mutating the user's running Kandev instance.

## Technical approach

In `apps/backend/internal/agent/hostutility/antigravity_login.go`, call
`inst.client.StreamUpdates` directly after successful Start. Wrap a non-nil
setup error with `open antigravity login stream`, and initialize only after
success. Delete `waitForAntigravityStream` and its polling/channel machinery.
Keep the attempt context, client-owned reader, and existing cleanup owner.
Pending RPC registration must be synchronized with stream retirement, so an
in-flight request is either included in disconnect cleanup or rejected before
it can be registered on a retired stream. Do not add another reader, retry
loop, or global connection owner.

Add `antigravity_login_stream_test.go` beside the caller. Reuse the existing
command fixture and real `agentctl.Client`; supply local HTTP responses for
capability, configure, and start, then a real `httptest` WebSocket exchanging
correlated initialize and authenticate messages. No vendor process or credentials
are needed. Gate handshake and responses with channels to control ordering;
do not create a production export or injection hook for the test.
The success assertion must observe initialize and `oauth-personal` authenticate,
not merely a nil stream result. Verify a live connection remains available until
owned cleanup, and exercise dial rejection, disconnect during initialize, and
cancellation while connection setup is pending. Join fixture handlers/readers.

| Caller / transport | Intended behavior | Evidence / fallback |
| --- | --- | --- |
| Local Antigravity, agentctl WebSocket | Successful setup proceeds to initialize and authenticate. | Real-client boundary regression; no synthetic OAuth readiness claim. |
| Rejected WebSocket handshake | Preserve the setup error; no initialize request. | Dial-failure subtest with reachable healthy control. |
| Disconnect during initialize | Fail through existing pending-request handling. | Disconnect subtest; no retry or successful sign-in. |
| Cancel pending connection setup | Return promptly and release owned resources. | Channel-controlled cancellation subtest. |
| Other providers / generic stream client | A request racing stream retirement is drained or rejected immediately. | Active-stream registration regression and focused client tests. |

## Tests

- `TestAntigravityLoginStreamStartup`: protects AC-AGENTS-ANTIGRAVITY-ACP-010.4
  and AC-AGENTS-ANTIGRAVITY-ACP-015.1 through real stream initialization and
  authentication. Demonstrate the old caller fails before applying the correction.
- `TestAntigravityLoginStreamFailure`: protects AC-AGENTS-ANTIGRAVITY-ACP-010.5
  for dial rejection, disconnect, and cancellation.
- `TestRegisterPendingRequestRejectsDisconnectedStream`: prevents a late
  authentication request from registering after stream disconnect cleanup.
- Existing `TestAntigravityLoginConfigurePreservesCommand` and attempt lifecycle
  tests continue to protect the prior argv and ownership corrections.

## End-to-end evidence

The new backend boundary regression exercises the actual sign-in caller and
transport without a browser or Google account. No rendered UI changes are
planned. Existing browser setup and native OAuth qualification remain owned by
[Task 03](../antigravity-local-subscription/task-03-google-sign-in.md) and
[Task 07](../antigravity-local-subscription/task-07-native-qualification.md);
this repair does not mark either wider gate complete.

## Work orders

- [x] [Task 01: Preserve successful sign-in stream startup](task-01-stream-startup.md)

## Verification results

Implemented and verified on November 8, 2026. The new real-client startup test
failed on the old caller with the reported false-closure error, then passed
after the direct StreamUpdates call replaced the wait loop. The failure-path
test covers handshake rejection, disconnect during initialize, and cancellation
classified by the existing attempt owner.

The focused regressions, full hostutility package with the race detector,
full agentctl package with the race detector, late-registration regression,
changed-package lint, specification/catalog checks, documentation coverage
preflight, and diff checks pass. The
[work order](task-01-stream-startup.md#results) records commands and environment
details. Internal design and companion delivery records are synchronized.
Public setup instructions and rendered UI are unchanged; this restores the
existing sign-in behavior. Real Google consent remains unverified.

## Risks

- A test stopping at Start misses this bug; require actual WebSocket RPCs.
- Connection order must be controlled without arbitrary sleeps or test-only APIs.
- Passing fixture tests does not prove Google consent, subscription entitlement,
  or vendor startup on every platform.
