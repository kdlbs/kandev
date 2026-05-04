---
status: draft
created: 2026-05-02
owner: tbd
---

# Slack Integration

## Why

Users live in Slack and surface task-worthy work there: bug reports in `#support`, feature ideas in DMs, alerts in `#oncall`. Today they have to context-switch to Kandev to capture each one, which loses the original message link and discourages capture in the moment. Teams on locked-down Enterprise Slack tenants cannot install bots, so a bot-only integration would shut them out.

## What

- Per-workspace Slack credentials, modeled on the Jira/Linear integrations (settings page with auth-status banner, reconnect CTA, link/import affordances gated on availability).
- Three auth modes per workspace, user-selectable:
  - **Bot install** — standard Slack OAuth, app installed by a workspace admin (`xoxb-` token).
  - **User OAuth** — user-scoped Slack OAuth, integration acts as that user (`xoxp-` token).
  - **Browser session** — user pastes the `xoxc-`/`xoxd-` token-and-cookie pair extracted from their browser; explicitly labelled "unsupported, may break" in the UI.
- Three task-creation triggers, available where the chosen auth mode supports them:
  - **Reaction** — adding a configured emoji (default `:kandev:`) to a message creates a task whose body is the message text plus a permalink back to Slack.
  - **Slash command** — `/kandev <title>` from any channel the integration can post in creates a task in a default workflow chosen in settings (bot mode only).
  - **Message shortcut** — "Create Kandev task" entry in Slack's message context menu opens a modal to pick workspace/workflow before creating (bot mode only).
- The integration MUST surface which triggers are available for the current auth mode, and gracefully hide ones that are not.
- A two-way link: created tasks store the originating Slack permalink and channel; the Slack message receives a threaded reply with the task URL.
- Auth health is polled on the same 90s cadence as Jira/Linear; the settings page shows a clear "Reconnect" affordance when the token is invalid.
- Token storage reuses the shared `secretadapter`; no Slack credentials live in plaintext on disk.

## Scenarios

- **GIVEN** a workspace admin has installed the Kandev Slack app, **WHEN** any user in that Slack workspace reacts to a message with `:kandev:`, **THEN** a new Kandev task appears in the configured default workflow with the message text and a permalink in its body, and the bot replies in-thread with the task URL.
- **GIVEN** a user has connected via User OAuth (no admin install), **WHEN** they react with `:kandev:` to a message in any channel they belong to, **THEN** a task is created and the in-thread reply is posted as that user.
- **GIVEN** a user on Enterprise Slack pastes their browser session tokens, **WHEN** the integration probes the tokens, **THEN** the settings page shows a connected status with a visible "unsupported" badge and the reaction trigger is enabled.
- **GIVEN** any auth mode, **WHEN** the stored token becomes invalid (revoked, rotated, expired), **THEN** the auth-health poller flips the workspace to "Reconnect required" within 90 seconds and trigger handlers stop firing for that workspace until reconnect.
- **GIVEN** a Slack workspace is connected via bot mode, **WHEN** a user runs `/kandev fix login redirect bug`, **THEN** a task with that title is created in the default workflow and Slack shows an ephemeral confirmation with the task URL.
- **GIVEN** a Slack message has already produced a Kandev task, **WHEN** the same `:kandev:` reaction fires again (e.g. another user adds the emoji), **THEN** no duplicate task is created and the existing task URL is reposted in the thread.

## Out of scope

- Posting Kandev task updates back into Slack channels beyond the initial in-thread confirmation (no live status mirroring).
- Slack as a chat surface for talking to a running agent (no `@kandev <prompt>` runtime, no streaming agent output to Slack).
- Multi-Slack-workspace fan-out within one Kandev workspace (each Kandev workspace connects to exactly one Slack workspace in v1).
- Parsing Slack threads to construct richer task context beyond the triggering message and a permalink.
- Routing tasks to specific workflows based on channel or keyword — v1 uses one default workflow per Kandev workspace.
- Importing historical Slack messages.

## Open questions

- Does the **browser session** mode cover Slack Enterprise Grid org-level tokens, or only single-workspace sessions? (Affects whether Enterprise users get one connection or one per workspace.)
- For **User OAuth**, does Slack's `reactions:read` scope deliver reaction events via Events API for non-installed apps, or do we need to fall back to polling `reactions.list`? (Affects latency and rate-limit footprint.)
- Should the slash command and message shortcut be available in **User OAuth** mode via a per-user "personal app" install, or restricted to bot mode only?
