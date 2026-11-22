---
id: "09-registry-entry"
title: "Registry entry and public docs"
status: pending
wave: 5
depends_on:
  - "08-core-removal"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-AZURE-DEVOPS-EXTRACTION-001
acceptance_criteria:
  - AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.5
system_design:
  - ../../specs/plugins/system-design/azure-devops-extraction.md
---

# Task 09: Registry entry and public docs

## Summary

Add `kandev-plugin-azure-devops` to `plugin-registry/plugins.yaml` after release `v0.1.0`, and complete any public docs pointer not already in K-A.

## In scope

- Registry entry with `repo: JnManso/kandev-plugin-azure-devops`, `categories: [integrations]`.
- `docs/public/integrations.md` and `feature-status.md` pointer if K-A did not include it.

## Out of scope

- Plugin code.

## Acceptance

- `plugin-registry` tests pass and the index workflow resolves the release.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd plugin-registry && node --test)
```

## Files likely touched

- `plugin-registry/plugins.yaml`
- `docs/public/integrations.md`

## Dependencies

Task 08-core-removal, and the plugin `v0.1.0` release (plugin work order 07, owned by the plugin repository)

## Risks

- Listing needs maintainer review.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/plugins/system-design/azure-devops-extraction.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

Pending.
