# ADR-2026-08-10-remote-contribution-head-drift: Separate Current Contribution and Local Checkout Histories

**Status:** accepted (remote-action policy superseded by 2026-08-12-local-first-contribution-replacement)
**Date:** 2026-08-10
**Area:** backend, frontend, protocol, GitHub, GitLab

## Context

The local-first replacement policy in
[ADR-2026-08-12](2026-08-12-local-first-contribution-replacement.md) supersedes this record's decision
to block all remote actions after divergence. The history-classification and non-destructive detection
rules remain active.

A remote-contribution task starts from the provider-reported head SHA and tracks the contributor's
source branch. After the task starts, the contributor can advance or rewrite that branch. The local
checkout, its remote-tracking ref, and the provider API can then describe different moments or different
commit graphs.

The Changes panel currently merges local commits with the provider's current PR commits by SHA and
labels every unmatched local commit as unpushed. This is correct for ordinary local-ahead work, but it
is false after a rewrite: the unmatched commits can be the contributor's superseded history, not work
created in Kandev. The green arrow expresses push state, not authorship, but the flat list makes those
meanings indistinguishable. The UI also uses base-branch `ahead` for Push even though the backend
already computes separate upstream-relative counts.

Automatically resetting to the provider head would make the display current by discarding local work.
Automatically merging or rebasing would create a new history without user intent. Matching rewritten
commits by message, author, file totals, or patch identity would hide real graph divergence and make
remote mutation decisions from heuristic evidence.

## Decision

Kandev treats provider history and local checkout history as separate authorities:

- The provider's current commit list is authoritative for what the pull request or merge request
  currently contains.
- The checkout's Git graph is authoritative for local HEAD and local work.
- The configured upstream ref supplies push/pull divergence evidence. Base-branch divergence remains a
  separate review/rebase concept.
- Commit identity and graph reachability are the only reconciliation evidence. Kandev does not collapse
  different SHAs because their content or metadata appears equivalent.

Git status publishes local HEAD, upstream ref identity, upstream head SHA, base-relative counts, and
upstream-relative counts. The frontend combines that evidence with the provider's ordered current
commit list and classifies the selected contribution as:

- `aligned`: local HEAD and provider head are the same.
- `local_ahead`: the tracked upstream equals the provider head and local commits are strictly ahead.
- `provider_ahead`: the provider history contains local HEAD and can be fast-forwarded.
- `diverged`: local and current provider histories both have unique commits, including a force-pushed
  provider history that no longer contains local HEAD.
- `unknown`: required Git or provider evidence is unavailable.

Aligned and local-ahead histories may retain the unified commit presentation. Provider-ahead history
may offer Pull. Diverged history is rendered as two explicitly titled lists: current provider commits
and local checkout commits. Commits in the local list are described as local-checkout history, not as
"unpushed" merely because their SHA is absent from the provider list.

Push availability and counts use `remote_ahead`; Pull availability and counts use `remote_behind` or a
proven provider-ahead relationship. Base-relative `ahead`/`behind` remains available for review scope,
change-request creation before an upstream exists, and base-branch rebase/merge context. When a linked
contribution is diverged, Push, force-push, and generic Pull are disabled on desktop and mobile. The
backend's existing prohibition on contribution force pushes remains the final enforcement boundary.

Kandev never changes the checkout merely because drift is detected. Local commit, amend, revert, reset,
and terminal workflows remain local operations; the warning must make clear that reconciliation is
required before remote mutation.

## Consequences

- A contributor rewrite no longer creates a misleading combined commit count or green arrows on
  superseded contributor commits.
- Users can inspect and recover local work without Kandev silently deleting or transforming it.
- Desktop and mobile Git controls share the same remote-action safety decision.
- The Git status wire and frontend store gain upstream-head and upstream-divergence fields that the
  backend already mostly computes.
- Provider commit loading becomes part of the confidence model. Loading and error states must not be
  collapsed into an empty history.
- This change detects and contains drift but does not provide a one-click reconciliation workflow.

## Alternatives considered

### Reset every contribution checkout to the latest provider head

Rejected. It keeps the panel visually simple by destroying local commits and uncommitted work, and it
turns an observation into an unauthorized destructive operation.

### Fetch, merge, or rebase automatically

Rejected. Each operation creates a different history and conflict policy. Kandev cannot choose among
them without user intent, especially after a force push.

### Deduplicate rewritten commits by patch or metadata

Rejected. Patch equivalence is useful diagnostic evidence but not Git identity or ancestry. It can hide
meaningful changes and cannot justify a Push or Pull decision.

### Keep one flat list and change the arrow tooltip

Rejected. A tooltip does not communicate that the rows come from incompatible histories, and the
combined count still falsely describes the current PR.

### Rely on Git push rejection

Rejected. A non-fast-forward rejection protects the remote but leaves the UI misleading and makes a
predictable unsafe action fail only after the user invokes it.
