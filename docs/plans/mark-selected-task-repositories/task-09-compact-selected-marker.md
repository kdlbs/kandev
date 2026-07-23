---
spec: docs/specs/tasks/multi-branch/spec.md
created: 2026-07-23
status: completed
---

# Task 09: Compact selected-repository marker

## Objective

Replace the space-consuming visible `Already added` badge with one compact accent-colored check in workspace/on-disk and Remote repository picker options, retaining an accessible `Already added` label.

## Acceptance

- A duplicate workspace, on-disk, or Remote option renders one accent-colored check rather than visible `Already added` text.
- The check is exposed to assistive technology as `Already added`; the option remains selectable for same-repository, different-branch tasks.
- The existing picker composition, scroll owner, and 44px mobile option-row target remain unchanged.

## Verification

- Focused web component tests for workspace and Remote picker options.
- Mobile task-creation E2E coverage for both picker variants.
- Fresh desktop and mobile screenshots of the marker state for PR #1881.
