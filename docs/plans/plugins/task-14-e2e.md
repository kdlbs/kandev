---
id: task-14
title: Playwright e2e for the plugin system
status: pending
wave: 4
depends_on: [task-09, task-12, task-13, task-04]
plan: docs/plans/plugins/plan.md
---

# Playwright e2e for the plugin system

## Title
End-to-end browser test: start the fixture plugin, register it, see it active in
the Plugins settings UI, exercise enable/disable, render its iframe page, and
verify event delivery on task creation.

## Inputs
- Read `apps/web/e2e/README.md` and `apps/web/e2e/fixtures/backend.ts` (how the
  backend is booted for e2e; env scrubbing — see the KANDEV_* env leak guidance).
- Fixture plugin binary from task-04 (`apps/backend/cmd/plugin-fixture`, prints
  `LISTENING <addr>`, `-secret` flag, `/_debug/deliveries`).
- Feature flag: e2e profile sets `KANDEV_FEATURES_PLUGINS=true` (task-10). Confirm
  the e2e backend fixture picks it up.
- Plugin registration/secret flow (clean, no pre-shared secret): (1) start the
  fixture (health needs no secret); (2) `POST /api/plugins/register` with a manifest
  whose `base_url` is the fixture's printed addr → kandev returns the generated
  `webhook_secret` and marks the plugin active after health; (3) `POST {fixture}/_config`
  with `{"secret": <returned webhook_secret>}` so the fixture verifies future
  deliveries with kandev's secret; (4) trigger events and assert. task-04 exposes
  `/_config` for exactly this.

## Acceptance
1. `apps/web/e2e/plugins.spec.ts` (in the default project unless it needs Docker):
   - boots fixture plugin, registers it, reloads Plugins settings page → plugin
     row shows status `active`.
   - disable → badge `disabled`; enable → back to `active`.
   - open detail → iframe present; assert the fixture page's
     `#plugin-dashboard` heading is visible inside the frame.
   - create a task (via UI or API) → poll fixture `/_debug/deliveries` until it
     contains `task.created` (event delivery works end-to-end).
2. Test cleans up: uninstall plugin, stop fixture process.

## Files
- `apps/web/e2e/plugins.spec.ts`
- `apps/web/e2e/helpers/plugin-fixture.ts` (spawn/stop helper capturing `LISTENING`)

## Verification
- `cd apps/web && pnpm e2e -g "plugin"`

## Output contract
Report: which project the spec runs in, and any flake mitigations (polling/timeouts).

## Dependencies
task-09, task-12, task-13, task-04.
