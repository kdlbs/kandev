---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-CONFIGURED-PULL-TARGET-001
---

# Configured Pull Target System Design

## Purpose and boundaries

Workspaces owns selection of a checkout's ordinary Pull source. This design
changes only source selection in `GitOperator.Pull`. The
[managed contribution design](../../tasks/system-design/remote-contribution-tasks.md)
retains source binding validation and precedence. The
[configured push design](configured-push-target.md) retains publication routing.
The [workspace status design](../../platform/system-design/workspace-git-status.md)
retains upstream observations and refresh behavior. Agent-shell permission
policy remains with [Agents](../../agents/requirements/git-operations-permission-boundary.md).

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| `AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.1`, `.2` | Source resolution and command construction |
| `AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.3`, `.4` | Routing precedence |
| `AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.5` | Source resolution and operand boundary |
| `AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.6` | Existing user operation path |

All rows implement `REQ-WORKSPACES-CONFIGURED-PULL-TARGET-001`.

## Routing precedence

Inside the existing workspace operation lock, retain current-branch discovery
and contribution-binding error handling. A valid `RemoteContribution` continues
to use `ContributionRemoteName()` and `HeadBranch` before considering ordinary
upstream configuration. A `ContributionDestination` continues to select only
the push destination and contributes no Pull target.

On the ordinary path retain `getUpstreamRef` as the existing upstream-presence
check. If it returns empty, retain `getDefaultRemoteBranch` and its main/master
preference, otherwise the current local branch, all on `origin`. Do not change
that helper's error-to-empty behavior as part of this repair. A reported
upstream enters authoritative source resolution; resolution failure must not
enter the no-upstream fallback.

## Source resolution and operand boundary

Add a small Pull-specific helper in
`apps/backend/internal/agentctl/server/process/git_pull_target.go`. Through
`runGitCommand`, query `for-each-ref` for `refs/heads/<current branch>` using a
constant format containing `%(refname)`, `%(upstream:remotename)`, and
`%(upstream:remoteref)`, separated by `%00`.

Git supplies the configured remote and full source ref separately. Validate the
current branch before constructing the query; select the exact local-ref tuple,
require exactly one tuple, and validate both returned operands. Keep field
values exact; remove only record framing, not arbitrary whitespace. Do not
split a display upstream at `/`, derive a source branch from the local tracking
ref's storage name, or strip source namespaces. In particular,
`refs/remotes/cache/review` can track `refs/heads/review` on `team/upstream`.

Use the existing `securityutil.IsValidBranchName` allowlist for remote names
and full source refs, with `.` as the existing safe local-repository literal.
Require a full `refs/` source ref, and verify a named remote is configured with
the existing classified command path (`git remote get-url <name>`). A URL,
option-looking operand, wildcard, control character, or unsupported ref syntax
is not admitted. Do not use push URLs or explicit-push-target resolution, which
has a different contract. Configuration reads do not create or retarget remotes.

Git documents these upstream fields in
[for-each-ref](https://git-scm.com/docs/git-for-each-ref#Documentation/git-for-each-ref.txt-upstream).
Its [branch configuration contract](https://git-scm.com/docs/git-config#Documentation/git-config.txt-branchltnamegtmerge)
distinguishes the source ref from tracking storage and supports the local `.`
remote. Lightweight disposable Git 2.43.0 probes confirm slash-containing named
remotes, local `.` sources, last-value remote configuration, and the primary
upstream field selecting the first merge ref when multiple merge values exist.
Kandev continues its single-source operation: the same upstream Git reports to
status is selected, without claiming native octopus Pull equivalence. Native
rebase rejects multiple merge sources; this repair adds no multi-source mode.

## Source resolution and command construction

Replace the ordinary path's initialized `origin`/local-branch target with the
resolved remote and full source ref only when an upstream is reported. The
existing commands remain `pull <remote> <source-ref>` and
`pull --rebase <remote> <source-ref>`. Both modes use the same resolution.
Do not add integration flags or override `pull.rebase`, `pull.ff`, signing,
hooks, environment, authentication or transport behavior. Existing Git
configuration still governs integration as it does with explicit operands today.

Keep `runGitCommand`, the Git operation lock, time budgets, output handling,
error propagation, conflict parsing and rebase auto-abort unchanged. A failed
source resolution populates the existing `Operation: pull` failure result before
the Pull command. Do not add error codes, fields, retries, status refreshes or
new metrics. Existing logs identify the selected remote/ref after success.

## Existing user operation path

`WorkspaceTracker` publishes `remote_branch` and `remote_behind` for Git's actual
upstream. `use-session-git-derived.ts` uses upstream presence and `pullBehind`
for eligibility. `changes-panel-hooks.ts` calls `gitOps.pull(false, repo)`;
`use-session-git.ts` retains repository selection and existing fan-out.
`apps/web/hooks/use-git-operations.ts` sends `worktree.pull` with the existing
rebase boolean and repository subpath. `GitHandlers.wsPull` forwards to
`Client.GitPull`, then `POST /api/v1/git/pull`, `gitOpForRepo`, and
`GitOperator.Pull`. The existing result returns through that path.

Desktop `changes-panel.tsx` and phone
`components/task/mobile/mobile-changes-panel.tsx` both use
`useChangesPanelData`, `ChangesPanelHeader`, and the shared Git handlers;
both pass `data.git.pullBehind` and `data.pullDisabled` to the header.
Desktop and mobile consume the same Changes data and Git operations. This
backend source correction changes no layout, touch target, menu, copy, scrolling,
navigation, eligibility or viewport behavior. Real operator integration tests
are the causal evidence; existing API rebase/conflict tests cover unchanged
request wiring and recovery. No new browser or build gate is needed.

## Verification and documentation

Use independently authored, disposable real Git repositories and bare remotes
in `git_pull_target_test.go`, reusing only public repository test patterns.
Record pre-Pull HEAD, actual upstream, `remote_behind = 1`, result and final
HEAD. Compare separate equivalent checkouts against native configured Git Pull.
For the clean working tree with no local-ahead commits and source ahead by one,
assert final HEAD equals the source commit under criterion `.1`. Use
non-conflicting local commits to prove rebase replay under criterion `.2`:
assert source ancestry and replayed contents, not HEAD equality to the source.

The [delivery package](../../../plans/configured-upstream-pull/plan.md) maps all
criteria to tests and controls. No database, API schema, configuration migration,
new dependency or ADR is needed: this is a local source-resolution correction
inside existing ownership and command boundaries.

At implementation, update only the affected prerequisite wording, Pull command
row and wrong-branch troubleshooting entry in
`docs/public/git-operations.md` (a how-to guide with operation reference).
Replace the documented workaround with configured-upstream behavior and retain
managed-source precedence and no-upstream fallback. Root README and screenshot
catalog contain no affected routing contract; they need no change.
