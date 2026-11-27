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
