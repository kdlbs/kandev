---
id: "03-public-docs"
title: "Document reset preservation"
status: done
wave: 2
depends_on:
  - "01-restore-runtime-configuration"
plan: "plan.md"
spec: "../../specs/ui/requirements/acp-model-configuration-summary.md"
---

# Task 03: Document Reset Preservation

## Acceptance

- The workflow guide states that a context reset preserves the selected ACP model, permission mode, and provider options.
- The guide states that a restoration error blocks the destination step's automatic prompt.
- Public documentation checks pass without navigation or metadata changes.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/tasks-and-workflows.md`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 02 after Task 01. The tasks own disjoint files and no shared configuration.

## Inputs

- Reset behavior in the linked spec
- Plan section `Public documentation`
- The `Configure events and transitions` section in the public workflow guide
- Public documentation style and validation rules

## Output contract

Report the changed section, the page type, exact documentation checks, and blockers. Update this task and `plan.md` in the same conversation.

## Results

Updated the `Configure each step` section in the public how-to guide
`docs/public/tasks-and-workflows.md`. It now states that context reset keeps
the selected ACP model, permission mode, and provider options, restores them
before the next automatic prompt, and blocks the destination step's automatic
prompt when provider restoration fails.

Verification:

- `node --test scripts/validate-public-docs.test.mjs` — 61 passed.
- `node scripts/validate-public-docs.mjs` — validated 41 published docs pages.
