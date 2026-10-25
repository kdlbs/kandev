---
id: "01-instance-create-deadline"
title: "Give instance creation its own deadline"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001
acceptance_criteria:
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.1
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.2
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.3
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.4
system_design:
  - ../../specs/executors/system-design/agentctl-instance-create-deadline.md
---

# Task 01: Give instance creation its own deadline

## Summary

`ControlClient.CreateInstance` waits up to 120 seconds for the controller.
Other control calls keep their 30-second deadline.

## In scope

- Add `createInstanceHTTPTimeout` and a per-call client copy in
  `ControlClient.CreateInstance`.
- Add the creation-deadline, caller-deadline, and credential tests beside the
  client.

## Out of scope

- Any change to `internal/agentctl/server`, lifecycle callers, or executors.
- Changes to other `ControlClient` methods or the shared client timeout.
- Retry, classification, or configuration of the deadline.

## Acceptance

1. Before the change, `TestCreateInstance_UsesLongerHTTPTimeout` fails with
   about 30 seconds remaining. After the change, it observes a deadline close
   to 120 seconds.
2. The shared client keeps its 30-second timeout. A shorter caller deadline
   still ends creation with a wrapped `context.DeadlineExceeded`.
3. A token-authenticated creation request carries its bearer credential. The
   package tests, build, and vet pass.

## TDD sequence

1. Mark this work order `in_progress` and synchronize the plan.
2. Add `TestCreateInstance_UsesLongerHTTPTimeout`, run it, and record the
   expected failure.
3. Add the constant and the per-call client copy.
4. Add the caller-deadline and credential tests.
5. Run all verification commands and record their results.
6. Mark this work order `done` only after its checks pass.

## Verification

Run from the repository root. The first command is also the required red check
before implementation.

```bash
(cd apps/backend && go test ./internal/agent/runtime/agentctl -run '^TestCreateInstance_UsesLongerHTTPTimeout$' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/agentctl -count=1)
(cd apps/backend && go build ./internal/agent/runtime/agentctl && go vet ./internal/agent/runtime/agentctl)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the local documentation preflight from the repository root.

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/agentctl-instance-create-deadline/plan.md',
  'docs/plans/agentctl-instance-create-deadline/task-01-instance-create-deadline.md',
  'docs/specs/executors/requirements/agentctl-instance-create-deadline.md',
  'docs/specs/executors/system-design/agentctl-instance-create-deadline.md',
];
const result = validateCoverage({
  changedFiles: [
    'apps/backend/internal/agent/runtime/agentctl/control.go',
    'apps/backend/internal/agent/runtime/agentctl/control_test.go',
    ...paths,
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/control.go`
- `apps/backend/internal/agent/runtime/agentctl/control_test.go`
- `docs/plans/agentctl-instance-create-deadline/plan.md`
- `docs/plans/agentctl-instance-create-deadline/task-01-instance-create-deadline.md`

Read-only dependencies include `internal/agentctl/server/instance/manager.go`
and the lifecycle callers of `CreateInstance`.

## Dependencies

None. All work stays sequential.

## Risks

- Replacing the transport instead of copying the client would drop the bearer
  credential and runtime binding guard.
- A test that measures wall-clock remaining time needs a margin. Accept a
  deadline between 119 and 120 seconds.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/agentctl-instance-create-deadline.md)
- [System design](../../specs/executors/system-design/agentctl-instance-create-deadline.md)
- [Evidence and root cause](plan.md#evidence-and-root-cause)
- `apps/backend/AGENTS.md` testing conventions.

## Results

Completed on 2026-10-12. `ControlClient.CreateInstance` sends its request
through a per-call copy of the shared `http.Client` with
`createInstanceHTTPTimeout`, 120 seconds. The copy shares the configured
transport. The shared client keeps its 30-second timeout for every other
control call.

Before the production change, `TestCreateInstance_UsesLongerHTTPTimeout`
failed with `deadline had 30s remaining, want close to 2m0s`. It now passes and
also checks that the shared timeout stays at 30 seconds.
`TestCreateInstance_CallerDeadlineStillApplies` and
`TestCreateInstance_SendsControlAuthToken` passed before and after the change,
confirming that caller deadlines and credentials are preserved.

The package tests, build, vet, specification validation and lint, and the
documentation coverage preflight passed. The tests used stubbed transports and
`httptest` servers only.

PR review follow-up: the per-call copy of the shared client is taken under
`c.mu`, the lock `SetAuthToken` holds when it replaces the transport.
`TestCreateInstance_CopiesClientUnderRotation` rotates the credential while
creating instances; under `-race` it reported a data race without the lock and
passes with it. The same unlocked reads in other control methods are unchanged
and outside this work order.
