---
created: 2026-09-13
status: completed
requirements:
  - REQ-OFFICE-RUN-DEDUP-001
  - REQ-OFFICE-RUN-DEDUP-002
  - REQ-OFFICE-RUN-DEDUP-003
  - REQ-OFFICE-RUN-DEDUP-004
system_design:
  - ../../specs/office/system-design/run-dedup-generation-01.md
  - ../../specs/office/system-design/run-dedup-generation-02.md
  - ../../specs/office/system-design/run-dedup-generation-03.md
---

# Implementation Plan: Office Run Deduplication Generation Identity

## Overview

Give Office wake requests an occurrence identity that survives retries and
allows later occurrences to run. Persist the task assignment generation, carry
it through task-created events, keep producer keys convergent, and report
deduplication outcomes with bounded metric labels.

## Scope

- Persist and increment `tasks.assignment_generation` for assignment writes.
- Include the assignee profile and generation in task-created event data.
- Use generation-aware keys for assignment and other durable wake occurrences.
- Use a keyless wake when a generation cannot be resolved.
- Keep agent-supplied metric reasons bounded to known labels or `custom`.
- Preserve the existing queue lookup, unique indexes, coalescing window, and run
  lifecycle.

## Implementation Wave

- [x] [task-01-run-dedup-generation](task-01-run-dedup-generation.md)

## Verification

```bash
cd apps/backend && go test ./internal/runs/service ./internal/office/service \
  ./internal/office/dashboard ./internal/task/service
python3 scripts/lint-spec-files.py --all
```

The focused backend tests cover assignment event replay, same-agent
reassignment, bounded metric labels, and the existing deduplication paths.
