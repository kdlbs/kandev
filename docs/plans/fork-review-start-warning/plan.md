---
created: 2026-11-24
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
system_design:
  - ../../specs/integrations/system-design/github-fork-review-start.md
legacy_specs: []
---

# Fork review manual-start warning

Deliver [one work order](task-01-warn-before-start.md) for the [manual-start requirement](../../specs/integrations/requirements/github-fork-review-start.md). Current session.ensure errors make Retry appear actionable. Use the existing explicit launch flow and preserve the durable restriction on unattended execution. Verify the waiting state, dismissal, and explicit launch on desktop and phone. The main risk is accidental setup during passive workspace preparation.

## ASCII UI preview

UI-01. Empty chat before the first session. The warning and action sit within the chat panel.

```text
Manual start required
Kandev could not confirm that this PR comes from the target repository.
Starting the review can run setup scripts from the PR.
Those scripts may have access to executor credentials.
[Start Task]
```

Desktop and phone use the same reading order. Text wraps within the existing scroll area. Use the existing accessible alert and a touch target of at least 44 px on phones. In Edit, place the warning next to the existing launch control. The wording and spacing shown here are illustrative.


## Acceptance evidence

- AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.7: `use-ensure-task-session.test.ts` checks that a successful waiting response becomes a manual-start state and clears when a session arrives. `sidebar-page-actions.test.tsx` covers Edit from a task outside the board snapshot.
- Desktop: `tests/task/fork-review-start.spec.ts` in `chromium` checks the task preview, chat warning, Edit, cancellation, saved runner, and reload.
- Phone: `tests/task/mobile-fork-review-start.spec.ts` in `mobile-chrome` checks the chat action, its touch target, the start dialog, cancellation, saved runner, and reload.
- Existing backend coverage for AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.6: `TestEnsureSession_ForkReviewAwaitsManualStart` and `TestEnsureSession_ForkReviewPrepareStepPassiveOpenAwaitsManualStart`.
