---
id: 05-docs-verification
title: Documentation and verification
status: completed
wave: 5
depends_on: [04-operation-rate-errors]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
  - REQ-INTEGRATIONS-GITHUB-RATE-004
system_design: ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 05: Documentation and Verification

## Acceptance

- Workflow Sync recovery and operation-local rate errors are documented.
- Structured transition observability avoids repeated skipped-tick warnings.
- Backend tests/lint, spec/docs validation, and diff hygiene pass.

## Verification

- `make -C apps/backend test`
- `make -C apps/backend lint`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

## Results

Implemented the public Workflow Sync recovery guidance, operation-local rate
error reference, structured classification and recovery telemetry, and
transition-only logging. Focused GitHub and Workflow Sync tests pass.

Passing repository gates:

- `make -C apps/backend lint`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs` (61 tests)
- `node scripts/validate-public-docs.mjs` (41 pages)
- `python3 .github/scripts/lint-harness-files.py AGENTS.md`
- `git diff --check`

`make -C apps/backend test` fails only in four environment-sensitive or
externally-owned packages, none touched by this change. Targeted package
reruns reproduce all four:

- `internal/agent/runtime/agentctl/launcher`: process-group child PID failures
  are observed and reproducible under the guarded runtime.
- `internal/agentctl/server/process`: parent PID and stale process-group
  failures, with `GetParentPID` returning `-1`, are observed and reproducible
  under the guarded runtime.
- `internal/common/config`: proven environmental by an
  `env -u KANDEV_HEALTH_TIMEOUT_MS` control, which passes with the injected
  `KANDEV_HEALTH_TIMEOUT_MS=180000` removed.
- `internal/task/service`: reproduced by three independent tasks on unrelated
  branches. Temp-directory location, sandbox denial of `os.OpenRoot`, and
  parent-chain traversal from `/` were tested and eliminated as causes. The
  externally-owned defect remains under investigation as Kandev Support
  request `c67824e3-6eca-4c57-96b3-7a205885dd83`.

The task does not bypass the guard or edit these unrelated packages. The
coordinator accepted the preserved broad-suite exception as externally owned;
all branch-owned acceptance and verification paths pass.
