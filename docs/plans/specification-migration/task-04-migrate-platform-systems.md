---
id: "04-migrate-platform-systems"
title: "Migrate platform specifications"
status: completed
wave: 4
depends_on:
  - "03-migrate-integrations-office"
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 04: Migrate platform specifications

## Summary

Organize cross-cutting runtime, configuration, lifecycle, observability,
notification, localization, and security sources under the platform system.
Move standalone platform capabilities to the correct owner when they belong to
the agent runtime, release, desktop, or executor boundary instead.

## In scope

- `docs/specs/platform/**`
- Agent runtime, launcher, recovery, process, credential, configuration, and
  observability root sources assigned to platform.
- Feature toggles and cross-cutting diagnostic sources when platform owns the
  contract.

## Out of scope

- UI-only presentation behavior.
- Product-wide principles and the specification guide.

## Acceptance

- Platform requirements state observable guarantees and acceptance criteria;
  system designs identify real runtime boundaries and ADRs.
- No platform source remains as a standalone `spec.md` or category document.
- Cross-system links use canonical system paths.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans
```

## Files likely touched

- `docs/specs/platform/**`
- `docs/specs/agents/**`
- `docs/specs/platform/**`
- `docs/specs/executors/**`

## Dependencies

Task 03.

## Risks

- Recovery and runtime documents are large and combine user-visible and
  technical behavior. Split by lifecycle boundary and retain both sides.

## Parallelism

`sequential`

## Inputs

- Platform system README.
- `docs/specs/guide/requirements.md`
- `docs/specs/guide/system-design.md`
- Relevant ADRs under `docs/decisions/`.

## Results

Completed. Migrated platform, runtime, launcher, configuration, diagnostics,
notification, localization, and related standalone sources.
