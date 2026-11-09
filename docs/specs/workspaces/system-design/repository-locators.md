---
status: current
system: workspaces
created: 2026-11-08
requirements:
  - REQ-WORKSPACES-REPOSITORY-LOCATORS-001
owners:
  - kandev
---

# Repository Locator System Design

## Purpose and boundaries

The workspace system owns source admission and preparation.
This design repairs agentctl's remote locator gate without changing executor credentials or canonical task inventory.
Registration updates consume the same conventional SCP grammar through the [registration design](remote-repository-registration.md#clone-url-repair).

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-WORKSPACES-REPOSITORY-LOCATORS-001.1, .2 | Conventional SCP grammar |
| AC-WORKSPACES-REPOSITORY-LOCATORS-001.3 | Admission sequence |
| AC-WORKSPACES-REPOSITORY-LOCATORS-001.4 | Launch and rollback |
| AC-WORKSPACES-REPOSITORY-LOCATORS-001.5 | Launch and rollback; verification |

## Components and responsibilities

- `internal/agentctl/server/api/workspace_materialize.go` owns authenticated materialize and remove handlers and destination containment.
- `validateRepositoryLocator` owns clone admission. `validateRemovalRepositoryLocator` retains its separate cleanup-only policy.
- A small `internal/common/gitlocator` package owns only strict conventional SCP parsing.
  Both backend and standalone agentctl can import this neutral package.
- `internal/agent/runtime/lifecycle/workspace_materialization.go` owns ordered projection, rescan, and compensation.
- `retained_launch_workspace.go:reconcileLaunchWorkspace` controls the primary/secondary materialization boundary.

`repoclone.CanonicalHTTPSCloneURL`, `ParseGitRemoteIdentity`, and task URL parsers already interpret SSH origins.
They serve conversion or discovery and accept broader forms. They are not clone-admission validators.
Do not broaden those converters or import task-service code into agentctl.

## Conventional SCP grammar

The parser recognizes the entire input, without trimming or percent decoding:

- The exact prefix is `git@`. Exactly one colon separates host and path.
- Host is an ASCII DNS name, including a single label, or a valid IPv4 address.
  DNS labels contain ASCII letters, digits, and interior hyphens, with normal label and total-length limits.
  Leading/trailing dots, empty labels, underscores, user information, brackets, slashes, and ports are invalid.
- Path contains one or more nonempty slash-separated segments.
  Each segment contains ASCII letters, digits, `.`, `_`, or `-`; `.` and `..` alone are invalid.
  A segment must not start with `-`. A terminal `.git` suffix is optional.
- Whitespace, controls, backslashes, `@`, `:`, `?`, `#`, percent escapes, and shell punctuation are invalid within the path.
  Leading/trailing slash, repeated slash, tilde expansion, and absolute paths are invalid.

Examples include `git@github.com:org/repo.git`, `git@gitlab.com:group/subgroup/project.git`, and `git@git.internal:team/repo`.
This is a supported subset of Git's SCP syntax, not a general SSH command parser.
IPv6 and explicit SSH ports require a separately supported URL form. This repair does not admit URL user information.

## Admission sequence

1. Reject empty input, raw whitespace, controls, absolute filesystem paths, backslashes, and raw query/fragment delimiters.
   Empty query/fragment suffixes also fail.
2. If the input starts with `git@`, apply the strict SCP parser before `url.Parse`.
   Return success only for a complete match. A failed match is final.
3. Parse other inputs through `net/url`. Preserve the existing network-scheme set: `https`, `http`, `ssh`, and `git`.
   Reject URL user information, queries, fragments, malformed URLs, and missing hosts.
   Raw whitespace, controls, backslashes, and empty query/fragment suffixes fail in step 1.
   Encoded path segments retain the existing URL path behavior.
4. Reject unsupported schemes and local/file/path locators.

This leaves URL path normalization with the existing URL path.
It adds no public-host allowlist and no credential or reachability probe.
Raw query/fragment and whitespace rejection closes ambiguous spellings rather than interpreting them as clone-path content.
The conventional SCP subset remains ASCII-restricted.
The [registration repair validator](remote-repository-registration.md#identity-validation) applies the narrower HTTPS
candidate rules: supported percent-encoded spaces and Unicode segments are allowed, while encoded authority or path
separators, encoded controls, repeated separators, and traversal after decoding are rejected.

Keep request errors bounded (`invalid repository locator`) and keep locators out of responses and new logs.
Destination validation and Git's argument separator remain unchanged.
`cloneMaterializedRepository` still uses `clone --no-checkout -- <locator> <destination>` through classified subprocess admission.

## Launch and rollback

`remoteWorkspaceProjectionFromLaunch` omits the primary when the executor prepare script owns it.
Plugin remote execution includes the primary. `reconcileLaunchWorkspace` invokes materialization for the required projection before agent startup.
The repair changes admission, not this selection or ordering.

Removal invokes the same remote validator. Its existing absolute/file-origin recognition remains cleanup-only.
`removeMaterializedRepository` must still capture the owned directory, validate Git metadata and exact origin, then remove that captured directory.
Do not convert locators before origin comparison.

`rollbackMaterializedWorkspaceRepositories` retains reverse order, its bounded uncancelled cleanup context, and error aggregation.
Only responses with `Reused == false` enter rollback ownership.
A later clone or rescan failure cannot remove a reused primary or secondary checkout.

## Verification

Pure grammar cases cover accepted hosts/namespaces and hostile forms.
Registered handler tests cover rejection before mutation, real SCP materialization, exact-origin removal, and rollback ownership.
Use temporary Git configuration with `url.<temporary-origin>.insteadOf` for unit fixtures.
The HTTP request must retain its SCP locator. Never relax the production validator for test filesystem paths.

A backend integration crosses lifecycle projection, the real HTTP client, and real agentctl routes.
Its two-repository fixture proves secondary preparation and cleanup after a later failure.
A separate disposable Kubernetes smoke uses the existing Kind fixtures and a credential-free test SSH Git server.
The smoke proves remote execution, not just local Git rewriting. Its absence must remain explicit in results.

## Related decisions

- [Provider-origin identity](../../../decisions/2026-07-20-repository-provider-origin-identity.md)
- [Provider-neutral remote repositories](../../../decisions/2026-07-20-provider-neutral-remote-repositories.md)

## Implementation plans

- [Git repository locator repair](../../../plans/git-repository-locator-repair/plan.md)
