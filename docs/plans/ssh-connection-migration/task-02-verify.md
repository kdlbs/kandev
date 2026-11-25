---
id: "02-verify"
title: "Verify imported connections on first use"
status: done
wave: 2
depends_on: ["01-import"]
plan: plan.md
requirements:
  - REQ-EXECUTORS-SSH-MIGRATION-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-MIGRATION-001.1
  - AC-EXECUTORS-SSH-MIGRATION-001.3
  - AC-EXECUTORS-SSH-MIGRATION-001.4
  - AC-EXECUTORS-SSH-MIGRATION-001.6
system_design:
  - ../../specs/executors/system-design/ssh-connection-migration.md
---

# Verify imported connections on first use

Integrate first-use validation into managed SSH admission. Compare route and identity revisions, use strict known_hosts matching where applicable and validate platform support before launching a workload. Keep repair actions in the shared editor. Update public upgrade instructions and localize new messages.

1. Boot with offline pinned direct and bastion hosts. Confirm bindings already exist. Bring each host online and launch a task through the original profile without visiting Settings. Verify the correct credential and pin for both hops, atomic promotion, and safe retry after a concurrent edit.
2. Test unknown/changed destination and bastion keys and a missing identity. Review displays full evidence. Cancel leaves the source intact. A changed source, pin or route invalidates an in-flight admission. Deliberate replacement stays distinct from equivalent import.
3. Desktop and phone present the same actions, keyboard focus and outcome. Verify denied callers cannot inspect settings, probe or bind. Include a real pinned direct route and a single-hop route.

## ASCII UI preview

Desktop, existing executor after upgrade

```text
SSH connections
Production builds    Imported connection
Release builders     Verify bastion fingerprint       [Review]
```

Phone, after selecting Release builders

```text
< SSH connections
Release builders
Verify bastion fingerprint
SHA256:...
[Review host key]
```

Use real state and values. Open the existing Access review before accepting a fingerprint. A healthy profile needs no visit to Settings. Missing evidence opens its ordinary Access view.

## Verification

Run each line from the repository root, under the systemd limit in the [plan](plan.md). Add the named migration tests during implementation.

```bash
cd apps/backend && go test ./internal/sshconnections -run "Migrat|Adopt|Bind"
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-connection-migration.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-connection-migration.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --project containers tests/ssh/managed-adoption.spec.ts
```

## Results

Desktop Chromium and phone Chromium exercised the original profile link, the Access repair view, and cancellation without trusting a host. The full Settings batch passed all 10 desktop and 7 phone cases. The encrypted-key and missing-agent cases checked that preflight identifies the problem and that a live login fails.

Real OpenSSH servers with multiple host keys passed the offline direct and bastion upgrade tests. The saved Ed25519 fingerprint differed from the client's first-choice algorithm. The bastion and destination used separate identities. Each original profile launched a workload after the servers came back online. An actual route edit during admission remained a conflict.

The transport tests verified an actual key rotation and a second valid host-key algorithm on the same server. Inspection uses the saved key when present and displays the replacement for review if the original key is gone. The regression tests passed under the race detector.
