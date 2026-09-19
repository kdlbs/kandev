---
id: "03-integration-evidence"
title: "Pin, verify, and document guarded TTY execution"
status: pending
wave: 3
depends_on:
  - "01-codex-acp-bridge"
  - "02-kandev-routing-audit"
plan: "plan.md"
requirements:
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-001
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-002
acceptance_criteria:
  - AC-AGENTS-GUARDED-TTY-EXECUTION-001.5
  - AC-AGENTS-GUARDED-TTY-EXECUTION-002.6
  - AC-AGENTS-GUARDED-TTY-EXECUTION-002.7
system_design:
  - ../../specs/agents/system-design/guarded-tty-execution.md
---

# Task 03: Pin, verify, and document guarded TTY execution

## Summary

Pin the reviewed bridge release, prove deployment coexistence and ordinary
non-TTY behavior, then collect the real guarded-agent model and dispatch
receipts. Update internal and public documentation according to actual shipped
visibility.

## In scope

- Exact managed-runtime pin and bridge compatibility documentation.
- Targeted/full backend checks and the credentialed task-owned acceptance run.
- Deployment-local versus upstream conclusion and documentation impact.

## Out of scope

- Publishing the upstream npm package without its repository's release owner.
- Reproducing deployment-only guard source in public Kandev.

## Acceptance

- The guarded agent returns TTY, cwd, repository, completion, and exit-zero
  evidence linked by one attestation ID.
- A denial yields zero dispatch, and a concurrent ordinary command completes
  promptly.
- The Kandev PR states the exact deployment and upstream scope with test
  receipts.

## Verification

```bash
make -C apps/backend test
make -C apps/backend lint
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/agent/agents/managed_npm_runtime_versions.json`
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`
- `apps/backend/internal/agent/agents/codex_acp_test.go`
- `docs/decisions/0034-agentclientprotocol-codex-acp.md`
- `docs/public/**` only when docs review finds a public surface

## Dependencies

- Tasks 01 and 02.

## Risks

- Credentialed E2E availability and upstream release timing.

## Parallelism

`sequential`

## Inputs

- Reviewed upstream release and PR evidence.
- Task 02 audit/result contract.
- Deployment repair `6fcc88f` exact invariants and baseline results.

## Results

Pending.
