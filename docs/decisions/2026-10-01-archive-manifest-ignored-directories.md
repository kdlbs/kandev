# ADR-2026-10-01-archive-manifest-ignored-directories: Bound ignored directory evidence

**Status:** accepted
**Date:** 2026-10-01
**Area:** backend

## Context

Task cleanup retains source identities before worktree removal. Git reports
ignored dependency and build directories as directory entries. Recursive hashing
turns those entries into work proportional to all installed artifact bytes.
Issue [#4130](https://github.com/kdlbs/kandev/issues/4130) demonstrates the resulting
deadline failures and backend startup delays.

Ignored files can contain local source or configuration. Existing tests preserve
their hashes without retaining secret contents. A blanket removal of all ignored
evidence discards that useful contract.

## Decision

An ignored directory receives path/status evidence and an explicit
`content_omission: "ignored_directory"` marker. Capture does not read descendants.
Ignored regular files retain content hashes. Tracked entries and dirty submodules
retain their existing content rules.

All source inspection obeys the cleanup attempt context between entries and
bounded reads. Task-resource cleanup recovery runs in its owned worker.
Worker registration does not wait for due filesystem cleanup.

The design follows
[Task Cleanup Source Manifest](../specs/tasks/system-design/archive-source-manifest.md).
Implementation is complete in the
[fix package](../plans/archive-manifest-bounded-cleanup/plan.md).

## Consequences

Installed ignored directory size no longer determines manifest hashing cost.
Consumers can distinguish omitted evidence from deleted paths. They cannot
claim content integrity for omitted directory contents.

Historical manifests remain readable and retain their original meaning.
Successful persisted evidence remains reusable. Ownership validation, claim
fencing, and the pre-cleanup persistence barrier remain mandatory.

Cascade retries retain the recoverability policy in
[ADR-2026-09-05-durable-task-cascade-mutations](2026-09-05-durable-task-cascade-mutations.md).
A deadline error alone does not authorize terminal failure or destructive cleanup.

## Alternatives Considered

- Remove all ignored entries: discards existing local-file evidence.
- Hash directory metadata: does not prove content identity and still needs an
  explicit omission contract.
- Exclude names such as `node_modules`: misses arbitrary ignored artifacts and
  can omit legitimate tracked source without additional Git classification.
- Increase the deadline or persist partial hashes: retains artifact-size cost
  and complicates evidence without fixing startup ownership.
- Terminalize cascade deadlines: can permanently block recoverable lifecycle
  operations and conflicts with the existing cascade retry contract.
