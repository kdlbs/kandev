---
id: "03-host-plugin-event-sanitization"
title: "Strip raw_content from plugin event deliveries"
status: done
wave: 2
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/discord.md"
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
The plugin deliverer now drops `raw_content` from every `message.*` payload
after `dataToMap` copies the bus event, so plugins receive only the stripped
`content` and `metadata.has_hidden_prompts`. The shared bus event is not
modified, so WebSocket clients and other subscribers keep `raw_content`.

- **Files:** `apps/backend/internal/plugins/delivery/payload.go`
  (`sanitizePluginPayload`), `apps/backend/internal/plugins/delivery/deliverer.go`
  (call at the conversion point), `apps/backend/internal/plugins/delivery/deliverer_test.go`
  (`TestDeliverer_StripsRawContentFromMessageEvents`, `message.added` and
  `message.updated`; it also asserts the shared event keeps `raw_content`).
- **Tests:** red first (both subtests failed with `raw_content` present), then
  `cd apps/backend && go test ./internal/plugins/delivery/...` passed;
  `scripts/lint-go-changed` on the three files reported 0 issues.
- **Risks:** a third-party plugin reading `raw_content` loses it. The public
  docs already promise stripped content; task 04 adds the explicit statement.
