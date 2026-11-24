---
id: "06-acceptance-and-docs"
title: "End-to-end acceptance and operator documentation"
status: done
wave: 6
depends_on:
  - "05-passthrough"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WRAPPERS-001
  - REQ-AGENTS-WRAPPERS-002
  - REQ-AGENTS-WRAPPERS-003
  - REQ-AGENTS-WRAPPERS-004
  - REQ-AGENTS-WRAPPERS-005
acceptance_criteria:
  - AC-AGENTS-WRAPPERS-001.1
  - AC-AGENTS-WRAPPERS-001.2
  - AC-AGENTS-WRAPPERS-001.4
  - AC-AGENTS-WRAPPERS-002.1
  - AC-AGENTS-WRAPPERS-002.4
  - AC-AGENTS-WRAPPERS-003.1
  - AC-AGENTS-WRAPPERS-003.4
  - AC-AGENTS-WRAPPERS-003.5
  - AC-AGENTS-WRAPPERS-004.1
  - AC-AGENTS-WRAPPERS-004.2
  - AC-AGENTS-WRAPPERS-004.5
  - AC-AGENTS-WRAPPERS-005.1
  - AC-AGENTS-WRAPPERS-005.2
  - AC-AGENTS-WRAPPERS-005.3
system_design:
  - ../../specs/agents/system-design/generic-launch-wrappers.md
---

# Task 06: End-to-end acceptance and operator documentation

## Summary

Prove the complete operator flow in isolated desktop and phone tests. Record fresh Pi/Greywall evidence without the diagnostic exception and document supported behavior.

## In scope

- Extend profile discovery browser tests with an unrelated subprocess wrapper and real API requests. Cover draft refresh, dependent options, retained selection, missing wrapper, and corrected retry.
- Add desktop and mobile passthrough wrapper tests with actual invocation receipts, resume, input, and cancellation. Also verify ordinary ACP chat across a backend restart.
- Add a reproducible opt-in real-provider harness and acceptance record. Use dynamically allocated paths and explicit private credentials. Preserve the retained manual instance.
- Update command-prefix reference and passthrough limits in `docs/public/agents-and-profiles.md`. Cover reference escaping, executor-local paths, unsupported native utility transports, auth-disabled limits, and no plugin requirement.
- Reconcile every changed work order with actual results. Promote the draft specification pair only when implementation and acceptance satisfy the contract.

## Out of scope

No changes to the deprecated #3812 task or manual instance. No real-provider prompts in ordinary CI. No claim of confinement or untested platform support.

## Acceptance

1. Desktop and phone tests prove the wrapper ran and user state survives refresh failures. Passthrough receipts prove initial and resumed execution.
2. Fresh Pi/Greywall discovery, prompt/tools, ACP chat, and passthrough succeed without an overlay, or the exact acceptance blocker remains recorded and the task stays incomplete.
3. Public docs describe the implemented matrix and limitations. All previous work-order checks have recorded results, and PR #4538 remains draft and unmerged.

## Verification

Run from the repository root. Implement new behavior with failing tests first.
The test and harness paths below are implemented.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/profile-capability-discovery.spec.ts tests/cli-mode/generic-launch-wrappers.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-profile-capability-discovery.spec.ts tests/cli-mode/mobile-generic-launch-wrappers.spec.ts)
python3 scripts/check-generic-wrapper-real.test.py
python3 scripts/check-generic-wrapper-real.py --check-prerequisites
python3 scripts/check-generic-wrapper-real.py --run --record docs/plans/generic-launch-wrappers/acceptance.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths from this work order when Go files change.
Record the expanded command and result. Do not replace targeted checks with an unrecorded broad suite.

## Files likely touched

- `apps/web/e2e/helpers/profile-capability-discovery.ts` and isolated fixture helpers.
- `apps/web/e2e/tests/settings/profile-capability-discovery.spec.ts` and `mobile-profile-capability-discovery.spec.ts`.
- `apps/web/e2e/tests/cli-mode/generic-launch-wrappers.spec.ts` and `mobile-generic-launch-wrappers.spec.ts` (new).
- `scripts/check-generic-wrapper-real.py` and its focused harness tests (new, opt-in).
- `docs/plans/generic-launch-wrappers/acceptance.md` (new) and this package's result sections.
- `docs/public/agents-and-profiles.md`.
- The paired generic-wrapper requirement/design frontmatter after verified completion.

