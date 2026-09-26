---
id: "02-explicit-url-entry"
title: "Explicit remote URL entry"
status: done
wave: 2
depends_on: ["01-public-provider-resolution"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/multi-branch.md"
---

# Task 02: Explicit remote URL entry

## Acceptance

- Paste, blur, and Tab leave a supported URL editable and do not call `onURLChange`.
- Plain Enter commits the trimmed supported URL exactly once.
- URL-shaped input displays a visible `Remote URL` hint that explains Enter submits it.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-chip-url.test.tsx`

## Files likely touched

- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chip-url.test.tsx`

## Dependencies

- Task 01 establishes the public resolution behavior started after Enter.

## Inputs

- `docs/specs/tasks/requirements/multi-branch.md` — explicit-submit scenarios.
- Existing Remote popover input and mobile geometry.

## Output contract

Report the changed interaction, exact tests/results, risk tags, and uncertainties; update this task to `done` only after targeted verification passes.

## Completion

- Implemented staged remote-URL input: paste, blur, and Tab preserve editable text; only plain Enter commits a trimmed supported URL.
- Added a visible `Remote URL` hint explaining Enter submission and retained picker-option immediate selection plus unsupported-provider validation.
- Verified with `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-chip-url.test.tsx` (8 passing tests).
