---
id: "04-document-recovery"
title: "Document executor-local recovery"
status: done
wave: 4
depends_on:
  - "03-container-recovery-evidence"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 04: Document executor-local recovery

## Summary

Update public and bridge documentation after executor recovery passes. Explain
the supported locations and the limits that protect user npm configuration.

## In scope

- Update the public agent runtime recovery section.
- Update bridge version documentation when its recovery boundary changes.
- Keep recovery limits visible and task-oriented.

## Out of scope

- General npm troubleshooting.
- Manual cache cleanup instructions.
- New public pages or navigation changes.

## Acceptance

- Public documentation names local PC, local Docker, and remote SSH support.
- Documentation states that Kandev preserves the registry and exact version.
- Documentation states that Kandev never clears the global npm cache.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public apps/backend/internal/agent/agents
```

## Files likely touched

- `docs/public/agents-and-profiles.md`
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`

## Dependencies

- Task 03 proves the documented executor behavior.

## Risks

- Documentation can imply automatic rollback if it does not name the same exact version.

## Parallelism

`sequential`

## Inputs

- Managed npm runtime recovery requirement and system design.
- Public agent and profile documentation.
- Existing bridge version documentation.

## Results

- Updated `docs/public/agents-and-profiles.md` with local PC, local Docker, and
  remote SSH recovery behavior and the executor-local cache boundary.
- Updated `ACP_BRIDGE_VERSIONS.md` with the same exact-version, one-retry, and
  no-global-cleanup limits.
- `node --test scripts/validate-public-docs.test.mjs` passed all 61 tests.
- `node scripts/validate-public-docs.mjs`, `python3 scripts/lint-spec-files.py
  --all`, and the requested diff checks passed.
