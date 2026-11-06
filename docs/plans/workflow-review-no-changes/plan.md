---
title: Persist automatic review skips in the existing task timeline
status: done
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
system_design:
  - ../../specs/agents/system-design/workflow-review-no-changes.md
---

# Persist automatic review skips

Complete the automatic workflow callback to the existing durable task status
surface. The [work order](task-01-passive-skip-notice.md) preserves the runner's
no-run rejection and reuses the existing completed-lifecycle message path.
This is independent of admission, model policy and normal review execution.

Affected existing task timeline:

```text
Task conversation
  [info] No changes to review yet.
  existing messages and agent state remain unchanged
```

The same stored status row is shown after reload on desktop and phones. No
successful review row, new control, schema, API or permanent test is added.

The original producer baseline failed three focused behavior checks. The
candidate passed the affected three Go packages with the race detector and
two UI checks using the actual SQLite callback output. Normal hooks must pass
before upstream publication; operating installation and task acceptance remain
separate work.
