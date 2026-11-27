---
id: "01-keep-launch-profiles"
title: "Keep plugin task launch profiles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.15
system_design:
  - ../../specs/plugins/system-design/plugins-02.md
---

# Task 01: Keep plugin task launch profiles

## Summary

Store a plugin task's launch agent and executor profiles in the task metadata,
and validate them before new task creation. A later workflow start uses the
saved agent profile only when workflow profile policy selects no profile.
Exact-create retries resolve a committed task before they check whether its
profiles still exist.

## In scope

- `backendapp/services.go` and `adapters_plugin_writer_test.go`.
- `plugins/host_write.go`, `host_exact_tasks.go`, and their tests.
- `task/service/external_id.go` and `permissions_external_id_test.go`.
- The requirement and system design updates.

## Out of scope

- Keeping `launch.prompt` for a later start.

## Acceptance

New tasks reject missing profiles before creation. Exact-create retries return
an already committed task when a saved agent or executor profile has since
been removed. Metadata tests fail when `pluginTaskMetadata` stops writing either
profile.

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
- Fixup (rebase) from `apps/backend`: `go test ./internal/plugins -count=1`,
  `go test ./internal/backendapp -run '^TestPluginsTaskWriter' -count=1`, and
  `go test ./internal/task/service -run '^TestFindTaskByExternalIDForCreateUsesCreateAuthorization$' -count=1`
  passed; `bash scripts/lint-go-changed` reported 0 issues on the eight changed
  Go files.
- Fixup (CI): `file-tree-rename.spec.ts` passed 20/20 with `--retries=0
  --repeat-each=5` after selecting Files once the rename dirtied the worktree;
  `mobile-turn-end-history-refresh.spec.ts` passed 8/8 with `--retries=0
  --repeat-each=8` after asserting the auto-scroll toggle's own pressed state.
