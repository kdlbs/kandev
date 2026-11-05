---
created: 2026-10-26
status: implemented
requirements:
  - REQ-PLUGINS-PLUGINS-001
system_design:
  - ../../specs/plugins/system-design/plugins-02.md
legacy_specs: []
---

# Implementation Plan: Plugin task launch profiles

## Overview

Keep the launch profiles a plugin gives when it creates a task without starting
an agent (issue #4803), against the
[requirements](../../specs/plugins/requirements/plugins.md) and
[design](../../specs/plugins/system-design/plugins-02.md).

## Evidence and root cause

`prepareCreate` passes the plugin's launch options to the task writer as
`TaskCreateInput.Launch`, but they are used only for the immediate start when
`start_agent` is set. `pluginTaskMetadata` builds the task metadata from the
plugin's namespaced metadata and `source`, and ignores `Launch`. A workflow
step's `auto_start_agent` (or a step that pulls the task) reads the profiles
from task metadata, finds no executor profile, and a plugin executor fails with
"executor profile not found". Native issue watchers write both profile keys
into the metadata; a plugin cannot, because its metadata is namespaced.

## Scope

- `pluginTaskMetadata` in `backendapp/services.go` stores non-empty launch
  profiles under `agent_profile_id` / `executor_profile_id`.
- `prepareCreate` parses launch options; create paths validate profiles before
  new task creation. Exact creates first resolve an existing task by external
  ID so a retry can recover a committed task after profile removal.
- Requirement AC-PLUGINS-PLUGINS-001.15 and the `Host.Tasks().Create` paragraph
  of the system design.

Out of scope: keeping `launch.prompt` for a later start; a deferred start uses
the step prompt and the task description, as it does for native watchers.

## Work orders

- [Task 01: Keep plugin task launch profiles](task-01-keep-launch-profiles.md)

## Verification

- Before this fixup, the PR author reported `go test ./internal/plugins`
  passing.
- Before this fixup, the PR author reported `go test ./internal/backendapp` passing with
  `TestManagedAttachmentCompatibilityConsumers` skipped after it exceeded a
  15-minute timeout. This does not establish that the full package suite passed.
- Fixup passed from `apps/backend`: `go test ./internal/plugins -run '^(TestExactTaskCreateRecoversDomainCommit|TestExactTaskCreateRejectsUnknownLaunchProfilesForNewTasks|TestPluginHost_Tasks_CreateOnly)' -count=1`.
- Fixup passed from `apps/backend`: `go test ./internal/backendapp -run '^TestPluginsTaskWriter_FindTaskByExternalID$' -count=1`.
- Fixup passed from `apps/backend`: `go test ./internal/task/service -run '^TestFindTaskByExternalIDForCreateUsesCreateAuthorization$' -count=1`.
- Fixup: `bash scripts/lint-go-changed` on the eight changed Go files reported
  0 issues.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- The new tests cover storing both profiles, storing only the ones given, the
  writer receiving them without a start, and rejecting unknown profiles without
  a start. The fixup adds exact-create retry coverage for removal of either
  profile after the domain task commits but before the command receipt completes.
- Fixup (rebase) from `apps/backend`: `go test ./internal/plugins -count=1`,
  `go test ./internal/backendapp -run '^TestPluginsTaskWriter' -count=1`, and
  `go test ./internal/task/service -run '^TestFindTaskByExternalIDForCreateUsesCreateAuthorization$' -count=1`
  passed. `bash scripts/lint-go-changed` on the eight changed Go files reported
  0 issues.
- Fixup (CI): rebased onto current `main`. The only conflict was
  `apps/web/e2e/tests/task/file-tree-drag-drop.spec.ts`, where `main` already
  carried the equivalent stabilization, so the branch kept `main`'s version.
- Fixup (CI): `pnpm e2e:run --host --no-build --project chromium
  tests/task/file-tree-rename.spec.ts -- --retries=0 --repeat-each=5` passed 20
  tests after the scenario selected Files once the rename dirtied the worktree.
- Fixup (CI): `pnpm e2e:run --host --no-build --project mobile-chrome
  tests/chat/mobile-turn-end-history-refresh.spec.ts -- --retries=0
  --repeat-each=8` passed 8 tests after the scenario asserted the auto-scroll
  toggle's own pressed state instead of its swapped label.
- Fixup (CI): the pseudo-locale spec cannot run on the local host because the
  local backend advertises an available Kandev update, which the pseudo check
  flags as un-externalized copy. The CI shard that owns that spec passed with
  this fix on the previous head.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py
  --all`, and the changed-spec ESLint and `e2e:sleep-ratchet` checks passed
  after the rebase.
