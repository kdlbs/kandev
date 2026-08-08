---
id: "07-channels-comments"
title: "Delete the facade's channels, comments and instructions mirrors"
status: pending
wave: 3
depends_on: ["01-duplication-detector"]
plan: "plan.md"
spec: none
parallel-safe: false
---

# Task 07: Channels, Comments, Instructions

Carries the one **open question** in the plan (D6), which is why it is not in
wave 2 despite being a leaf domain.

## Scope

Delete from `internal/office/service`:

- `channels.go` (145) — `SetupChannel` `HandleChannelInbound` `createChannelTask`
  `GetChannelByID`
- `comments.go` (55) — `CreateComment` `publishCommentCreated`
- `instructions.go` (47) — `CreateDefaultInstructions`

**Keep `channel_relay.go`** — it is run-engine code, not a channels mirror, and
stays in `office/service` (see plan §"What `office/service` keeps").
`NewChannelRelayWithClient` currently shows as unreachable in
`deadcode ./...`; that is a separate question, not this task's.

Identical groups: `HandleChannelInbound`, `CreateComment`,
`CreateDefaultInstructions` (this last one pairs with **`office/agents`**, not
`office/channels`).

`SetupChannel` reads as drifted at 0.975 but is **cosmetic** — verified:

- `s.GetAgentFromConfig` vs `s.agents.GetAgentInstance`: `agents/service.go:207`
  is `return s.GetAgentFromConfig(ctx, idOrName)`, identical including the
  by-name fallback through `GetAgentInstanceByNameAny`.
- `s.createChannelTask` vs `s.repo.CreateChannelTask`:
  `service/channels.go:90` is a pass-through.
- the remaining diff is a reworded ADR-0005 doc comment.

Re-verify both pass-throughs at implementation time before relying on this.

## D6 — OPEN QUESTION, do not decide silently

`publishCommentCreated` differs in two ways:

| | facade (`service/comments.go:41`) | `channels/comments.go:34` |
| --- | --- | --- |
| event source | `"office-service"` | `"channels-service"` |
| payload type | `CommentPostedData` (exported) | `commentPostedData` (unexported) |

The payload type difference is internal and harmless — the wire shape is
identical. **The source string is wire-visible on `events.OfficeCommentCreated`.**
No in-repo consumer filters on it (verified by grep across `internal/` and
`apps/web/`), but a Kandev plugin subscribing to the office event stream could.

Since `office/channels` is the wired owner (`office/routes.go:57-58`), deleting
the facade copy changes the emitted source from `"office-service"` to
`"channels-service"` **whether or not that is intended**. Options:

1. Accept the change (it reflects reality after this refactor).
2. Set `channels`' source back to `"office-service"` to preserve the wire value.

**Ask before implementing.** Do not let a deletion make this choice implicitly.

## Test migration

`office/channels` has only `handler_test.go` (31 LOC). The facade's
`channels_test.go` (114) and `instructions_test.go` (143) cover service-layer
behavior with no sub-package equivalent.

| From | To | Note |
| --- | --- | --- |
| `service/channels_test.go` | `channels/service_test.go` | direct move, receiver → `*ChannelService` |
| `service/instructions_test.go` | `agents/instructions_test.go` | `CreateDefaultInstructions` lives in `office/agents` |
| `service/channel_relay_test.go` | — | **stays**; `channel_relay.go` stays |

If D6 option 1 is chosen, the moved channel tests must assert the new source
string explicitly so the change is pinned rather than incidental.

## Acceptance

1. Detector Section A drops by **3** groups; Section B same-name pairs drop by 2.
2. `office/channels` has service-layer test coverage where it had none.
3. D6 is decided explicitly and the chosen source string is asserted in a test.
4. `channel_relay.go` and its test are untouched.

## Verification

```bash
cd apps/backend && go test ./internal/office/channels/... ./internal/office/agents/... -count=1 -v
cd apps/backend && go test ./internal/office/service/... -run ChannelRelay -count=1
cd apps/backend && go test ./internal/office/... -count=1
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m
```

## Files likely touched

- deleted: `internal/office/service/channels.go`, `comments.go`, `instructions.go`
- moved: `service/channels_test.go` → `channels/service_test.go`;
  `service/instructions_test.go` → `agents/instructions_test.go`
- possibly `internal/office/channels/comments.go` (D6 source string)

## Dependencies

Task 01.

## Parallelism

`sequential` — blocked on the D6 answer.

## Rollback position

Single revert. If D6 option 1 turns out to break a plugin, the source string is a
one-line follow-up revert independent of the deletions.

## Output contract

Summary, files changed, detector delta, **the D6 decision and who made it**, the
test-move table, and re-verification that both `SetupChannel` pass-throughs still
hold.

## Results

Pending.