## Dependencies

[Task 05](task-05-passthrough.md). Read its recorded results before implementation.

## Risks

Real provider access and wrapper platform support are external prerequisites. Missing prerequisites do not count as a pass. The runner must never target or stop the retained manual instance.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/generic-launch-wrappers.md).
- [System design](../../specs/agents/system-design/generic-launch-wrappers.md), especially the sections that match this work order.
- [Plan](plan.md), compatibility matrix and test mapping.
- [Historical reproduction](reproduction.md), which is diagnostic evidence only.
- Scoped backend/agentctl guidance and nearby existing tests. For browser work, read web guidance and the e2e and mobile-parity skills.

## Results

Completed on 2026-10-19. All 11 selected desktop and phone browser tests pass. Focused backend and race checks, web typecheck, changed-file lint, and document validators pass. Broader baseline failures remain listed in [acceptance.md](acceptance.md).

The opt-in runner and its 12 credential-free safety tests are implemented. A fresh isolated OpenAI Codex login completed one full Pi/Greywall run: direct control, authenticated wrapped discovery, utility prompt/tools, ACP chat, and native passthrough. Discovery returned 9 models and 6 modes. Each model case completed read/bash/write/read with independently verified output. Each wrapped operation produced one receipt. No overlay or provider exception was used.

The runner cleaned its resources. The disposable source login was removed separately. The retained manual instance and deprecated task are unchanged. The requirements are active, the design is current, and the PR remains draft. Exact versions, commands, policy, and limits appear in [acceptance.md](acceptance.md).


On 2026-10-20, CI exposed a workflow preview test that read an aborted response body.
The test now observes completed successful requests before it reads the preview result.
All three desktop cases and the existing phone drawer case pass without retries.
No product behavior, requirement, or design contract changed. Commands, trace evidence,
and host setup limits appear in [acceptance.md](acceptance.md).


On 2026-10-21, all remote CI checks passed at `e4fcd18e9ee`; the four preview cases passed without retries.
The complete browser run contained 14 flaky verdicts, with no unexpected final result.
A later base update conflicted with the preview helper. The merge preserves completed-request observation,
destination-step matching, session readiness, and all existing assertions.
After integration, all eight selected desktop cases and seven phone cases pass without retries.
Focused runtime race checks, all 66 changed Go paths, runner safety checks, and document checks pass.
The expanded native-resume selection exceeded its package time budget during the SQLite state matrix.
The narrower lifecycle check passed. Exact commands and limits appear in [acceptance.md](acceptance.md).

## Real-provider harness contract

The harness accepts private executable paths and credential locations through environment variables.
`--check-prerequisites` reports missing tools, credentials, or unsupported platform without starting an instance or sending a prompt.
`--run` allocates its own temporary home, database, ports, workspace, and process ownership record.
It builds the current source without a Go overlay. It never reads the retained manual instance database or its authentication copy.
Explicit operator policy grants only the required installation, state, cache, working-directory, and managed npm paths.
The configured prefix uses runtime references rather than resolved test constants.

The harness proves a direct control, wrapped discovery without a prompt, a read/execute/write/read provider round trip, ACP chat, and native passthrough.
Use a unique nonce and wrapper receipt per scenario. Check the output file independently and record exact tool/runtime versions.
A simple recording launcher may execute the real sandbox command, but every positive case must prove both layers ran.
Keep raw traces and credentials private. Commit only sanitized counts, statuses, versions, platform, and limitations.
Stop only processes created by the runner or identified by its private ownership nonce. The on-disk ownership record is evidence, not authority to terminate a process. Clean its credentials and operation-owned resources on success, failure, and interruption.
Add `scripts/check-generic-wrapper-real.test.py` with credential-free tests for prerequisite refusal, ownership isolation, and sanitization.
Run `python3 scripts/check-generic-wrapper-real.test.py` before either harness command.
Preserve `reproduction.md` as historical evidence. Record permanent acceptance separately.

### PR review remediation

During each cleanup phase, rescan nonce-owned processes and verify PID birth identities before signaling. Verify children created during TERM are also removed, while unrelated processes remain live.
