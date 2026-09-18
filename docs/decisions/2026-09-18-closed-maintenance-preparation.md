# ADR-2026-09-18-closed-maintenance-preparation: Prepare workflow repairs through closed operations

**Status:** accepted
**Date:** 2026-09-18
**Area:** backend, protocol, workflow

## Context

Repeated native questions, denials and launch failures can justify investigation
without establishing permission to change an enforcement policy. A prompt-only
file list cannot constrain a general worker's shell, repository writes or remote
actions. The central assistant already uses a restricted native broker.

## Decision

Only typed native events contribute to friction fingerprints. Preserve their
occurrence times, account/profile revision and known origin; unknown provider
classifier details remain unknown. Reconciliation does not turn an old request
into a new incident. Retain bounded metadata without prompt bodies.

A human grant selects an accessible local repository, ordinary workflow and
entry step without automatic actions, execution profile, exact files, local
actions, fixed positive and negative check arguments, local container image and
expiry within seven days. Grant creation checks the native workspace and profile
and qualifies the installed sandbox before accepting execution authority.

Preparation creates one ordinary review task linked to a maintenance objective
and a private checkout of the selected repository's committed base. The task
cannot launch a general agent, including after the feature is disabled or its
metadata changes. Preparation does not invoke contribution-fork creation.
Maintenance objectives and their human confirmation records cannot authorize
ordinary delivery delegation.

The broker exposes closed file reads, compare-and-swap patches, checks and local
commit operations. Every effect rechecks binding, owner, intent, native resource
access, grant expiry/revocation and profile/configuration revisions. File names
must be exact, relative, non-symlink paths. Known enforcement/configuration paths
are protected; this path filter is not a semantic classifier for arbitrary code.

Checks run only through a qualified local Linux Docker daemon and an immutable
local image. They receive the entire committed repository snapshot read-only,
no network, a read-only root, a temporary scratch directory, a non-root identity,
no capabilities, no privilege escalation and bounded process/memory/CPU/time
resources. They receive no host home, credentials, socket or inherited process
environment. Unsupported hosts/images remain proposal-only. Kandev does not pull
an image automatically.

Git operations use a private checkout with ambient configuration, hooks and
signing disabled. A local commit requires passing positive and negative checks
for the same grant revision and exact current tree. This setting applies only to
the isolated repair artifact; development repository hooks remain in force.
Receipts retain effects even if a grant is subsequently revoked. A prepared
commit is not a resolved incident: resolution requires subsequent native success
and a separate human review. No operation publishes, pushes, creates a PR,
deploys, restarts Kandev, changes a live checkout or grants itself authority.

## Consequences

Local repairs are inspectable and revocable before subsequent effects. The
initial executor needs a local Linux Docker daemon, an available suitable image
and checks that can run without network access or writes to the repository.
Owners must understand that approved checks can read the repository snapshot,
including committed files outside the patch allowlist.

Review presentation, artifact export, recurrence after closure and observed
resolution remain required parts of the maintenance work order; implementing
the executor alone does not complete that feature. Other executor families need
equivalent enforcement and negative tests before being supported.

## Alternatives Considered

- A general worker with prompt instructions cannot enforce the exact file and
  effect boundary, so it cannot consume this narrow grant.
- A proposal-only implementation is safe on unsupported hosts but does not
  provide the requested local preparation on hosts that can enforce it.
- Running tests directly on the host would expose ambient credentials and
  unbounded local effects. The isolated container controls are required.
- Automatically publishing a validated patch would cross the explicitly local
  maintenance grant and remains a separate human action.
