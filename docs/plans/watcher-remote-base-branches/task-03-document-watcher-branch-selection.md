---
id: "03-document-watcher-branch-selection"
title: "Document watcher branch selection"
status: done
wave: 3
depends_on:
  - "02-prove-watcher-branch-selection"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001
acceptance_criteria:
  - AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.1
  - AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.2
  - AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.3
system_design:
  - ../../specs/integrations/system-design/watcher-remote-base-branches.md
---

# Task 03: Document Watcher Branch Selection

## Summary

Update the public integration guide so users know that watcher branch pickers
distinguish local and `origin/` refs and that freshness follows the repository's
worktree-sync policy.

## In scope

- Update the Jira watcher guidance with the qualified remote-ref choice.
- Clarify the relationship to **Always pull before creating a new worktree**.
- Keep the guidance consistent for other shared watcher surfaces where they are
  already documented.

## Out of scope

- A new public documentation page.
- Repeating the complete worktree-refresh reference in the integration guide.

## Acceptance

- The Jira watcher procedure explains local versus qualified remote choices.
- The guide links remote freshness to the repository setting without promising
  a watcher-specific pull operation.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/integrations.md`

## Dependencies

- Task 02 proves the shipped selector behavior on desktop and phone.

## Risks

- Documentation must not imply that selecting a remote ref overrides a disabled
  repository refresh policy.

## Parallelism

`sequential`

## Inputs

- Watcher remote base-branch requirement and system design.
- Existing public executor and Git-operation refresh guidance.

## Results

Updated the Jira watcher guidance to explain local and qualified remote refs
and link their freshness behavior to the repository worktree-sync policy.

Both public-document validation commands passed. The test suite reported 61
passing tests, and the validator reported 41 validated published pages.
