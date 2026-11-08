---
id: "02-repair-ci-recovery-readiness"
title: "Repair CI recovery and readiness races"
status: done
wave: 2
depends_on:
  - 01-preserve-checkout-identity
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.4
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 02: Repair CI recovery and readiness races

## Outcome

Preserve the retained remote owner while startup recovery is pending. Make
existing browser tests wait for the state that their assertions require.
These corrections address failures from CI run 40246957875 at head 47fe32ea.

## Scope and owned files

- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_admission.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_pending_remote_test.go`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-settings-index.spec.ts`
- This work order and its sibling plan.

## Existing contract

The recovery design already forbids workspace restoration through a startup
recovery guard. Reuse that guard for pending remote owners at the existing
workspace admission boundary. Preserve the retryable refusal and all durable
ownership checks. No new recovery policy or public interaction is introduced.

## Implementation

1. Reject workspace-only creation while the same session retains a pending
   remote recovery guard. Cover session, environment, and explicit restore access.
2. Wait for the Quick Chat opening turn to settle before submitting a title command.
3. Measure the migration control after its drawer animation settles.
4. Wait for each settings tree expansion before the next touch action.

## Validation

The lifecycle regression must fail without the guard and pass with it. After
release, each access path must create its workspace normally. Run the lifecycle
package with the race detector. Run the SSH interruption regression with zero
retries. Run the affected desktop and mobile tests with zero retries. Retain
the original behavioral assertions and single message submissions.

## Results

The regression failed for all three access paths before the correction. The
focused race tests passed afterward. The broader workspace and remote-recovery
race suite passed 266 tests/subtests with one existing skip. Six mobile runs
and the desktop Quick Chat regression passed with retries disabled. CI artifacts identify an execution-store collision during SSH recovery,
a title submission before the opening turn settled, and two animation timing
failures. The two mobile failures did not recur in three unchanged local runs.

The full lifecycle package run did not complete successfully on this Nix host.
Existing restart tests require the absent `/bin/sleep`, and the package reached
the default ten-minute limit. The focused suite above completed in 55 seconds.
Hosted full-suite checks remain required. The isolated Kubernetes stop/restart regression passed unchanged with zero
retries. Local SSH setup was blocked by the host firewall during a Git fetch
from the private Docker bridge to the fixture server. A temporary rule now
permits fixture traffic from that owned bridge. The original SSH setup deadline
is restored. The unchanged SSH interruption regression then passed with zero retries and
its original setup deadline. Full Kubernetes shard validation remains pending.

### Recovery publication ordering review

CodeRabbit proposed fencing late recovery publication against runtime creation.
The production call graph publishes pending owners during startup, before
`publishReadiness` installs the real API and WebSocket router. The bootstrap
handler rejects API and WebSocket traffic before that swap. Background retries
consume existing pending entries and do not queue new owners. The proposed
late-publication interleaving is therefore outside the production lifecycle.
The existing startup guard/activation and pre-swap listener tests passed with
the race detector. The thread was resolved with that evidence. No new mutex
across runtime I/O is required.

The full local Kubernetes shard replay could not validate the preceding test
sequence. Its in-cluster backend never became ready. The native binary uses a
Nix-store ELF interpreter that is absent from the Ubuntu fixture image; direct
execution in that image confirms the missing-interpreter failure. Later workers
also rejected a changed manifest identity after the commit completed. These
results are incomplete evidence. Hosted CI must validate the full sequence.
The owned daemon is stopped, and the private bridge and firewall rule are removed.
