---
id: "01-sdk-error-origin"
title: "Identify received ACP error responses"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-004
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-004.1
  - AC-PLATFORM-TURN-CONTINUITY-004.2
  - AC-PLATFORM-TURN-CONTINUITY-004.4
system_design:
  - ../../specs/platform/system-design/acp-prompt-error-continuity.md
---

# Task 01: Identify received ACP error responses

## Summary

Give the shared adapter typed proof that an error came from a correlated remote response.
Deliver the additive API in the maintained SDK fork and pin its immutable revision in Kandev.

## In scope

- Source repository: `https://github.com/kdlbs/acp-go-sdk`.
  Use a separate checkout at `../acp-go-sdk`, based on the pinned `48a57e4292e8` revision or its compatible reviewed successor.
  From the Kandev repository root, this sibling path is outside the current worktree.
  Use an explicitly permitted checkout path if the execution environment restricts that location.
- Add `RequestError.IsRemoteResponse()` with a private marker set at accepted response correlation.
- Leave local constructors, send/decode failures, deadlines, cancellation, and disconnects unmarked.
- Preserve `errors.As`, the peer-disconnect sentinel, notification ordering, and existing JSON serialization.
- Add positive and negative pipe-based tests, including identical-looking remote and local internal errors.
- Include unmatched replies, invalid request IDs, contradictory error/result envelopes, and concurrent request IDs.
- Produce a reviewable SDK change and record the immutable revision consumed by Kandev.
- Update `apps/backend/go.mod` and `go.sum` to that revision, preserving the existing replacement module path.

## Out of scope

- Provider-specific classification, host retention changes, raw stream observers, or a second protocol parser.
- ACP wire changes, module-cache edits, or permanent local filesystem replacements.
- External publication, push, or merge without the user's explicit authorization for that operation.

## Acceptance

1. A genuine matching remote error reports positive origin after the existing notification barrier.
   Local errors and invalid or unmatched envelopes cannot report positive origin.
2. Error wrapping and wire bytes remain compatible. Provider Data cannot set or serialize the origin marker.
3. The host dependency resolves to the recorded immutable fork revision.
   Missing dependency publication is a blocker, not permission to omit provenance checks.

## External publication checkpoint

Prepare the concrete SDK patch, regression results, and local commit before requesting publication permission.
Before pushing or publishing a revision to `kdlbs/acp-go-sdk`, obtain explicit user authorization for that external operation.
If the user already authorized that exact publication in this session, retain that authorization and do not ask again.
Otherwise, stop at the reviewable local result and explain that host dependency pinning waits for the external push.
No external push or approval request belongs to this documentation-only design turn.

## Verification

Start `TestRemoteRequestErrorOrigin` and `TestLocalRequestErrorOrigin` in RED before SDK implementation.
Run from the Kandev repository root after the sibling checkout and immutable revision exist:

```bash
(cd ../acp-go-sdk && go test -trimpath -race . -run 'RemoteRequestError|LocalRequestError|RequestErrorOrigin|NotificationBarrier|NotificationQueue|RequestCancel|PeerDisconnected' -count=1)
(cd apps/backend && go list -m -json github.com/coder/acp-go-sdk)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/server/acp -count=1)
git diff --check -- apps/backend/go.mod apps/backend/go.sum
```

Record the SDK source base, resulting commit, module replacement revision, test names, and actual results.
If an immutable revision cannot be obtained, leave the work order blocked and explain the dependency.
Do not change host lifetime policy before this prerequisite is available.

## Files likely touched

- `kdlbs/acp-go-sdk`: `errors.go`, `connection.go`, `errors_test.go`, and a focused response-origin test file.
- `apps/backend/go.mod`
- `apps/backend/go.sum`
- `apps/backend/internal/agentctl/server/acp/connection_test.go`, only if dependency compatibility needs an additional host regression.

## Dependencies

None. This work order supplies the SDK dependency for Task 02.

## Risks

- A local `RequestError` can resemble a remote internal error exactly.
- The marker must come from request correlation, not its diagnostic or JSON data.
- The maintained fork is a separate repository. Its immutable revision must be available before the host can consume it.

## Parallelism

`sequential`

## Inputs

- [Design: SDK response provenance](../../specs/platform/system-design/acp-prompt-error-continuity.md#sdk-response-provenance)
- [Requirements: 004](../../specs/platform/requirements/transient-turn-runtime-continuity.md#req-platform-turn-continuity-004-continue-chat-after-acp-prompt-errors)
- The current fork's `Connection.handleResponse`, `SendRequest`, `waitForResponse`, and notification barriers.
- The host's `internal/agentctl/server/acp/connection.go` logger-ready wrapper.

## Results

Done. The maintained fork now carries `(*RequestError).IsRemoteResponse()` with an
unexported, nonserialized marker set only by `Connection.handleResponse` for a
correlated, error-only response envelope. Local constructors, context failures,
send/decode failures, peer disconnects, unmatched ids, and contradictory
envelopes remain unmarked.

- Fork source base: `48a57e4292e8`.
- Published branch: `kandev/acp-prompt-error-origin` in `kdlbs/acp-go-sdk`.
- Immutable revision: `5e124db622e9e7ed37d1e49b78a28ffcdb58a444`.
- Kandev pin: `github.com/kdlbs/acp-go-sdk v0.13.6-0.20261029234457-5e124db622e9`.
- `FORK.md` documents the delta.

Verification:

- `go test -race . -run 'RemoteRequestError|LocalRequestError|RequestErrorOrigin|NotificationBarrier|RequestCancel|PeerDisconnected' -count=1`: passed.
- The full fork suite still reports the pre-existing
  `TestConnectionReceivesLargeSingleLineNotification` timeout; it fails on the
  base revision without this change.
- `go list -m -json github.com/coder/acp-go-sdk` resolves the pinned replacement.
