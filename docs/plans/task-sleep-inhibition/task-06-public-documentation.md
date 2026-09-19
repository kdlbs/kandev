---
id: "06-public-documentation"
title: "Host sleep documentation"
status: done
wave: 4
depends_on: ["03-system-api-wiring"]
plan: "plan.md"
spec: "../../specs/platform/requirements/task-sleep-inhibition.md"
---

# Task 06: Host sleep documentation

## Acceptance

- `docs/public/operations.md` explains how to enable the setting, its disabled default and admin/install scope, and the exact `STARTING`/`RUNNING` lifecycle boundary.
- The section distinguishes system sleep from display/explicit sleep and clearly states the backend-host, Linux logind, container/Kubernetes, and remote-executor limitations.
- Public-doc validators pass and the page remains primarily an operations how-to/reference.

## Verification

```bash
rg -n "sleep|Task Actions|Kubernetes|logind" docs/public/operations.md docs/specs/platform/requirements/task-sleep-inhibition.md
```

```bash
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/operations.md`

## Dependencies

Task 03 establishes the final endpoint/status implementation to document.

## Parallelism

Parallel-safe with Task 04 only: it owns a single public-doc file disjoint from frontend implementation. Execution remains sequential unless the user explicitly authorizes subagents.

## Inputs

- Spec in full.
- Plan section: Public documentation.
- Docs-maintainer guidance; `docs/public/operations.md` is the existing deployment/operations owner.

## Risks

- Do not imply that enabling inside a container controls the physical node or that Kandev can override lid-close/user/administrator power policy.

## Output contract

Report the updated section and its Diataxis classification, exact validation results, files changed, blockers/risks, and synchronized task/plan status.

## Results

- Added the operations how-to/reference section describing the admin path, disabled
  default, `STARTING`/`RUNNING` boundary, display and explicit-sleep exclusions,
  macOS/Windows/Linux support, system-service failures, and container/Kubernetes/
  remote-executor limitations.
- Public-doc validation passed: `node --test scripts/validate-public-docs.test.mjs`
  (58 passed) and `node scripts/validate-public-docs.mjs` (`Validated 41 published docs pages.`).
