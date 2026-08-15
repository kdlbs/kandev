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
- [ ] [task 01 — design package](task-01-design-package.md)

Wave 1 (plugin repository bootstrap, depends on 01):
- [ ] [task 02 — plugin repository bootstrap](task-02-plugin-repository-bootstrap.md)

Wave 2 (core plugin behavior, depends on 02 — parallelizable within the wave):
- [ ] [task 03 — connection, secrets, health poll](task-03-connection-secrets-health.md)
- [ ] [task 04 — projects and field mapping](task-04-projects-field-mapping.md)
- [ ] [task 05 — issue read/write and attachments](task-05-issue-read-write-attachments.md)

Wave 3 (sync and native surfaces, depends on wave 2):
- [ ] [task 06 — task linking and bidirectional sync](task-06-task-linking-bidirectional-sync.md)
- [ ] [task 07 — issue watchers](task-07-issue-watchers.md)
- [ ] [task 08 — settings UI and native registrations](task-08-settings-ui-native-registrations.md)

Wave 4 (release, depends on wave 3):
- [ ] [task 09 — contract E2E, release, registry pointer](task-09-contract-e2e-release.md)

Expect a corrective sub-wave (task 08b, 08c, ...) after live/manual UI evaluation,
per the Bitbucket plan's precedent — do not assume one settings-UI task is the final
word on parity with Jira/Linear/Sentry's native settings pages.

## Current status

2026-08-15: Spec and plan authored following the direct architecture redirection (see
spec's Why section and this task's plan-history). No plugin-repo code exists yet;
`yattdev/kandev-plugin-redmine` exists on GitHub but is empty. Proceeding straight
through to bootstrap + a plugin-repo subtask per the author's explicit instruction,
without pausing for a separate spec/plan review gate.

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
