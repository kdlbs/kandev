---
id: "01-run-dedup-generation"
title: "Implement Office run deduplication generation identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-DEDUP-001
  - REQ-OFFICE-RUN-DEDUP-002
  - REQ-OFFICE-RUN-DEDUP-003
  - REQ-OFFICE-RUN-DEDUP-004
acceptance_criteria:
  - AC-OFFICE-RUN-DEDUP-001.1
  - AC-OFFICE-RUN-DEDUP-001.2
  - AC-OFFICE-RUN-DEDUP-001.3
  - AC-OFFICE-RUN-DEDUP-001.4
  - AC-OFFICE-RUN-DEDUP-001.5
  - AC-OFFICE-RUN-DEDUP-001.6
  - AC-OFFICE-RUN-DEDUP-001.7
  - AC-OFFICE-RUN-DEDUP-001.8
  - AC-OFFICE-RUN-DEDUP-001.9
  - AC-OFFICE-RUN-DEDUP-002.1
  - AC-OFFICE-RUN-DEDUP-002.2
  - AC-OFFICE-RUN-DEDUP-002.3
  - AC-OFFICE-RUN-DEDUP-002.4
  - AC-OFFICE-RUN-DEDUP-003.1
  - AC-OFFICE-RUN-DEDUP-003.2
  - AC-OFFICE-RUN-DEDUP-003.3
  - AC-OFFICE-RUN-DEDUP-003.4
  - AC-OFFICE-RUN-DEDUP-004.1
  - AC-OFFICE-RUN-DEDUP-004.2
  - AC-OFFICE-RUN-DEDUP-004.3
  - AC-OFFICE-RUN-DEDUP-004.4
  - AC-OFFICE-RUN-DEDUP-004.5
  - AC-OFFICE-RUN-DEDUP-004.6
system_design:
  - ../../specs/office/system-design/run-dedup-generation-01.md
  - ../../specs/office/system-design/run-dedup-generation-02.md
  - ../../specs/office/system-design/run-dedup-generation-03.md
---

# Task 01: Implement Office run deduplication generation identity

## Scope

- Add and preserve the task assignment generation across schema migrations.
- Carry the immutable assignee profile ID and generation in task-created events.
- Derive the same generation-aware assignment key in each producer.
- Keep unresolved generations keyless and observable.
- Bound metric label cardinality for agent-supplied reasons.
- Avoid changes to queue coalescing and other deferred deduplication work.

## Acceptance

An assignment replay uses one durable generation key. A later assignment to the
same agent gets a new generation and can enqueue work. A task-created event
replayed after its run is claimed remains deduplicated. An unknown metric reason
does not create a new permanent expvar label.

## Verification

```bash
cd apps/backend && go test ./internal/runs/service ./internal/office/service \
  ./internal/office/dashboard ./internal/task/service
python3 scripts/lint-spec-files.py --all
```

## Results

Implemented in the contributor change plus fixup commit
`44a5ccc94d8d1fc212b9e839a240d1db2fe16e5f`. The fixup also carries the current
`main` branch and resolves the PR merge conflict. Focused backend tests and
specification lint pass.
