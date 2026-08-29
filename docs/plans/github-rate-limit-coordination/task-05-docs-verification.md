---
id: 05-docs-verification
title: Documentation and verification
status: completed
wave: 5
depends_on: [04-agent-rate-snapshot]
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

- Workflow Sync recovery and the agent snapshot contract are documented.
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

Implemented the public Workflow Sync recovery guidance, the zero-call agent
snapshot reference, structured classification/deferral/suspension/recovery
telemetry, and transition-only logging. Focused GitHub and Workflow Sync tests,
including their race suites, pass.

Passing repository gates:

- `make -C apps/backend lint`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs` (61 tests)
- `node scripts/validate-public-docs.mjs` (41 pages)
- `python3 .github/scripts/lint-harness-files.py AGENTS.md`
- `git diff --check`

`make -C apps/backend test` remains blocked by deterministic runtime failures
outside this plan's packages. Focused reruns reproduce process-parent and
process-group failures in `internal/agent/runtime/agentctl/launcher` and
`internal/agentctl/server/process`; the current Kandev process exports
`KANDEV_HEALTH_TIMEOUT_MS=180000`, which overrides the fixture expected by
`internal/common/config`; and the guarded filesystem rejects the parent-chain
inspection exercised by local-directory tests in `internal/task/service`.
The task does not bypass that guard or modify these unrelated packages.

The guard-sensitive filesystem result was independently reproduced on three
unrelated branches. Re-running the seven local-repository tests with `TMPDIR`
inside this task's writable worktree produces the same failures because the
shared parent-chain helper opens filesystem root `/`. The coordinator accepted
the preserved broad-suite exception as externally owned; all branch-owned
acceptance and verification paths pass.
