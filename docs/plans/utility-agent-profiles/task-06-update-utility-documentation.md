---
id: "06-update-utility-documentation"
title: "Update utility profile documentation"
status: completed
wave: 4
depends_on: ["03-update-utility-consumers", "04-settings-profile-pickers"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 06: Update utility profile documentation

## Intent

Update public how-to/explanation and feature inventory text so users and plugin authors understand
profile-backed utility execution, eligibility, permissions, and recovery.

## Acceptance

- Developer/review docs explain selecting a default profile and overrides, profile-owned launch and
  permission behavior, and how to repair missing/disabled/unconfigured profiles.
- Plugin docs preserve the plugin-facing utility-agent ID contract while explaining that the
  selected utility agent resolves its own profile; no docs instruct users to choose a utility-only
  agent/model pair.
- Public-doc validation and a repository terminology search pass without broken links or stale
  user-facing guidance.

## Files likely touched

- `docs/public/developer-tools.md`
- `docs/public/sessions-and-review.md`
- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md`
- `docs/features.md`

## Dependencies

Tasks 03 and 04 so documented behavior and labels match the implemented contract.

## Parallelism

Parallel-safe with task 05 after product behavior is complete: documentation and E2E files are
disjoint. Sequential execution remains the default.

## Inputs

- Spec: complete observable contract and failure modes.
- ADR: `ADR-2026-08-08-utility-agent-profile-execution`.
- Plan: `Public documentation`.
- Existing docs types: `developer-tools.md` is primarily how-to/explanation;
  `plugins-manifest.md` is reference; `sessions-and-review.md` is task-oriented how-to.

## Verification

```bash
rg -n "Default utility agent model|default model|agent and model|utility agent" docs/public docs/features.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Output contract

Report updated public pages and their primary content types, terminology changes, exact validation
results, files changed, blockers, risks, and synchronized task/plan status. Do not add a new public
page or edit `docs/public/meta.json` unless implementation reveals a genuinely missing navigation
destination.

## Results

Updated developer, review, plugin authoring, plugin manifest, and feature inventory documentation. Public-doc validation passed.
