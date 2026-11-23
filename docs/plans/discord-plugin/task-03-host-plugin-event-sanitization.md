---
id: "03-host-plugin-event-sanitization"
title: "Strip raw_content from plugin event deliveries"
status: cancelled
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-003
acceptance_criteria:
  - AC-PLUGINS-CHAT-BRIDGE-HOST-003.1
  - AC-PLUGINS-CHAT-BRIDGE-HOST-003.2
system_design:
  - ../../specs/plugins/system-design/chat-bridge-host-controls.md
---

# Task 03: Strip `raw_content` from plugin event deliveries

Repository: `kdlbs/kandev`. Use `/fix` and `/tdd`. This repairs a documented
contract: plugins never see injected system content.

- **Acceptance:**
  1. A plugin subscribed to `message.added` or `message.updated` never
     receives a `raw_content` key, including when the message has
     `<kandev-system>` content. `content` stays stripped, and
     `metadata.has_hidden_prompts` stays.
  2. WebSocket clients and other bus subscribers still receive `raw_content`
     unchanged.
- **Verification:** `cd apps/backend && go test ./internal/plugins/delivery/...`
- **Files likely touched:** `apps/backend/internal/plugins/delivery/payload.go` (or the conversion point in `deliverer.go`), `apps/backend/internal/plugins/delivery/deliverer_test.go`.
- **Dependencies:** None (the ADR in task-01 records it).
- **Parallelism:** parallel-safe with the other host tasks (`delivery/` only).
- **Inputs:**
  - The publisher at `apps/backend/internal/task/service/service_events.go:1417-1419`
    (`origin/main` 018808f8).
  - `delivery/payload.go` `dataToMap`.
  - The sanitized-content statements in `docs/public/plugins-manifest.md`.
- **Risks:** A third-party plugin may depend on `raw_content`. No official
  plugin is known to; check the catalog.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Cancelled by the maintainer decision on
[#5420](https://github.com/kdlbs/kandev/issues/5420#issuecomment-7083766483):
`message.*` event payloads keep `raw_content` so existing consumers do not
break. The field came from the web UI chat rendering (#272), and plugin
delivery forwards the bus payload unchanged. `Messages().List` stays stripped.

The stripping change was implemented and then reverted on this branch; plugin
delivery is unchanged from `main`. Task 04 instead corrects the public docs:
`message.*` payloads may include `raw_content` with the unstripped
`<kandev-system>` content, and plugins that must not handle system content
rely on `content` or `Messages().List`.
