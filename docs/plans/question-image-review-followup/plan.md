---
created: 2026-10-25
status: completed
requirements:
  - REQ-TASKS-QUESTION-IMAGES-003
  - REQ-TASKS-QUESTION-IMAGES-005
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
legacy_specs: []
---

# Question image review follow-up

## Overview

Repair confirmed findings from the user-requested secondary review of merged
[PR #4766](https://github.com/kdlbs/kandev/pull/4766). Review baseline:
`6a72759faa45fe77c18fec4212ebb4ed2ef06ac7`, whose tree matches original head
`9f407f2a31ccc53dd5911071489063922773a7f1`. The user authorized fixes and a
follow-up PR. The existing approved requirements and design remain authoritative.

## Scope and evidence

The collapsed clarification panel keeps its overlay mounted to preserve answers.
It disables keyboard shortcuts but does not disable image reads. Queued question
and choice previews therefore continue to load under a hidden panel. This violates
AC-TASKS-QUESTION-IMAGES-003.1. Reuse the image hook's existing inactive cleanup.
The remediation review also found that reopening a four-image group could reuse
revoked URLs for queued images. Clear the snapshot when image resources are released.

## Work orders

| Order | Work order | Status |
| --- | --- | --- |
| 1 | [Stop hidden image reads](task-01-stop-hidden-image-reads.md) | completed |

## Desktop and mobile outcome

Keep the existing shared panel, collapse button, scroll owner, keyboard behavior,
phone hit targets, and image inspection surface. Collapse stops image work and
releases preview URLs. Expansion reloads previews while preserving answer drafts.
The existing phone clarification panel is the mobile exemplar. No new copy or
layout is introduced.

```text
Expanded, desktop or phone       Collapsed
[question + compact previews]    [Questions   Expand]
[choices + answer draft]         No image reads or retained preview URLs
[Collapse]                      Answer draft stays mounted
```

## Validation and delivery

Task 01 owns the failing regression, focused checks, and final results. Run the
shared browser flow on desktop and phone. Publish only confirmed review fixes.
No public documentation change is required for restoring the documented loading
contract. The secondary review identified the collapse issue and then the related
stale-URL reopening issue. Both fixes passed its remediation review. It
found no additional concrete security, ownership, architecture, or simplification
defect. Its review covered the merged production diff and prior review fixes.
