---
id: "03-document-and-prove-upgrade-behavior"
title: "Document and prove upgrade behavior"
status: done
wave: 3
depends_on:
  - "02-activate-defaults-at-startup"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-002.3
  - AC-AGENTS-RUNTIME-UPDATES-002.4
  - AC-AGENTS-RUNTIME-UPDATES-002.5
system_design:
  - ../../specs/agents/system-design/runtime-default-activation.md
---

# Task 03: Document and Prove Upgrade Behavior

## Summary

Update the operator documentation for default activation and later version selection. Run the existing Settings rollback flow as end-to-end evidence.

## In scope

- Explain when Kandev replaces an operator selection.
- Explain which processes keep their current runtime.
- Explain how to select an older stable version after startup.
- Synchronize internal default-version documentation with the embedded catalog.
- Run the focused desktop Settings rollback scenario.

## Out of scope

- New UI copy, controls, screenshots, or mobile layout changes.
- A new browser fixture for compile-time default catalogs.
- General agent-runtime documentation rewrites.

## Acceptance

- Public documentation states the default-generation upgrade rule and downgrade path.
- Internal version tables match the embedded managed-runtime catalog.
- The existing browser rollback flow passes against the production build.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
pnpm e2e:run tests/settings/agent-runtime-update.spec.ts -- --grep "selects an older stable version for rollback"
git diff --check -- docs/public docs/specs docs/decisions docs/plans apps/backend/internal/agent/agents
```

Run the first two commands from the repository root. Run the Playwright command from `apps/web`.

## Files likely touched

- `docs/public/agents-and-profiles.md`
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`
- `docs/decisions/0034-agentclientprotocol-codex-acp.md`

## Dependencies

Task 02.

## Risks

- Documentation can imply that every Kandev release resets selections. The rule applies only when that agent's package or default changes.
- The browser test proves the existing downgrade action. Backend tests must prove upgrade-time default activation.

## Parallelism

`sequential`

## Inputs

- Completed backend behavior from Tasks 01 and 02.
- `docs/public/agents-and-profiles.md` as the operator how-to guide.
- Existing desktop rollback scenario in `agent-runtime-update.spec.ts`.

## Results

Updated the public managed-agent guide with the default-generation upgrade
rule, active-process behavior, and post-startup rollback path. Synchronized
the internal ACP bridge version table and Codex decision with the embedded
catalogue, and recorded the startup rule in the durable decision/spec files.

Verification: specification lint passed; public documentation tests passed
(61 tests) and the validator passed (46 published pages); the focused rollback
E2E passed (1 test); and the scoped `git diff --check` passed.
