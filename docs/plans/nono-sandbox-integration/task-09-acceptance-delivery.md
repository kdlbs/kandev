---
id: "09-acceptance-delivery"
title: "Prove real Nono acceptance and prepare the follow-up PR"
status: done
wave: 10
depends_on:
  - "02-bootstrap-recovery"
  - "08-operator-policy-learning"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-001
  - REQ-AGENTS-SANDBOX-003
  - REQ-AGENTS-SANDBOX-004
  - REQ-AGENTS-SANDBOX-005
  - REQ-AGENTS-SANDBOX-006
  - REQ-AGENTS-SANDBOX-008
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005
  - REQ-PLUGINS-SANDBOX-001
acceptance_criteria:
  - AC-AGENTS-SANDBOX-001.1
  - AC-AGENTS-SANDBOX-001.2
  - AC-AGENTS-SANDBOX-001.3
  - AC-AGENTS-SANDBOX-001.4
  - AC-AGENTS-SANDBOX-003.1
  - AC-AGENTS-SANDBOX-003.2
  - AC-AGENTS-SANDBOX-003.3
  - AC-AGENTS-SANDBOX-003.4
  - AC-AGENTS-SANDBOX-003.5
  - AC-AGENTS-SANDBOX-003.6
  - AC-AGENTS-SANDBOX-004.1
  - AC-AGENTS-SANDBOX-004.2
  - AC-AGENTS-SANDBOX-004.3
  - AC-AGENTS-SANDBOX-004.4
  - AC-AGENTS-SANDBOX-004.5
  - AC-AGENTS-SANDBOX-005.1
  - AC-AGENTS-SANDBOX-005.2
  - AC-AGENTS-SANDBOX-005.3
  - AC-AGENTS-SANDBOX-005.4
  - AC-AGENTS-SANDBOX-006.1
  - AC-AGENTS-SANDBOX-006.2
  - AC-AGENTS-SANDBOX-006.3
  - AC-AGENTS-SANDBOX-006.4
  - AC-AGENTS-SANDBOX-008.2
  - AC-AGENTS-SANDBOX-008.5
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.5
  - AC-PLUGINS-SANDBOX-001.1
  - AC-PLUGINS-SANDBOX-001.2
  - AC-PLUGINS-SANDBOX-001.3
  - AC-PLUGINS-SANDBOX-001.4
  - AC-PLUGINS-SANDBOX-001.5
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
  - ../../specs/tasks/system-design/bootstrap-attempt-recovery.md
  - ../../specs/plugins/system-design/sandbox-provider-capability.md
---

# Task 09: Prove real Nono acceptance and prepare the follow-up PR

## Summary

Run complete real-provider acceptance and document only proven support before the new draft PR.

## In scope

Use disposable authenticated test data, a packaged plugin, pinned Nono, fresh current-source binaries, explicit network policy, and negative enforcement controls.
Verify that real runtime evidence drives active status for the exact attempt and policy.
Document the protection scope and unsupported evidence shapes. A seeded mock reply cannot close this gate.

## Out of scope

No retained-instance mutation, replacement-run recovery claim, merge, parent-branch commit, or parent PR modification.

## Acceptance

- Discovery, ACP tool work, terminal input/tools, same-session restart, cancellation, cleanup, approvals, reports, learning, and MCP pass with positive receipts.
- Denied read/write and network controls fail while permitted work succeeds. Policy edits by agents, callback forgery, and credential disclosure checks fail safely.
- Delivery evidence identifies exact commits, plugin/runtime hashes, platform, commands, limitations, and the verified main base with #4538 as its merged prerequisite.
  The new draft PR targets `main`. Do not reopen or modify #4538.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
python3 scripts/check-nono-real.test.py
python3 scripts/check-nono-real.py --run --record docs/plans/nono-sandbox-integration/acceptance.md
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `docs/plans/nono-sandbox-integration/acceptance.md (new)`
- `scripts/check-nono-real.py`
- `docs/public/agents-and-profiles.md`
- `docs/public/executors.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/plugins-authoring.md`

## Dependencies

