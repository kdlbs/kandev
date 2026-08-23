---
id: "07-runtime-version-docs"
title: "Update runtime version documentation"
status: complete
wave: 5
depends_on: ["01-default-pins", "02-effective-version-selection", "03-update-status-api", "05-settings-update-indicator"]
plan: "plan.md"
spec: "../../specs/agents/runtime-updates.md"
---

# Task 07: Update runtime version documentation

## Acceptance

- Public agent documentation explains Kandev defaults, operator selections,
  the blue update hint, return-to-default behavior, and active-session effects.
- Internal bridge documentation shows exact effective-version commands and
  distinguishes offline-preferred launch from online-preferred update behavior.
- A focused search finds no live documentation claiming managed packages are
  unversioned or that selections are host-only.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs && git diff --check
```

## Files likely touched

- `docs/public/agents-and-profiles.md`
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`
- Other directly affected public/spec/decision references found by focused `rg`

## Dependencies

Tasks 01, 02, 03, and 05.

## Parallelism

Parallel-safe with Task 06 after Tasks 01-05. This task owns documentation only.

## Inputs

- Updated runtime-updates spec and ADR
- Final API labels and command behavior from Tasks 01-05
- `/docs-maintainer` public documentation boundary

## Output contract

Report public/internal docs changed, content type, focused stale-claim search,
exact validation results, risks, and synchronized task/plan status.

## Results

Complete. Public and internal runtime documentation now explains reviewed
defaults, install-wide selections, return-to-default, blue update hints,
unknown checks, active-session behavior, exact effective-version commands, and
offline-preferred launches versus online-preferred updates. The current Codex
decision was reconciled, and the superseded unversioned/host-only claims remain
only as historical context in superseded records.

Verification: public-doc tests passed 61/61 and the validator accepted 41
published pages; `git diff --check` passed; focused stale-claim search found no
current public or bridge documentation making the old claim.
