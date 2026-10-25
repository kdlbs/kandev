# ADR-2026-08-28-explicit-submodule-repository-trust: Verify submodule repository ownership

**Status:** accepted
**Date:** 2026-08-28
**Area:** backend

## Context

Explicit local repository registration must validate a selected path without
allowing that path to borrow unrelated Git metadata. Git submodules use a
regular-file `.git` pointer but do not have linked-worktree reciprocal files.

## Decision

Kandev accepts regular-file `.git` metadata only when either the existing
linked-worktree reciprocal proof succeeds or an initialized submodule proves
ownership through `core.worktree` in its canonical module metadata directory.
The resolved worktree must canonically equal the selected directory. Metadata
with `commondir` remains a linked-worktree-only shape and cannot use the
submodule proof. Submodule metadata that enables Git includes or
`extensions.worktreeConfig` is rejected because this validator does not
evaluate those alternate configuration sources.

## Consequences

Initialized submodules can be registered explicitly while arbitrary pointers,
missing or empty `core.worktree`, alternate configuration sources, and
mismatched paths fail closed.

## Alternatives Considered

Accepting any pointer to Git-looking metadata would widen the exact-path grant.
Using Git subprocesses would make validation environment-dependent and obscure
the reciprocal proof.
