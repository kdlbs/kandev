---
status: shipped
created: 2026-05-02
owner: tbd
---

# Slack Integration

## Why

Users live in Slack and surface task-worthy work there: bug reports in `#support`, feature ideas in DMs, alerts in `#oncall`. Today they have to context-switch to Kandev to capture each one, which loses the original message link and discourages capture in the moment. Teams on locked-down Enterprise Slack tenants cannot install bots, so a bot-only integration would shut them out.

## What (v1, shipped)

- Install-wide Slack credentials (one Slack user/team per Kandev install), modeled on the Jira/Linear integrations: settings page with auth-status banner, reconnect CTA, and 90s auth-health polling.
- Single auth mode: **Browser session** — user pastes the `xoxc-` token + `d` cookie pair extracted from their browser; works on locked-down Enterprise Slack tenants where bot installs are not possible.
- Single task-creation trigger: **`!kandev` thread triage** — the trigger polls the connected user's own messages for `!kandev <instruction>`, hands the surrounding thread context to a configured utility agent (with the Kandev MCP server attached), and posts the agent's reply back in-thread. The agent picks the destination Kandev workspace/workflow/repo via MCP tools.
- An 👀 reaction is added to the triggering message on detection so the user can confirm it was picked up.
- Token storage reuses the shared `secretadapter`; no Slack credentials live in plaintext on disk.

## Scenarios

- **GIVEN** a user has pasted their `xoxc-`/`d` cookie pair in settings, **WHEN** they type `!kandev fix the safari login bug` in any Slack channel or DM, **THEN** the trigger reacts with 👀, the configured utility agent picks a workspace/workflow/repo via the Kandev MCP, creates a task, and posts the result back as a threaded reply.
- **GIVEN** a Slack credential is connected, **WHEN** the stored token becomes invalid (revoked, rotated, expired), **THEN** the auth-health poller flips the install to "Reconnect required" within 90 seconds and `RecordAuthHealth` invalidates the cached client so the next probe rebuilds it from current secrets.
- **GIVEN** a `!kandev` message has already been processed (its TS is at or below the watermark), **WHEN** the next poll picks it up, **THEN** it is filtered out and no duplicate task is created.

## Out of scope (v1)

- Bot install (`xoxb-` token) and User OAuth (`xoxp-` token) auth modes.
- Reaction trigger (`:kandev:` emoji), `/kandev <title>` slash command, and "Create Kandev task" message-shortcut entry.
- Posting Kandev task updates back into Slack beyond the initial in-thread confirmation (no live status mirroring).
- Slack as a chat surface for talking to a running agent (no `@kandev <prompt>` runtime, no streaming agent output to Slack).
- Multi-Slack-workspace fan-out within one Kandev install (one Slack user/team per install).
- Routing tasks to specific workflows based on channel or keyword — the utility agent picks the workflow via MCP.
- Importing historical Slack messages.

## Future scope

- Bot-install auth mode with the full trigger set (reaction emoji, slash command, message shortcut) for teams whose admins are willing to install the Kandev Slack app.
- User OAuth auth mode for users who want a properly-scoped token without admin involvement.
- Multi-Slack-workspace fan-out for Enterprise Grid tenants.

## Open questions

- Does the **browser session** mode cover Slack Enterprise Grid org-level tokens, or only single-workspace sessions? (Affects whether Enterprise users get one connection or one per workspace.)
- Should the slash command and message shortcut be available in **User OAuth** mode via a per-user "personal app" install, or restricted to bot mode only?
