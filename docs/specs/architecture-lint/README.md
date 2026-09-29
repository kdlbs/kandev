---
status: active
system: architecture-lint
specification_version: 1
migration: complete
owners:
  - kandev
---

# Architecture lint

## Purpose

Architecture lint keeps selected code boundaries and compatibility obligations
enforceable during local development and CI. Developers and maintainers use the
same deterministic checks before committing and during review.

## Ownership

- Architecture rule identities, scanners, and findings.
- Exact rule baselines and their shrink-only comparison.
- Compatibility-ledger schema and validation used by architecture rules.
- The local and CI entry points that run these repository checks.

## Exclusions

- Product API behavior and user-facing compatibility promises, which belong to
  their owning product systems.
- CI workflow events, permissions, and job execution, which belong to the
  [CI automation system](../ci/README.md).
- Public documentation policy and its evaluator.

## Related systems

- [CI automation](../ci/README.md): CI runs architecture checks but does not own
  their rule contracts.
