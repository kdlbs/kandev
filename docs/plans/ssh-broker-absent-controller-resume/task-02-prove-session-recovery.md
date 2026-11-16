---
id: "02-prove-session-recovery"
title: "Prove SSH recovery after controller exit and host reboot"
status: in_progress
wave: 2
depends_on:
  - "01-handle-confirmed-absence"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-EXECUTOR-001.9
  - AC-EXECUTORS-SSH-EXECUTOR-001.10
  - AC-EXECUTORS-SSH-EXECUTOR-001.18
  - AC-EXECUTORS-SSH-EXECUTOR-001.20
  - AC-EXECUTORS-SSH-EXECUTOR-001.22
  - AC-EXECUTORS-SSH-EXECUTOR-001.23
  - AC-EXECUTORS-SSH-EXECUTOR-001.24
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
  - ../../specs/executors/system-design/ssh-agentctl-reconciliation.md
---

# Task 02: Prove SSH recovery after controller exit and host reboot

## Summary

Prove both corrected branches through real SSH and the existing recovery actions.
Use a synthetic previous-boot receipt for reproducible reboot coverage.
Update the existing SSH explanation after implementation.

## In scope

- Add the [plan's four broker cases](plan.md#e2e-tests) to
  `tests/ssh/recovery.spec.ts` and retain its four existing cases.
- Reuse `ssh-test-base` and the managed lease/transport setup from
  `plugin-git-credentials.spec.ts`.
  Prove that the remote launch received broker URL and lease keys without exposing values.
- Cover `agent.stop` then resume, external SIGTERM then resume,
  a previous-boot receipt then resume, and a previous-boot receipt then `fresh_start`.
- For reboot setup, stop only the fixture-owned controller and backend.
  Wait for process exit. Use `helpers/node-sqlite.ts` to change only the
  boot component of the selected existing launch receipt.
  Scope the update to its expected session and execution and require one matched row.
  Close the database before `backend.restart()`. Do not change the machine's boot ID.
- Assert persisted replacement generation, workspace sentinel, unaffected sibling,
  conversation behavior, and a newly accepted response.
  Resume retains the conversation. Fresh start keeps its existing replacement policy.
- Run the existing `cleanup-reconciliation.spec.ts` cases.
- Update `docs/public/executors.md` with confirmed-absence and reboot recovery,
  refreshed broker credentials, and unknown-evidence refusal.
  Mention that disposable runtime files can remain after already-absent receipt cleanup.
- Restore shared fixture settings and release owned resources in `finally`.

## Out of scope

New broker infrastructure, live provider credentials, actual host reboot,
product test flags, UI markup, mobile layout changes, and new recovery actions.

## Acceptance

1. Stop and external-exit scenarios resume in the same task/session and provider
   conversation. The replacement has a new receipt generation and its own process identity.
   A subsequent prompt completes with a new response marker.
2. Both previous-boot recovery actions complete through the real helper and preflight.
   Resume preserves the provider conversation. Fresh start follows its existing policy.
   Workspace data and the sibling survive. Evidence clearly labels the synthetic receipt.
3. The four existing recovery cases and two cleanup cases pass alongside the new cases.
   Public guidance describes verified reboot recovery, authentication/ownership refusals,
   and possible retained runtime files. A skipped broker case is not acceptance.

## Verification

Run from the repository root. In a fresh worktree, install dependencies once:

```sh
(cd apps && pnpm install --frozen-lockfile)
```

The existing fixture needs a reachable HTTPS broker URL.
`KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL` supports the existing `{port}` placeholder.
The URL must reach the isolated backend from the SSH container.
If the URL is missing, the required expansion stops the command block.

```sh
: "${KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL:?Configure the isolated HTTPS broker fixture}"
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --shards 1 --project containers tests/ssh/recovery.spec.ts tests/ssh/cleanup-reconciliation.spec.ts
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Planned discovery at the investigation base: eight recovery cases and two cleanup cases.
Record actual discovery, pass, failure, and skip counts.
The managed runner rebuilds the backend, helpers, and web assets.
Keep this work order `in_progress` until all required cases run without skips.
Retain Task 01's RED evidence and record this integrated GREEN after its correction.

## Files likely touched

- `apps/web/e2e/tests/ssh/recovery.spec.ts`
- `apps/web/e2e/helpers/ssh-broker-recovery.ts` (only if needed for shared scenario setup)
- `docs/public/executors.md`
- This work order and `plan.md` for results

## Dependencies

Task 01. Docker and the existing HTTPS broker fixture are execution prerequisites.

## Risks

The managed lease fixture proves the shared lifecycle branch, not live GitHub authorization.
A synthetic receipt proves boot classification, not a physical host reboot.
Restore fixture data and leave the machine's real boot identity untouched.

## Parallelism

`sequential`

## Inputs

- [SSH requirements](../../specs/executors/requirements/ssh-executor.md), especially AC-001.9
- [Reconciliation design](../../specs/executors/system-design/ssh-agentctl-reconciliation.md)
- Existing `apps/web/e2e/tests/ssh/recovery.spec.ts`,
  `apps/web/e2e/tests/ssh/cleanup-reconciliation.spec.ts`,
  and `apps/web/e2e/tests/ssh/plugin-git-credentials.spec.ts`
- Existing `helpers/plugin-git-credentials.ts`, `helpers/node-sqlite.ts`,
  `fixtures/ssh-test-base.ts`, and `pages/session-page.ts` under `apps/web/e2e`
- [Public documentation guide](../../public/README.md)

## Results

Partial on 2026-11-16. E2E discovery found all 10 planned cases. The container
suite passed six cases: the four existing recovery tests and both cleanup
reconciliation tests. The four new broker cases were skipped because
`KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL` is not configured. Those skips
do not satisfy acceptance. The public SSH recovery guidance is updated. A
review follow-up corrected all three E2E defects: broker environment assertions
share the sorted production key names, each setup owns and removes a unique Git
fixture directory, and the new reply is located by its API message ID. The
managed suite was rerun after those changes with the same result: 10 discovered,
6 passed, 4 skipped, and 0 failed. Task 02 remains `in_progress` until all four
broker cases pass with the isolated HTTPS fixture.

The backend build, web typecheck, targeted Oxlint, Prettier, public-doc
validation, docs catalog validation, specification lint, and `git diff --check`
passed. No physical host reboot is claimed; reboot coverage uses a synthetic
previous-boot receipt.

The review follow-up also makes process-liveness checks propagate Docker
execution failures and restores the SSH worker's prior system Git settings
after broker-fixture cleanup. The latest managed run passed the liveness error
assertion and again discovered 10 cases: 6 passed, 4 broker cases skipped, and
0 failed. The broker cleanup path remains unexercised locally because the
isolated HTTPS broker URL is not configured. Task 02 remains `in_progress`.
