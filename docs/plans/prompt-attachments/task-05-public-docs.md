---
id: "05-public-docs"
title: "Prompt attachment documentation"
status: completed
wave: 4
depends_on: ["03-frontend-staged-uploads"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prompt-attachments.md"
---

# Task 05: Prompt attachment documentation

## Acceptance

1. The task/workflow how-to documents ten attachments, 100 MiB per-file and
   aggregate raw limits, staged upload states, retry/expiry behavior, and the
   separate task-document limit without preserving the obsolete base64 mismatch.
2. WebSocket reference keeps the 32 MiB frame contract and distinguishes
   descriptor-only current clients from bounded legacy inline attachment data;
   the internal live-updates spec no longer attributes the transport ceiling to
   current base64 uploads.
3. Public-doc link/structure validators pass, and the final report classifies
   `tasks-and-workflows.md` as a how-to guide and `websocket-api.md` as reference.

## Verification

```bash
rg -n "10 MB|10 MiB|20 MB|20 MiB|32 MiB|100 MiB|attachment" docs/public/tasks-and-workflows.md docs/public/websocket-api.md docs/specs/office/live-updates.md docs/specs/tasks/requirements/prompt-attachments.md docs/decisions/2026-08-04-file-backed-prompt-attachments.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/tasks-and-workflows.md`
- `docs/public/websocket-api.md`
- `docs/specs/office/live-updates.md`
- `docs/specs/tasks/requirements/prompt-attachments.md` only if implementation reveals a
  confirmed contract correction

## Dependencies

- Task 03 confirms the final user-visible state names, response behavior, and
  compatibility boundary.

## Parallelism

Parallel-safe with task 04 after task 03. This task owns documentation only.

## Inputs

- Prompt attachment spec and ADR
- Plan: Public documentation
- Tasks 01-03 final API, UI, and compatibility results

## Output contract

Report public/internal docs changed, Diataxis classifications, exact validator
results, links checked, blockers, risks, and synchronized task/plan status.

## Results

Updated the public task/workflow and WebSocket references plus the internal live-updates spec to document staged descriptors, 100 MiB file/aggregate limits, ten-file count, retry/expiry behavior, and the unchanged 32 MiB socket boundary. Public-doc validators passed: 58 tests and 41 published pages validated.
