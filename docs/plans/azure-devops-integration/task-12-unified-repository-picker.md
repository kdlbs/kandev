---
id: "12-unified-repository-picker"
title: "Unified task repository picker"
status: done
wave: 6
depends_on: ["10-remote-repository-contracts", "11-secure-azure-clone"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 12: Unified Task Repository Picker

## Acceptance

- The Remote picker groups and searches repositories from configured GitHub, GitLab, and Azure DevOps providers.
- Each result shows its provider identity and selects its canonical remote URL and default branch.
- Manual supported HTTPS/SSH URLs remain available with provider-aware validation.
- Loading, empty, unavailable, partial-provider failure, retry, and long-list states are complete.
- Desktop and mobile layouts preserve the dialog's stable dimensions and primary actions.

## Verification

- Component tests for grouping, search, selection, partial failures, manual URLs, and branch dispatch.
- Desktop and mobile task-create Playwright flows for all three providers.
- Web typecheck and lint.

## Files Likely Touched

- `apps/web/components/task-create-dialog-remote-repo-*.tsx`
- `apps/web/components/task-create-dialog-types.ts`
- Provider-neutral discovery/branch hooks and API types.
- Task-create E2E fixtures and tests.

## Output Contract

Report picker behavior, provider/error coverage, RED/GREEN commands, mobile screenshots or assertions, and update this task plus its plan checkbox.
