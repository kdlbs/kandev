---
spec: docs/specs/tasks/multi-branch/spec.md
created: 2026-07-22
status: completed
---

# Implementation Plan: Mark Selected Task Repositories

## Overview

Teach both task-creation repository pickers to distinguish repositories already chosen in another row without removing those options. This preserves intentional same-repository, different-branch tasks while making accidental duplicate selection obvious. Workspace/on-disk and Remote provider pickers can be implemented in parallel because they own separate components and tests.

## Backend

No backend, API, persistence, or validation changes. Existing multi-branch task contracts remain unchanged.

## Frontend

### Workspace and discovered repositories

- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`: derive the repository IDs and normalized local paths selected by rows other than the current row; keep those options available in task creation and render a compact accent-colored check with an accessible `Already added` label beside each matching option. Preserve quick chat's current hide-selected behavior through its explicit duplicate policy.
- `apps/web/components/task-create-dialog-repo-chips.tsx`: remove the stale comment that describes task creation as never filtering only because branch pairs differ, and document the marked-but-selectable behavior.
- Reuse the existing `Pill` option rendering. This changes option content only; the existing popover, responsive behavior, scroll owner, and touch targets remain intact.

### Remote provider repositories

- `apps/web/components/task-create-dialog-remote-repo-chips.tsx`: compute per-row repository identities selected by other Remote rows using provider/id metadata with normalized URL fallback, and pass them to each chip.
- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`: mark matching provider options with the same compact accent-colored check while leaving them selectable. The current row's own repository is not marked unless another row also selects it.

### Mobile design contract

- Desktop outcome and mobile entry point: the existing task-creation dialog and its repository pill triggers continue to expose the same choices; the marker appears inside the same option row at both viewport sizes.
- Nearest shipped exemplar: the existing workspace `Pill` and Remote repository popover option rows remain the interaction and geometry baseline.
- Presentation and hierarchy: no new surface or navigation is introduced. The selector popover remains the single scroll owner, with the repository name primary and a compact check as the secondary duplicate signal.
- Shared behavior: identity derivation and marker state are viewport-independent. Existing mobile option rows retain their 44px touch target and viewport containment.

## Tests

- **What:** another workspace row's repository ID is marked, remains selectable, and the current row does not mark itself. **File:** `apps/web/components/task-create-dialog-repo-chips.test.tsx`. **How:** component tests opening each row's `Pill` options.
- **What:** discovered local paths use normalized identity and update when the other row changes or disappears. **File:** `apps/web/components/task-create-dialog-repo-chips.test.tsx`. **How:** component rerender tests with path-keyed rows.
- **What:** another Remote row's provider repository is marked by provider/id or normalized URL while remaining selectable, and the current row is excluded. **Files:** `apps/web/components/task-create-dialog-remote-repo-chip.test.tsx` and a focused row-level test beside `task-create-dialog-remote-repo-chips.tsx`. **How:** component tests with provider-backed repository fixtures.

## E2E Tests

- **Scenario:** a selected workspace repository is marked in the next selector on a phone viewport and remains selectable. **File:** `apps/web/e2e/tests/task/mobile-create-task-repository-selection.spec.ts`. **What to verify:** open the mobile create-task dialog, select a seeded repository, add a row, open the next picker, and assert the matching option exposes the accessible `Already added` check and can still be chosen.
- **Scenario:** a selected provider-backed Remote repository is marked in the next selector on a phone viewport. **File:** `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`. **What to verify:** select a mocked provider repository, add a Remote row, reopen the picker, assert the matching option exposes the accessible check, remains inside the viewport, and remains selectable.

## Implementation Waves

Wave 1 (parallel):

- [x] [task-01-workspace-repository-marker](task-01-workspace-repository-marker.md) — done
- [x] [task-02-remote-repository-marker](task-02-remote-repository-marker.md) — done

Wave 2:

- [x] [task-03-remote-identity-remediation](task-03-remote-identity-remediation.md) — done
- [x] [task-04-workspace-coverage-remediation](task-04-workspace-coverage-remediation.md) — done

Wave 3:

- [x] [task-05-remote-identity-review-fix](task-05-remote-identity-review-fix.md) — done

Wave 4:

- [x] [task-06-github-case-normalization](task-06-github-case-normalization.md) — done

Wave 5:

- [x] [task-07-lint-cleanup](task-07-lint-cleanup.md) — done

Wave 6:

- [x] [task-08-remote-popover-lint](task-08-remote-popover-lint.md) — done

Wave 7:

- QA integration and mobile behavior — QA worker, balanced model
- Code review — code-review worker, frontier model
- Full format/typecheck/test/lint verification — verify worker, cheap model

Wave 8:

- [x] [task-09-compact-selected-marker](task-09-compact-selected-marker.md) — done

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-repo-chips.test.tsx`
- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx`
- `cd apps && pnpm --filter @kandev/web e2e -- e2e/tests/task/mobile-create-task-repository-selection.spec.ts --project=mobile-chrome`
- `cd apps && pnpm --filter @kandev/web e2e -- e2e/tests/task/mobile-create-task-remote-repo.spec.ts --project=mobile-chrome`
- `make fmt`
- `make typecheck test lint`

## Risks

- Remote rows can originate from provider picks or pasted URLs; identity comparison must normalize URLs without conflating different providers or repositories.
- Supported HTTPS, SSH, and provider subresource URLs must canonicalize to the same repository identity as provider picker options.
- Marker state must exclude only the current row, or reopening a single selected row will incorrectly label its own value as a duplicate.
- Quick chat deliberately hides repositories already selected in another context row; the task-dialog marker must not regress that separate behavior.
