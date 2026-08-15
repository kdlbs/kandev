---
spec: "../../specs/redmine-plugin/spec.md"
created: 2026-08-15
status: in_progress
---

# Implementation Plan: Redmine Connector Plugin

## Overview

The host remains provider-neutral. `yattdev/kandev-plugin-redmine` owns all Redmine
API knowledge: auth, project/field discovery, issue read/write, sync, watchers, and
settings UI. Unlike the Bitbucket plugin, **no host-side contract work is required**:
`kdlbs/kandev` PR #2117 already shipped the generic seams this plugin needs
(`registerIntegrationSettings`, `registerTaskAction({placement:"link"})` +
`host.openTaskLinkDialog`, `reference_sources` composer search, `PluginOwnedTaskTrees`
cascade delete), and `kandev-plugin-bitbucket` has already proven them end-to-end for
a comparable connector. This plan is almost entirely a **plugin-repository** plan; the
only host-repo deliverables are this spec/plan, a contract E2E suite, and — once a
release exists — the `plugin-registry/plugins.yaml` catalog pointer.

An earlier native `internal/redmine` implementation (full backend package, frontend
settings/linking UI, passing 6-test Playwright suite) was built for this same task
before the architecture was redirected to the plugin model. It is preserved on
`archive/redmine-native-implementation` as a porting reference for field-mapping and
echo-suppression logic — deliberately *not* restored, and this plan does not assume
familiarity with its exact code shape, only its acceptance criteria (carried into this
spec's Scenarios).

## Host contracts

None required. All seams this plugin needs already exist on `main`:

- `registerIntegrationSettings` (frontend plugin registry)
- `registerTaskAction({placement:"link"})` + `host.openTaskLinkDialog`
- `reference_sources` dynamic composer search with submit-time reauthorization
- `PluginOwnedTaskTrees` (`PreviewPluginOwnedTaskTree` / `DeletePluginOwnedTaskTreeRequest`)
- `Tasks().Create` / `Tasks().Update`, `GetState`/`SetState`, `GetSecret`/`SetSecret`/`DeleteSecret`

If implementation surfaces a genuine host gap (not just an inconvenience solvable
plugin-side, per the `kandev-plugin-bitbucket` precedent of building its own
credential-isolation and health-poll layers), stop and record it as a new host task
here rather than silently working around it — this is the plan's own version of the
dw-work "architectural change not in the plan" stop condition.

## Plugin repository

- Bootstrap `yattdev/kandev-plugin-redmine` from `kdlbs/kandev-plugin-template` in its
  attached Kandev workspace (a sibling task/worktree, never nested under the host
  repo). Rename template identity throughout (manifest `id`, Go module, Makefile
  binary/package names, UI registration id, release asset name) to `redmine`.
- Implement connection, project/field mapping, issue read/write + attachments, task
  linking + bidirectional sync + echo suppression, issue watchers, and the native
  settings UI, per the spec's Scenarios.
- Own workspace-scoped secret composition and the ~90s jittered health-poll loop
  in-process (no host `healthpoll` equivalent exists for plugins).

## Tests

Each plugin-repo task runs its own `go test ./...` / `make test` / `pnpm test` scoped
to owned paths, per that task file's Verification section — mirroring
`kandev-plugin-bitbucket`'s per-task verification convention rather than one
end-of-plan full-suite run.

## E2E tests

`apps/web/e2e/tests/plugins/redmine-*.spec.ts` in `kdlbs/kandev`, contract-level only
(install the plugin, exercise settings/linking/mention surfaces through the host UI) —
mirrors `apps/web/e2e/tests/plugins/bitbucket-*.spec.ts`. Deep Redmine-API-shaped
testing (mock client behavior, sync cursor edge cases, echo suppression) lives in the
plugin repo's own test suite, not here.

## Implementation waves and task files

Wave 0 (design):
- [x] [task 01 — design package](task-01-design-package.md)

Wave 1 (plugin repository bootstrap, depends on 01):
- [x] [task 02 — plugin repository bootstrap](task-02-plugin-repository-bootstrap.md)

Wave 2 (core plugin behavior, depends on 02 — parallelizable within the wave):
- [x] [task 03 — connection, secrets, health poll](task-03-connection-secrets-health.md)
- [x] [task 04 — projects and field mapping](task-04-projects-field-mapping.md)
- [x] [task 05 — issue read/write and attachments](task-05-issue-read-write-attachments.md)

Wave 3 (sync and native surfaces, depends on wave 2):
- [x] [task 06 — task linking and bidirectional sync](task-06-task-linking-bidirectional-sync.md)
- [x] [task 07 — issue watchers](task-07-issue-watchers.md)
- [x] [task 08 — settings UI and native registrations](task-08-settings-ui-native-registrations.md)

Wave 4 (release, depends on wave 3):
- [ ] [task 09 — contract E2E, release, registry pointer](task-09-contract-e2e-release.md)

Expect a corrective sub-wave (task 08b, 08c, ...) after live/manual UI evaluation,
per the Bitbucket plan's precedent — do not assume one settings-UI task is the final
word on parity with Jira/Linear/Sentry's native settings pages.

## Current status

2026-08-15: Tasks 01-08 (waves 0-3) are complete on the plugin repo
(`yattdev/kandev-plugin-redmine`, branch `feature/redmine-plugin-build-os2`, local-only,
not yet pushed to `origin/main`): bootstrap, connection/secrets/health poll, projects and
field mapping, issue read/write and attachments, task linking and bidirectional sync,
issue watchers, and the native settings UI with `reference_sources` wiring. The
plugin's own Go test suite covers connection.save/link.set round-trips against an
`httptest` fixture server (`server/actions_test.go`). Only task 09 (contract E2E,
release, registry pointer) remains, gated on `wave 4`'s dependency on 06/07/08.

The `kdlbs/kandev` side of task 09 is already in progress on a sibling task/branch
(`feature/feat-implement-redmi-ib8`): `apps/web/e2e/tests/plugins/redmine-packaged-plugin.spec.ts`
exists there (commit `4ed46b9c6`), gated on `KANDEV_REDMINE_PLUGIN_PACKAGE` and scoped to
zero-network assertions (unconfigured defaults, settings-page rendering, Link dialog
opening) — it deliberately does not exercise a real connection.save/link.set flow, which
is covered instead by the plugin repo's own httptest-backed tests above. Remaining task 09
work on this branch: `make package-host`, install/exercise against a disposable dev
instance, cut a GitHub Release on `yattdev/kandev-plugin-redmine`, and add the
`plugin-registry/plugins.yaml` pointer once that release exists.

## Risks

- **Secret isolation is entirely plugin-built.** The host's flat
  `plugin:<id>:secret:<key>` namespace has no workspace dimension; a bug in the
  plugin's own key-composition or encryption layer is a cross-workspace credential
  leak, not merely a missing feature. Treat this with the same rigor the native
  integrations' `AGENTS.md` demands of `resolveWorkspaceID`/`authorizeWorkspaceAccess`.
- **No host healthpoll or watchreset equivalent.** The plugin must get its own
  polling/backoff/health loop and its own watcher dedup/cursor/reset logic right,
  with no shared library to lean on — exactly the two things
  `project-new-integrations-are-plugins-not-native` flags as "still NOT" provided to
  plugins.
- **First issue-tracker plugin.** `registerReviewProvider` is change-request/PR-shaped
  and does not fit a Redmine issue; there is no issue-shaped host provider registry to
  reuse, so status/priority/label surfaces must go through generic task fields, not a
  specialized review-provider UI.
- **Throttle-cap correctness (carried from the native plan's own risk log).** If the
  plugin's watch-metadata key used to gate `maxInflightTasks` does not exactly match
  what it writes into created tasks' metadata, the cap silently never applies — this is
  the same class of bug Sentry originally shipped with natively; task 07's acceptance
  must assert enforcement, not just persistence.
- **Diff size and repo split discipline.** Keeping host-repo and plugin-repo changes
  cleanly separated (per each task's Owned paths) matters more here than in a
  single-repo integration, since Review and CI operate per-repository.
