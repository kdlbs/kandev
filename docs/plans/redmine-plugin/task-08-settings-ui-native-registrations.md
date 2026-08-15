---
id: "08-settings-ui-native-registrations"
title: "Settings UI and native registrations"
status: completed
wave: 3
depends_on: ["03-connection-secrets-health", "04-projects-field-mapping"]
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 08: Settings UI and native registrations

## Intent

Build the native (non-iframe) settings UI via `registerIntegrationSettings`: connection
form, project picker, field-mapping table, sync-option toggles, and watcher
management. Wire `reference_sources` for `#` composer mentions.

## Owned paths

Attached `yattdev/kandev-plugin-redmine` worktree: `ui/` bundle — settings page,
project picker, mapping table, sync toggles, watcher management, composer mention
source registration.

## Dependencies

Tasks 03, 04. Task 06/07 backend work can land after this UI shell exists, provided
their host contracts (`Tasks().Create/Update`, `plugin_state`) are already stable.

## Acceptance

1. `registerIntegrationSettings({ id: "redmine", label, description, icon, Component
   })` contributes a native settings page, index card, and workspace navigation entry
   — rendered inside the Kandev SPA, not an iframe.
2. The connection form is credential entry (base URL + API key), not an OAuth
   redirect, and shows a clear distinct error per failure mode (invalid key vs.
   API-disabled vs. unreachable).
3. The project picker preloads and persists selections; the field-mapping table
   renders live statuses/trackers/priorities plus the custom-fields
   admin/derived-fallback UI note from task 04.
4. The two sync-option toggles (`autoStatusWriteback`, `syncTitleDescription`) are
   visible and persist independently.
5. `#` mentions resolve Redmine issues through `reference_sources` with submit-time
   reauthorization.
6. `initialize`/`destroy` on the UI bundle are safe to run repeatedly across
   disable/enable cycles in the same browser tab; no duplicate registrations or
   leaked subscriptions.

## Verification

```sh
pnpm test -- ui/
pnpm run typecheck
pnpm run lint
```

## Risks

Expect a corrective follow-up wave (task 08b, 08c, ...) once this is manually
evaluated against Jira/Linear/Sentry's native settings pages for parity, per the
Bitbucket plan's precedent (its task 12 spawned 12b-12i). Do not treat this task's
initial pass as final.
