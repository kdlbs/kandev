---
status: active
system: integrations
created: 2026-11-24
owners:
  - Kandev
---

# PR snapshot collection

### REQ-INTEGRATIONS-PR-STATE-001: Read a stable snapshot of the selected pull request

**Intent:** An observer can collect GitHub review evidence from a workspace without a checkout.

#### Acceptance criteria

- **AC-INTEGRATIONS-PR-STATE-001.1:** An explicit repository selects the target for snapshot, comment and log reads even when the working directory has no remote.
- **AC-INTEGRATIONS-PR-STATE-001.2:** History mode checks the opening and closing head commits. A head change makes review collection incomplete.
