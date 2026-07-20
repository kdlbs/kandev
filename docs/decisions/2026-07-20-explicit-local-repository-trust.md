# ADR-2026-07-20-explicit-local-repository-trust: Explicit Local Repository Trust

**Status:** accepted
**Date:** 2026-07-20
**Area:** backend, frontend

## Context

Kandev uses `repositoryDiscovery.roots` to bound automatic filesystem scans, defaulting to the
current user's home directory. The same roots were also reused to reject manually entered local
repository paths and later Git operations. Packaged Windows users could therefore browse to a valid
repository outside their home directory but could not add or use it, and the packaged runtime did
not provide a durable configuration-file location for widening the roots.

The existing product already treats an explicitly selected local folder as trusted by the local
user, and repository records persist an exact `local_path`. Automatic traversal and explicit
selection have different risk and lifecycle characteristics and should not share one boundary.

## Decision

`repositoryDiscovery.roots` and `repositoryDiscovery.maxDepth` govern automatic repository
discovery only. They do not determine whether a user may explicitly select a repository.

An explicit local repository selection is validated and canonicalized by the backend before it is
persisted. The resulting repository record, scoped to its workspace, is the durable trust grant for
that exact repository path. The grant does not extend to the repository's parent directory, drive,
or other repositories beneath the same parent.

Git metadata indirection must preserve that exact grant. A normal `.git` directory must not redirect
its common directory, and a file-based `.git` pointer is accepted only when the target metadata has
the reciprocal backpointer and common-directory placement created by `git worktree`. Saved
repository operations also require the path to resolve to the same canonical location recorded at
creation time. Unverifiable layouts such as arbitrary `.git` pointers or `--separate-git-dir` are
rejected rather than treated as linked worktrees.

Read-only pre-registration operations may inspect an explicitly supplied path so the UI can
validate it and show repository status before saving. Fetches and destructive Git operations must
resolve a persisted repository identity server-side instead of trusting a raw path from the
request. Provider-backed repositories may remain pathless until Kandev materializes them.
Workspace-qualified routes must additionally verify that the persisted repository belongs to the
workspace named by the route before reading provider or filesystem state.

Containment checks that remain necessary for automatic scans or nested Git metadata use canonical
filesystem paths and platform-correct comparisons. Windows drive-letter casing, separators, and
UNC volume semantics must not be implemented with a raw string-prefix comparison.

## Consequences

- Native Windows, macOS, and Linux users can add an exact repository anywhere the Kandev process
  can access without changing a global scan root.
- Automatic discovery remains bounded and does not start traversing whole drives or newly trusted
  parent trees.
- Repository creation and local-path updates become authoritative validation boundaries instead of
  relying on prior UI validation.
- Legitimate linked Git worktrees remain usable, while unverifiable external Git-metadata layouts
  require a separate product and security decision before support is added.
- Git operations increasingly require repository IDs, which narrows the impact of forged raw paths
  but requires migration of existing path-oriented service and handler calls.
- Deployments exposed to untrusted users still need an authentication and authorization policy;
  discovery roots are not a substitute for that policy.

## Alternatives Considered

1. **Persist user-editable global discovery roots.** Rejected because selecting one repository
   should not grant access to every sibling repository or make automatic scans traverse a wider
   tree.
2. **Trust all drives for native desktop or CLI launches.** Rejected because launch-mode detection
   is not a durable security boundary and the behavior would diverge across packaging modes.
3. **Search `~/.kandev/config.yaml` or expose `--config` only.** Rejected as the primary fix because
   it leaves the normal Add Local Repository workflow broken and requires users to understand
   backend configuration.
4. **Use the launcher's original working directory as the default root.** Rejected because it is
   session-dependent, does not help repositories on other drives, and conflates process startup
   location with durable repository trust.
