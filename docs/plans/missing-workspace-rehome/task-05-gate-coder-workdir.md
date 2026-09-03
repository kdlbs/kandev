---
id: "05-gate-coder-workdir"
title: "Gate Coder launch on workdir admission"
status: completed
wave: 5
depends_on:
  - "04-prove-phase-recovery"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001
acceptance_criteria:
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.3
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.5
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.6
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.7
system_design:
  - ../../specs/executors/system-design/coder-task-root-durability.md
---

# Task 05: Gate Coder launch on workdir admission

## Summary

Reject unsafe Coder SSH workdir configuration before task materialization or
agent startup while preserving independent task, process, port, and dev-server
isolation.

## In scope

- Remote Coder detection at the SSH launch boundary.
- Authoritative `durable` policy and explicit `allow_ephemeral` risk escape.
- Structural and live usability checks for the configured root.
- Collision-free concurrent write probes and ordinary SSH compatibility.
- Actionable profile guidance and public documentation.

## Out of scope

- Creating or mounting the Coder persistent volume.
- Proving an external storage provider's durability guarantee.
- Changing task-directory, process, port, or code-server isolation.

## Acceptance

- An unsafe or unclassified Coder profile fails before Direction or task
  materialization begins.
- The failure identifies the exact configuration correction.
- The risky escape is explicit and still requires a usable root.
- Simultaneous admissions do not share probe paths or introduce a global lock.

## Verification

```bash
cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test(ValidateCoderWorkdirPolicy|SSHExecutorRejectsCoderLaunch)'
cd apps/backend && go test ./internal/orchestrator/executor -run TestResolveExecutorConfig_AuthoritativeSSHKeys
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Results

Coder detection now runs immediately after SSH connection. The selected
profile must declare its workdir policy, and the remote root must pass the
structural and live write checks before Kandev uploads helpers, creates a task
directory, or starts the agent. Concurrent probes use random child paths and
leave the existing per-task and per-session isolation model unchanged.
