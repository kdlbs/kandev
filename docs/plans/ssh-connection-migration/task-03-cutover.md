---
id: "03-cutover"
title: "Remove inline setup and new inline execution"
status: done
wave: 3
depends_on: ["02-verify"]
plan: plan.md
requirements:
  - REQ-EXECUTORS-SSH-MIGRATION-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-MIGRATION-001.1
  - AC-EXECUTORS-SSH-MIGRATION-001.4
  - AC-EXECUTORS-SSH-MIGRATION-001.5
  - AC-EXECUTORS-SSH-MIGRATION-001.6
  - AC-EXECUTORS-SSH-MIGRATION-001.7
system_design:
  - ../../specs/executors/system-design/ssh-connection-migration.md
---

# Remove inline setup and new inline execution

Update the SSH executor editor, create/update/test handlers, profile overrides, runtime flag registry, boot projection, reachability, lifecycle, remote cleanup and recovery paths. Remove the optional adoption UI and migrate callers to the shared editor. Keep only readers necessary for historical configuration and runtime evidence. Align the active manager specifications and public documentation at release.

1. Exercise an empty database and a skipped-version upgrade. Only the shared editor exists. Old create/update/test payloads cannot introduce inline connections through alternate callers. Migrated profiles use their original IDs and automatically verify at first use. Only missing, unsupported or conflicting evidence requires repair.
2. Verify a running session, a retained session with full original evidence and one lacking a trusted hop. Prove that recovery or cleanup cannot dial a new endpoint after executor migration. Exercise rollback from a disposable backup and confirm the old installation can read the original data.
3. Verify explicit configuration disables and defaults across desktop/service, HTTP, WebSocket and background workers. Opening an old link offers review or normal editing. An old client cannot reintroduce inline execution. Test the complete upgrade from the previous binary and schema in one step, with no preparatory release or administrator command. Document behavior while SSH is explicitly disabled.

## ASCII UI preview

```text
New SSH executor
Name             [             ]
SSH connection   [Choose shared connection]
                 [Manage connections]
                 [Test executor] [Save]
```

Phone uses the shared connection picker and stacked actions. Existing executor links that need review show the affected host and Review connection.

## Verification

Run each line from the repository root, under the systemd limit in the [plan](plan.md). Add the named migration tests during implementation.

```bash
cd apps/backend && go test ./internal/sshconnections ./internal/runtimeflags ./internal/agent/runtime/lifecycle -run "Migrat|ManagedSSH|LegacySSH|RetainedSSH|Registry"
pnpm --dir apps/web run typecheck
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-connections.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-connections.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --project containers tests/ssh/managed-connections.spec.ts
```

## Results

The editor, management probes and new SSH workloads use shared connections. Regressions cover existing runtime ownership, direct retained connections, incomplete bastion evidence, explicit disable, rejected inline writes, and protected profile overrides. Reopening the old binary on a restored backup preserved the original executor and profiles. Public upgrade and rollback instructions are in `docs/public/executors.md`.
