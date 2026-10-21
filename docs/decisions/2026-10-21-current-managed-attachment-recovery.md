# ADR-2026-10-21-current-managed-attachment-recovery: Bound attachment recovery to current managed destinations

**Status:** accepted
**Date:** 2026-10-21
**Area:** backend

## Context

Task source attachment already owns host Local/Worktree materialization through
the authenticated executor cloner. Live Worktree branch attachment must return
an actual sibling checkout. A saved missing provider path bypasses cloning in
both paths, while empty-path branch attachment skips materialization.

The contributor candidate for issue #4426 also accepts missing pre-workspace
provider/origin and owner/name clone paths and recreates them at today's
workspace destination. Those absent paths cannot supply surviving Git origin
or common-directory evidence. Existing managed-clone relocation admission does
not grant that authority. A wider legacy recovery rule needs its own decision.

## Decision

Recover an empty managed provider path or an absent exact current
workspace/provider/scope-derived destination during an already-authorized host
attachment. Validate existing and returned clone ownership before use. Preserve
explicit local ownership, including local rows with provider metadata.

Existing legacy managed clones remain reusable only after exact path and Git
ownership/origin validation. Do not recreate or redirect an absent legacy clone;
return re-import guidance. Retain existing task/session/environment and remote
materialization boundaries. Failed verified-path persistence prevents use.

The expanded operator-configured clone base is the control-root boundary.
Exact lexical current and allowed existing legacy derivation remains authoritative;
scoped identities cannot borrow unscoped legacy paths. Permit symlink aliases in
parents above the base for compatibility. Reject redirection at the configured
base itself and its descendants. Retain observed physical object/base identity
through the existing clone operation guard, including outside-base alias targets
when the base is absent. Alias-target/base replacement during authentication fails
before mutation within existing operation limits.

After inspection, physical Git top/gitdir/common-dir comparison may use existing
`filepath.EvalSymlinks` to reconcile trusted outside-base parent aliases. This
neither grants ownership nor rewrites lexical registration/clone placement.
Separate supported `rev-parse` queries preserve valid whitespace/newline path
bytes, removing only the terminal record newline.

Post-clone verification failure grants no authority to delete unverified shared
content. Existing task-owned compensation does not permit `RemoveAll` without a
demonstrated producer and operation-owned deletion proof. No cleanup journal,
registry or general filesystem capability/lock model is introduced.

Verified attachment writes use the narrowly named internal Go entrypoint
`Service.UpdateVerifiedManagedRepositoryPath`, called only by the attachment
`repoLocalPathUpdater` after repoclone/executor admission. The service authorizes
the persisted repository's workspace, matches the admitted identity/version,
checks the clean absolute lexical directory path, and uses existing exact
`UpdateRepositoryIfUnchanged` before `RepositoryUpdated` publication. It changes
only path/version on the loaded current row, preserving settings, bindings and
checkout intents. Read, validation, cancellation and exact-write errors are fatal.
The existing event publisher retains its ordinary delivery semantics.

Ordinary `UpdateRepository` and explicit-local canonicalization remain unchanged.
This entry accepts no explicit-local source, stale/invalid snapshot, missing path
or incomplete identity. Eligible pathless nonlocal/provider rows retain their
existing contract. No HTTP/MCP request flag, skip-validation mode, token system,
schema or new persistence model represents trusted caller provenance.

A verified workspace-level managed clone has a separate lifetime from the
new task attachment. Later attachment failure compensates task-owned rows,
worktrees and entries, but may retain that verified clone and its successfully
saved repository path for retry. No new recovery record, schema or background
repair is introduced.

This selects option A from the #4426 investigation. The parent reviewed the
design package and later explicitly authorized its sequential implementation.
Delivery still requires parent actual-diff validation and separate authorization.
Delivery is partial: a future PR must reference #4426 without
an issue-closing keyword while missing legacy recovery remains unresolved.

## Consequences

Current managed clone loss can be repaired without treating arbitrary saved
paths as clone authority. A deleted legacy clone still requires explicit
re-import. Ordinary task-checkout relocation and native-session restoration
retain their existing admission and preservation rules.

## Alternatives Considered

- Admit absent exact legacy layouts and redirect them to the current workspace
  clone. Deferred because it extends the ownership evidence accepted for
  recovery; contributor-branch approval markers do not establish that contract.
- Require re-import for every saved-missing source. Rejected for current managed
  destinations because their exact registered identity already determines the
  authorized clone location and attachment has an authenticated clone seam.
- Broaden host filesystem or provider security policy. Excluded: this decision
  scopes one existing materialization operation, not general authentication,
  repository trust, runtime recovery or sandbox behavior.

The parent selected this bounded control-root clarification during PR review.
Requiring every parent above the configured root to be canonical was rejected as
an unnecessary compatibility restriction; canonicalizing the global clone layout
was rejected because it would rewrite placement outside this attachment operation.
