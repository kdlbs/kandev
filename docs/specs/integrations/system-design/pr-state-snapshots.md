---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-PR-STATE-001
---

# GitHub PR snapshot collection

The helper passes the selected repository to the GitHub CLI repository lookup.
With no explicit selection it preserves the CLI checkout lookup.
It reads the opening head before collecting review history and compares it with
the closing head. The history option controls the timestamp filter independently
of this comparison. A caller can retry an incomplete snapshot.

[Review evidence trust decision](../../../decisions/2026-07-23-opencode-review-evidence-trust.md)
