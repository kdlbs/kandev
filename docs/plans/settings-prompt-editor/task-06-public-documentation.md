---
id: "06-public-documentation"
title: "Update public documentation"
status: done
wave: 4
depends_on: ["02-quick-actions", "03-watch-prompts", "04-core-settings-prompts"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-prompt-editor.md"
---

# Task 06: Update public documentation

## Acceptance

- Integration documentation explains placeholder and saved-prompt completion for quick actions and watch prompts.
- Workflow and developer-tool documentation explain the shared completion behavior and utility-agent exception.
- Public documentation validation passes without a new navigation entry.

## Verification

```bash
rg -n "quick actions|saved prompt|workflow prompt|utility" docs/public/integrations.md docs/public/workflow-tips.md docs/public/developer-tools.md && node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/integrations.md`
- `docs/public/workflow-tips.md`
- `docs/public/developer-tools.md`

## Dependencies

Tasks 02, 03, and 04.

## Parallelism

Parallel-safe with Task 05 after Tasks 02 through 04. This task owns only public documentation files.

## Inputs

- Spec: `What`, `Settings surfaces`, and `Out of scope`.
- Plan: `Public documentation`.
- Current docs: existing saved-prompt, workflow, watcher, quick-action, and utility sections.

## Output contract

Report pages changed, each page's primary Diátaxis type, exact command results, link impact, blockers, risks, and synchronized task and plan status.

## Results

- Updated `docs/public/integrations.md`, `docs/public/workflow-tips.md`, and `docs/public/developer-tools.md`.
- The pages explain placeholder and saved-prompt completion, route-level save/reset behavior, nested references, current-prompt exclusion, and the utility-agent exception.
- `node --test scripts/validate-public-docs.test.mjs` passes: 61 tests.
- `node scripts/validate-public-docs.mjs` passes: 41 published pages validated.
