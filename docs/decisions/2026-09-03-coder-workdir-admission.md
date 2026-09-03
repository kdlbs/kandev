# ADR-2026-09-03-coder-workdir-admission: Gate Coder launches on durable workdir admission

**Status:** accepted
**Date:** 2026-09-03
**Area:** backend, infra, workflow

## Context

Coder is the preferred interactive executor because it combines sandboxing,
isolated development servers, auto-pause, and out-of-band IDE access. A warning
cannot prevent a multi-phase workflow from starting on storage that disappears
when the Coder workspace is rebuilt.

Kandev can prove remote path usability at launch, but cannot infer the
persistence contract of every custom Coder mount from its path alone.

## Decision

Detected Coder SSH environments require an executor-profile-owned workdir
policy before any agent or task workspace is created. The normal policy is
`durable`; Kandev then rejects structurally unsafe paths and live-probes the
configured root for existence and write/execute access.

Known ephemeral home and temporary roots require the explicit
`allow_ephemeral` policy. Its name and errors identify the data-loss risk. The
escape hatch never bypasses the live usability checks. Admission probes use
unique child paths and do not alter task, port, or process isolation.

## Consequences

Misconfigured Coder workflows fail before Direction instead of after a later
phase transition. Operators must provision the mount and deliberately record
its policy. Existing non-Coder SSH launches are unchanged.

The durability declaration is an operator assertion, not proof that an
external storage provider will never lose data. Rehome and loss-warning
behavior remains necessary for provider failures after admission.

## Alternatives Considered

- Keep the warning advisory. Rejected because it permits the known task-loss
  sequence to begin.
- Reject all SSH custom roots. Rejected because Kandev cannot infer arbitrary
  non-Coder storage contracts and would regress ordinary SSH use.
- Infer durability only from path names. Rejected because Coder templates and
  mounts are customizable; path names are not a storage contract.
- Serialize all Coder launches through a shared probe. Rejected because unique
  probe paths provide safety without coupling otherwise isolated tasks.