02-bootstrap-recovery, 08-operator-policy-learning

## Risks

Missing disposable login, unsupported kernel, or missing platform evidence blocks that acceptance item. Do not mark it passed or replace it with mocks.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).
- [Design](../../specs/tasks/system-design/bootstrap-attempt-recovery.md).
- [Design](../../specs/plugins/system-design/sandbox-provider-capability.md).

## Results

Implementation is authorized. The completed evidence and remaining delivery actions are recorded below.


## Current acceptance run (2026-10-26)

The primary session is running a fresh authenticated verification instance on loopback with source commit `ae48d78209253e0529260727a430c2fd566de788`, the dedicated packaged provider, pinned Pi ACP 0.0.34, Pi CLI 1.3.0, and Node 24.19.0. It uses a copy of this task's stopped test credential file, not the main or older retained instance login. Main Tailscale :9998 and the stopped user-test endpoint are preserved. A temporary harness copy initially dereferenced CLI entry symlinks; restoring only the owned copies made the direct version check pass. That setup fault is not product or sandbox evidence. Actual profile-scoped probes and managed-agent journeys remain in progress.


## Managed acceptance results (2026-10-27)

The final tracked `--run` driver passed all six managed transport checks with actual Pi ACP 0.0.34, Pi CLI 1.3.0, Node 24.19.0, and the packaged Nono 0.80.0 provider. It uses an explicitly selected, process-owned disposable fixture and an explicit test login. It refuses port 9998, unmarked fixtures, recycled owner identity, foreign paths, and missing prerequisites. It creates and cleans only its own sessions. The driver does not discover credentials or select an existing instance automatically.

The run covers ACP tools/MCP, native input/MCP and 120x40 plus 88x28 PTY sizes, paired permitted/denied filesystem and network work, active cancellation with detached-child cleanup, frozen same-session restart with a rotated MCP endpoint, and session-scoped state deletion that preserves the sibling and workspace. The stop acknowledgement is asynchronous. Assertions wait for the captured attempt's stopped state before checking process exit. A real WebSocket regression covers gateway frames coalesced with the requested response.

Operator approval, explicit learning, and reviewed adoption have separate actual managed receipts. They are not mislabeled as automated transport checks. Workspace eligibility has SQLite/PostgreSQL race and desktop/phone coverage. The final packaged-provider regression passes approval, MCP, state preservation, negative controls, and deletion.

The acceptance record stores runtime and source identifiers plus exact tested host/agentctl binary hashes. Its host base commit precedes the workspace-eligibility slice compiled into those binaries. The source slice and tests are delivered with this work order. No clean-head build claim is inferred from that base identifier.

Current-policy attestation remains unsupported. Actual status stays unknown, consistent with Task 01 and SANDBOX-008.2. Linux amd64 Local/Worktree is the tested platform. macOS, Windows, remote executors, non-file approvals, and a live Nono active-shield claim remain unsupported. No replacement sales run is recovery evidence.

Final runner checks: five entry tests (one explicit-runtime test skipped in the credential-free run), six instance-safety tests, one coalesced-WebSocket regression, ten real conformance controls, and six real managed transport checks passed. The real conformance command ran separately with its pinned runtime, so its ten results are not inferred from the skipped unit case.

To repeat managed transport acceptance, set `NONO_ACCEPTANCE_BINARY` to the pinned executable, `NONO_ACCEPTANCE_SEED_MANIFEST` to an explicit disposable acceptance-fixture manifest, and `NONO_ACCEPTANCE_PI_AUTH` to that fixture's explicit test login file. Run `python3 scripts/check-nono-real.py --run --record <private-or-reviewed-record>`. Do not point it at the main instance or a retained user environment. Raw responses remain below the private fixture root.

Draft host PR #4769 and dedicated provider PR #2 remain open for review. Eight desktop/phone captures were uploaded and verified in host PR comment `6431064019`. The owned verification instance was stopped after private evidence archival. Main Tailscale :9998 remains listening, and the user-test endpoint :57397 remains stopped. The implementation is delivered on the task feature branches. No PR was merged.
