---
created: 2026-10-15
status: done
requirements:
  - REQ-COSTS-PROVIDER-USAGE-SNAPSHOT-001
  - REQ-COSTS-PROVIDER-TURN-ALLOWANCE-002
system_design:
  - ../../specs/costs/system-design/provider-usage-snapshot.md
legacy_specs: []
---

# Provider usage snapshot implementation

Deliver native provider snapshots and the first shared Kandev-turn allowance
backend increment on current upstream. Provider HTTP request, token/USD,
model/group limits, learning and other consumer/binding coverage remain original
increments; a turn is not renamed as one provider request.

## Work orders

- [x] [01: Native snapshot and settings](task-01-native-snapshot.md)
- [x] [02: Shared Kandev-turn allowances](task-02-shared-turn-allowances.md)

## Compatibility

| Binding | Reading | Unknown fallback |
| --- | --- | --- |
| Local Claude/Codex shared subscription credential | Existing provider client/cache | Unsupported profile override or unavailable credential/read |
| Explicit local TeamClaude proxy | Existing resolver/client/cache | Unrecognized proxy binding |
| OpenCode Go paid model | Account-wide paid windows | Missing/invalid credential, payload or reset |
| OpenCode Go free / Zen | Account/model identity only | No usage API; no paid windows |
| Other or remote binding | No host read | Unsupported binding |

## ASCII UI preview

UI-01, Settings > Agents, same wrapping composition on desktop and phone:

```text
Profile name                      [existing actions]
[model] [existing fallback]
Provider / shared account / paid models
Provider reading: 5-hour 42% / reset <time>  [Refresh]
```

Loading, unavailable and no-API rows use an explicit localized state instead of
the percentage. The page retains its scroll owner; the refresh button is 44px
and above the card link. Existing model/fallback/actions are preserved.

## Verification

Use existing runtime flag/profile/config and frontend feature contract checks.
Disposable fake HTTP/store and UI checks cover AC-001.1 through .5 without real
provider/model calls. Keep temporary proof outside normal test collection and
remove it after inspection; no permanent test framework or recurring checks.
Exact commands and results belong in the work order. No operating activation.

## Remaining original work

Other provider adapters, observed-window/learned-state migration, provider-request,
token/USD and model/group budgets, their owner/reset/learning editor, and the
separate utility/review inference consumer remain original work. Shared native
Kandev-turn settings and settlement are covered here; they do not stand in for
those other units. Statistics and existing cache-backoff contributions retain
their current owners.
