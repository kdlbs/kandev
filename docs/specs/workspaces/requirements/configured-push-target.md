---
status: draft
system: workspaces
created: 2026-09-08
owners:
  - kandev
---

# Configured Push Target Requirements

## Overview

The workspace push API can today publish only to `origin` or to a remote a
contribution binding selects. A caller cannot name a third destination, and
cannot state which branch it believes it is publishing.

This capability adds two independent, optional inputs to the workspace push and
push-preflight contracts: an explicit push target, and an expected branch. The
expected branch exists so a caller that decided to publish `feature/x` cannot
publish different content under that name when the agent moved `HEAD` between
the decision and the push.

The workspace system owns this contract because it governs workspace Git state
and its remotes, not the runtime that executes the push.

## Terminology

- **Push target:** The destination a push publishes to, expressed either as a
  configured Git remote name or as a remote URL.
- **Explicit push target:** A push target the caller names in the request,
  rather than one Kandev derives from `origin` or a contribution binding.
- **Configured remote:** A remote already present in the checkout's Git
  configuration.
- **Effective push URL set:** A configured remote's push URLs when it has at
  least one, otherwise the single-element list holding its fetch URL. A remote
  with neither has an empty set.
- **Contribution routing:** The existing remote-contribution and
  contribution-destination behavior that selects a remote and refspec for the
  caller.
- **Expected branch:** The branch name a caller states it intends to publish.
- **Current branch:** The branch the workspace checkout's `HEAD` points at.
  A detached `HEAD` has no current branch.

## Requirements

### REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-001: Explicit Push Target Selection

**Intent:** Let a caller publish a task branch to a remote it names, without
changing what a caller that names nothing observes today.

**User story:** As a Kandev service that publishes task work, I want to name the
remote a push goes to, so that a work branch can be preserved somewhere other
than `origin`.

#### Acceptance criteria

- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.1:** The workspace push and
  push-preflight contracts shall accept an optional explicit push target.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.2:** When a request omits the
  explicit push target and omits the expected branch, Kandev shall perform the
  same push it performs today, including `origin` selection, contribution
  routing, upstream selection, force behavior, and first publication.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.3:** When the explicit push target
  satisfies the permissive ref-syntax allowlist Kandev already applies to branch
  arguments, Kandev shall interpret it as a configured remote name; otherwise
  Kandev shall interpret it as a remote URL.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.4:** When the explicit push target
  is interpreted as a name and no configured remote carries that name, Kandev
  shall refuse the request with error code `push_remote_not_found` and shall not
  contact any remote.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.5:** When the explicit push target
  is interpreted as a URL, Kandev shall resolve it to a configured remote whose
  effective push URL set holds exactly one entry equal to the supplied value.
  Each configured value shall be trimmed likewise, and the comparison shall
  otherwise be exact: no case folding, no `.git` suffix added or
  removed, no redirect resolved, no normalization. An empty set never matches.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.6:** When a URL push target matches
  more than one configured remote, Kandev shall select the matching remote whose
  name sorts first by byte order and shall publish to it.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.7:** When a URL push target matches
  no configured remote, Kandev shall refuse the request with error code
  `push_remote_url_unmatched` and shall not contact any remote.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.8:** Kandev shall never create,
  rename, retarget, or delete a remote while serving a push or push-preflight
  request. A push target that is not already configured shall be refused rather
  than materialized.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.9:** When a request carries an
  explicit push target and the workspace has contribution routing configured,
  Kandev shall refuse with error code `push_remote_contribution_conflict` and
  shall not push.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.10:** When a request carries an
  explicit push target together with the set-upstream flag, Kandev shall refuse
  the request with error code `push_remote_upstream_unsupported` and shall not
  push.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.11:** A push that carries an
  explicit push target shall not set, change, or remove the current branch's
  upstream tracking configuration.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.12:** When a request carries an
  explicit push target that resolves to a remote other than `origin`, Kandev
  shall not perform empty-remote first publication. When it resolves to
  `origin`, first publication shall behave exactly as it does for a request that
  names no push target.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.13:** A push that carries an
  explicit push target shall remain a non-force push unless the caller requests
  force, and force shall stay refused whenever contribution routing applies.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.14:** No result field Kandev
  populates, no error message Kandev composes, and no log record Kandev emits
  shall contain a remote URL; each shall identify a push target by its resolved
  remote name only. Git's passed-through output is excluded and keeps naming the
  destination it contacted, as it does today.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.15:** Remote-name matching shall be
  exact and case-sensitive. Kandev shall not accept a prefix, a suffix, or a
  case-insensitive variant of a configured remote name.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.16:** When Kandev cannot read the
  checkout's remote configuration while resolving an explicit push target, it
  shall refuse the request with error code `push_remote_config_unreadable` and
  shall not push. An unreadable configuration shall never be treated as an absent
  push target, as an unmatched push target, or as a successful resolution.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.17:** When no configured remote
  matches a URL push target under
  AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.5 but at least one remote carries that
  value among several push URLs, Kandev shall refuse with error code
  `push_remote_fanout` and shall not push. A matching
  single-entry remote takes precedence and resolves normally; only when none
  exists does the fan-out refusal apply, and only when neither exists does
  `push_remote_url_unmatched` apply. Naming a multi-push-URL remote stays
  allowed and shall publish through it unchanged, reaching every configured push
  URL without reduction or reordering.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.18:** On the explicit-push-target
  push path, and on the preflight paths this capability introduces or changes,
  Kandev shall apply to Git's passed-through output the same credential redaction
  it already applies to empty-remote first-publication output. The push path
  naming no explicit push target, and the existing contribution preflight paths,
  shall keep the output handling they have today.

### REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-002: Expected-Branch Verification

**Intent:** Let a caller state the branch it decided to publish, so an agent that
switches branches after that decision cannot cause different content to be
published under that name.

**User story:** As a Kandev service that publishes task work, I want the push to
be refused when the workspace is no longer on the branch I decided to publish,
so that I never publish one branch's commits as another branch.

#### Acceptance criteria

- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.1:** The workspace push and
  push-preflight contracts shall accept an optional expected branch, independent
  of whether the request carries an explicit push target.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.2:** When the expected branch is
  supplied and is not a valid branch name, Kandev shall refuse the request with
  error code `push_branch_invalid` and shall not contact any remote. Valid means
  the strictest branch allowlist Kandev already applies to a persisted branch
  name: beyond the permissive rules on characters, length, `..` and `.lock`, an
  expected branch ending in `/`, containing `//`, or equal to a reserved Git
  symbolic pseudo-ref (`HEAD`, `ORIG_HEAD`, `FETCH_HEAD`, `MERGE_HEAD`) shall
  also be rejected.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.3:** When the expected branch is
  supplied and does not equal the current branch at the first verification point,
  Kandev shall refuse with error code `push_branch_mismatch`, shall report the
  expected and current branches in the result fields the system design declares,
  and shall not contact any remote. A detached `HEAD` shall be reported as an
  empty current branch, not as the literal `HEAD`.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.4:** When the expected branch is
  supplied and `HEAD` is detached, Kandev shall treat the request as a mismatch
  under AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.3.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.5:** On the push operation Kandev
  shall verify the expected branch at two points inside the workspace Git
  operation lock: once before it contacts any remote, so that a mismatch refuses
  the request with no side effect; and again as the last Git read before the push
  command, with no other Kandev-issued Git command between that verification and
  the push. A mismatch at either point shall refuse the request.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.6:** When the second verification
  fails, Kandev shall refuse the push, shall keep the task branch unpublished,
  and shall report the expected and current branches as in
  AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.3. When empty-remote first publication
  published the baseline earlier in the same request, the refusal shall carry
  error code `push_branch_mismatch_after_baseline` and shall set the result's
  baseline-published field; otherwise it shall carry `push_branch_mismatch` and
  shall omit that field. A second-point refusal is not required to be
  side-effect-free: empty-remote preparation may already have read `origin`'s
  advertised refs, and may already have retired the local first-publication
  marker without publishing anything.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.7:** When a request carries an
  explicit push target and an expected branch, Kandev shall publish the current
  `HEAD` to the branch named by the expected branch on the resolved remote.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.8:** When a request carries an
  explicit push target and no expected branch, Kandev shall publish the current
  `HEAD` to the branch named by the current branch on the resolved remote, and
  shall refuse a detached `HEAD` with error code `push_branch_detached` without
  contacting any remote.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.9:** When a request carries an
  expected branch and no explicit push target, the expected branch shall act only
  as a precondition. Kandev shall not change the remote, the refspec, the
  upstream behavior, or the first-publication behavior that the request would
  otherwise have.

- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.10:** When the destination branch
  does not exist on the resolved remote, the push shall create it. Creating it
  on a remote other than `origin` shall not publish a baseline commit.

- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.11:** Kandev shall determine whether
  `HEAD` is detached by reading `HEAD` as a symbolic ref and observing whether it
  resolves under `refs/heads/`, and shall not infer the detached state by
  comparing an abbreviated ref name against the literal string `HEAD`.

- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.12:** Every Git read Kandev needs to
  build the push command shall complete before the second expected-branch
  verification required by AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.5, including
  the upstream-tracking read on the path naming no explicit push target. Between that verification and the push, Kandev
  shall issue no Git command other than the push itself.

### REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-003: Push Preflight Validation

**Intent:** Make push preflight answer whether the destination-branch push that
would run is writable, instead of reporting success for a target it never
checked.

**User story:** As a Kandev service preparing to publish, I want preflight to
check the destination my push would actually use, so that a failure surfaces
before the push runs.

#### Acceptance criteria

- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.1:** Push preflight shall apply the
  same push-target resolution, expected-branch validation and comparison, refusal
  codes, and refusal ordering that the push contract applies, and shall verify the
  expected branch once, before its dry-run. Preflight publishes no baseline, so it
  has no second verification and shall never report
  `push_branch_mismatch_after_baseline`.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.2:** When a request carries an
  explicit push target, preflight shall verify write access to the resolved
  remote for the refspec the equivalent push would use.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.3:** When a request carries no
  explicit push target and no contribution routing applies, preflight shall
  verify write access to `origin` for the refspec the equivalent push would use,
  instead of reporting success without checking a remote.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.4:** When no explicit push target is
  supplied, no contribution routing applies, and the checkout has no `origin`
  remote, preflight shall report failure with error code
  `push_no_remote_configured`.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.5:** Push preflight shall not create,
  move, update, or delete any ref on any remote, and shall not change local refs,
  the working tree, or upstream tracking configuration.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.6:** Preflight against a remote that
  refuses the write shall report failure with the remote's output rather than an
  unhandled error, credential-redacted under
  AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.18.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.7:** A successful preflight to which
  contribution routing did not apply shall report the resolved remote name and
  destination branch it validated, in the same result fields a successful push
  uses, whether or not the request carried an explicit push target. A successful
  preflight under contribution routing shall report neither field and shall keep
  the result shape it has today.

### REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-004: Determinism, Concurrency, and Result Reporting

**Intent:** Make the outcome of a push request predictable when inputs conflict,
when two callers arrive together, and when a caller retries.

#### Acceptance criteria

- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.1:** When a request would be refused
  for more than one reason, Kandev shall report the first applicable refusal in
  this fixed order: malformed request body; existing contribution binding
  validation failure; `push_remote_contribution_conflict`;
  `push_remote_upstream_unsupported`; `push_branch_invalid`; explicit push-target
  resolution refusals (`push_remote_config_unreadable`, `push_remote_not_found`,
  `push_remote_url_unmatched`, `push_remote_fanout`);
  `push_no_remote_configured`; `push_branch_detached`; `push_branch_mismatch`
  raised at the first verification point. Kandev shall evaluate every refusal in
  this ordered list before it contacts any remote, including before empty-remote
  first publication.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.2:** Every refusal listed in
  AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.1 shall leave the workspace checkout,
  its refs, its upstream configuration, and every remote unchanged. The
  second-verification refusals governed by
  AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.6 are the only refusals in this
  capability that are not listed there, and the only ones permitted to leave a
  published baseline or a retired first-publication marker behind.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.3:** A push or push-preflight
  request shall hold the existing single workspace Git operation lock for the
  whole request. When a second request for the same checkout arrives while one
  holds the lock, Kandev shall report the existing operation-in-progress result
  rather than queue, merge, or serialize it.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.4:** Repeating a successful push
  request with identical inputs while the workspace `HEAD` and the destination
  branch are unchanged shall report success and shall not change the destination
  branch.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.5:** When the destination branch on
  the resolved remote has commits the local `HEAD` does not contain, a non-force
  push shall be refused by the remote and Kandev shall report that failure
  without retrying and without escalating to a force push.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.6:** A successful push that carried
  an explicit push target shall report the resolved remote name and the
  destination branch name. A successful push that carried no explicit push target
  shall omit both, so an existing consumer observes an unchanged result shape.
  This omission rule governs the push operation only; preflight reports them in
  every case under AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.7.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.7:** Kandev shall trim leading and
  trailing whitespace from the explicit push target and from the expected branch
  before any other check, including before the validation
  AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.2 requires. Either input supplied
  empty, or empty after that trim, shall be treated as absent and shall produce
  the behavior of a request that omits it.
- **AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.8:** The explicit push target and
  expected branch shall be scoped to the repository the request already selects
  in a multi-repository task, and shall not affect any other repository in the
  same workspace.

## Out of scope

- **Exact-OID lease.** No `--force-with-lease=<ref>:<oid>` form. A force push
  keeps emitting the bare lease it emits today, and force stays refused for
  contribution routing.
- **Eliminating the residual race window.** An agent shell can move `HEAD`
  outside the operation lock, so the expected-branch check narrows the window but
  cannot close it. Closing it requires the exact-OID lease above.
- **Preflighting the empty-remote baseline.** A push on the `origin` path may
  first publish the baseline, a different refspec. Preflight validates only the
  destination-branch refspec, so it can still report success where that baseline
  publication would fail.
- **Auto-push policy.** When and under what condition a branch publishes
  automatically is a separate capability; this contract only makes such a caller
  expressible.
- **Creating or configuring remotes.** Adding a backup remote belongs to
  workspace materialization, not the push contract.
- **Changing contribution routing.** Remote-contribution and
  contribution-destination selection, their force-push refusal, and their
  preflight behavior are unchanged.
- **A user-facing control.** No web, desktop, or mobile surface gains either
  control; the existing push control keeps sending neither input.
- **Pushing tags, multiple refspecs, or deleting a remote branch.**
- **Credential selection.** Unchanged: the task runtime Git credential route.

## Related requirements

- [Empty Remote Repository Requirements](empty-remote-repositories.md)
- [Branch Policies](branch-policies.md)

## System design

- [Configured Push Target System Design](../system-design/configured-push-target.md)
