---
id: "07-sprites-redial"
title: "Sprites redial"
status: blocked
wave: 4
depends_on: ["04-reconnect-coordinator"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.6
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity.md
---

# Task 07: Sprites redial

## Summary

`SpritesExecutor` implements `RedialRemoteInstance`. It resolves the sprite
with `reconnectSprite`, opens a new proxy with `setupPortForwarding`, and
replaces the entry in `proxies`.

**Blocked:** this needs a Sprites account or test environment, so that the
redial ships with real evidence. Do not implement from unit fakes alone.
Unblock when an account is available.

## In scope

- **`RedialRemoteInstance`** in a new `executor_sprites_redial.go`. A sprite
  that no longer exists yields `ErrRedialTargetGone`.
- **Transport-loss detection:** Sprites has no watchdog today. A proxy error
  or a failed `GetRemoteStatus` enters Disconnected through task 03's branch.

## Out of scope

- Sprite lifecycle or billing behavior.

## Acceptance

1. With a real sprite, a dropped proxy is re-established by the coordinator
   without restarting agentctl.
2. A deleted sprite yields `ErrRedialTargetGone`.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... -run 'TestSpritesRedial')
make -C apps/backend lint
```

Plus a manual run against a real sprite, recorded in Results with the sprite
region and date.

## Files likely touched

- New `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_redial.go`
  and its test
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_network.go`,
  `executor_sprites_lifecycle.go`

## Dependencies

- Task 04.
- A Sprites test account.

## Risks

- **No watchdog.** Proxy-loss detection latency depends on the proxy
  library's error reporting.

## Parallelism

`sequential`.

## Inputs

- System design section: Redial contract, Sprites.

## Results

Pending.
