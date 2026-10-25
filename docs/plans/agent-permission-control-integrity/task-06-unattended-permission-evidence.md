---
id: "06-unattended-permission-evidence"
title: "Prove unattended and attended behavior end to end"
status: done
wave: 2
depends_on:
  - "07-initial-session-mode"
  - "01-auto-approve-never-denies"
  - "02-confirm-session-mode"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.3
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 06: Prove unattended and attended behavior end to end

## Summary

The reported failure is a workflow outcome, so unit coverage does not close it.
Run the same scenario twice against the mock agent with only the agent profile
different, and assert both the capability and the restriction.

## Scope

- Add a `cmd/mock-agent` scenario that requests permission for a
  state-changing shell command, so the check needs no real provider and no
  network. Follow the recipe in `apps/backend/cmd/mock-agent/AGENTS.md` and
  rebuild the mock agent before running E2E.
- Add a Playwright spec in `apps/web/e2e` that runs the scenario twice against
  one repository, one executor, and one workspace mode:
  - unattended-permission profile: the tool call completes, no pending
    permission request is surfaced, and the test answers nothing;
  - default profile: a pending permission request is surfaced, the tool call
    stays pending, and it completes only after the test answers it.
- Add backend integration coverage asserting the same contract at
  `process.Manager.handlePermissionRequest` for the option shapes in
  `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003`, so a regression is caught below
  the browser layer too.

## Exclusions

- No test against a real provider CLI or network.
- No assertion about which commands a real provider classifies as needing
  permission.
- No new executor type or container scenario; this runs in the default
  Playwright project, not `containers`.

## Acceptance

1. The unattended direction passes with no human interaction and no pending
   permission request, and the assertion resolves the resulting commit object
   rather than checking for the absence of an error message.
2. The attended direction surfaces a pending permission request, blocks the
   tool call, and completes after the answer.
3. Both directions differ only by agent profile.

## Files likely touched

- `apps/backend/cmd/mock-agent/` (new scenario plus its registration)
- `apps/backend/cmd/mock-agent/AGENTS.md`
- New `apps/web/e2e/tests/agent-permission-unattended.spec.ts`
- `apps/web/e2e/fixtures/` (profile seeding helper, if one does not already
  exist for a per-test agent profile)
- New `apps/backend/internal/agentctl/server/process/manager_permission_contract_test.go`

## TDD sequence

1. Write the attended direction first; it should pass on current behavior and
   is the guard against over-correcting work order 01.
2. Write the unattended direction; confirm it fails before work orders 07, 01 and 02
   land, or documents the surviving provider-side refusal if it still fails
   after them.
3. Add the mock-agent scenario needed by both.
4. Add the backend integration contract test.
5. Rebuild the mock agent and run both directions; both pass.

If the unattended direction still fails after work orders 07, 01 and 02, stop and
report: that is the evidence that the remaining refusal is provider-side, which
the plan's "Where the denial comes from" section anticipates. Do not add a
Kandev-side command allowlist to force it green.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && make build-mock-agent && go test ./internal/agentctl/server/process/... -race -count=1
cd "$(git rev-parse --show-toplevel)/apps/web" && pnpm e2e:run --grep "agent-permission-unattended"
```

`make build-mock-agent` is the rebuild step; the mock agent must be rebuilt
before the E2E run or the new scenario is not present in the binary.

## Dependencies

Work orders 07, 01, and 02.

## Risks

An E2E check that only ever asserts the success case says nothing about the case
the mechanism exists for. Both directions are required; do not land one without
the other.

## Results

Done, and the unattended direction passes.

Implemented:
- `cmd/mock-agent` scenario `git-commit-permission`: writes a probe file with
  per-run unique content, requests permission for `git commit`, and creates the
  commit only when granted, emitting the resulting SHA.
- `apps/web/e2e/tests/chat/agent-permission-unattended.spec.ts` runs the same
  scenario twice against one repository, executor and workspace mode, differing
  only by agent profile. Blanket auto-approval is turned off for the file so
  the profile is the only variable.
- `manager_permission_contract_test.go` asserts the same contract below the
  browser layer.

Two defects the first run surfaced, both fixed rather than retried away:
- The transcript was read once instead of asserted on, so the read raced the
  scenario's closing line. It now asserts with an auto-waiting locator first.
- `git commit` exited 1 because the worktree carries no committer identity, and
  a repeated run staged nothing. The scenario now passes an explicit identity
  and writes unique content per run.

Verification (2026-09-22):

```
cd apps/backend && make build-mock-agent && go test ./internal/agentctl/server/process/... -race -count=1
cd apps/web && pnpm e2e:run --grep "Unattended permission for a state-changing Git command" --repeat-each=3
```

6 passed, 0 flaky.

What this does and does not establish: with the mock agent, an unattended
profile now runs a state-changing Git command with nobody answering, and the
default profile still holds it until a person does. It does not establish what
a real provider does once the mode reaches its process — that is the field
measurement the reporter still owns, and the repository correlation recorded in
the plan remains the leading open candidate for their case.
