---
status: draft
system: canvases
requirements:
  - REQ-CANVASES-DEFAULT-AVAILABILITY-001
created: 2026-09-25
owners:
  - kandev
---

# Canvas Default Availability System Design

## Purpose and boundaries

This design graduates the existing canvas composition and entry paths. It does
not change the canvas lifecycle in [agent-authored web apps](agent-authored-web-apps.md)
or the Plugins-owned [isolated runtime](../../plugins/system-design/isolated-web-app-contributions.md).
The initial off-by-default decision in
[ADR-2026-08-26](../../../decisions/2026-08-26-plugin-backed-web-app-canvases.md)
describes first delivery; the rollout and retirement here follow
[ADR 0007](../../../decisions/0007-runtime-feature-flags.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-CANVASES-DEFAULT-AVAILABILITY-001` | Composition, retirement, and verification |

## Composition and retirement

Promote the `prod`, `dev`, and `e2e` profile defaults to true for one stable
release. Keep the active registry entry and backend guards working during that
release. Before promotion, complete the pending authoring-bundle follow-up and
run a real-install security and lifecycle pass: first publication, later grant
review, restricted data, iframe isolation, custom-origin hosting, startup
failure, and rollback.

In the retirement release, construct the canvas service, authoring skill,
notifications, routes, and MCP capability without reading `FeaturesConfig.Canvases`.
Remove the gate from backend service composition, task-session MCP profiles, and
HTTP, WebSocket, SSE, and background entry paths. Keep authorization and
capability checks in those paths. Remove frontend `useFeature("canvases")` checks
from routes, sidebar, task panels, workspace settings, and phone navigation while
preserving their existing permission and data-state checks.

Remove the profile entry, typed config field, active registry definition, feature
response field, and frontend default. Append the exact retired identity
`features.canvases` / `KANDEV_FEATURES_CANVASES` to the append-only registry.
Leave old SQLite override rows untouched. Remove the startup catalog's live
classification for the environment variable. An old explicit or persisted false
value becomes inert.

## UI and recovery

Desktop keeps the existing sidebar and task canvas panel. Phone keeps the
existing workspace canvas navigation and task canvas picker; both share the
same data and permission decisions. No new navigation model is introduced.
Canvas runtime failure still presents the existing recovery action. A missing
or unauthorized canvas never becomes visible solely because the release toggle
is gone.

## Verification

Retain tests for creation, grant review, isolation, startup failure, and
promotion. Replace disabled-flag tests with unconditional availability and
authorization tests. Verify both desktop and phone journeys against a production
profile without a feature override, plus stale false override and environment
regressions at the retirement boundary.
