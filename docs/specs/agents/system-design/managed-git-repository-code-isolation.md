---
status: draft
system: agents
created: 2026-10-25
requirements:
  - REQ-AGENTS-GITISOLATION-001
  - REQ-AGENTS-GITISOLATION-002
  - REQ-AGENTS-GITISOLATION-003
---

# Managed Git repository-code isolation system design

## Purpose and ownership

Agents owns the policy. `internal/common/subproc` owns the one place every managed Git process is built; the repository model in `internal/task` owns the stored setting; `internal/agentctl` and `internal/agent/runtime` carry the setting to the execution host.
It generalizes the pins in `internal/worktree/submodule.go`, `internal/worktree/preservation.go`, and `internal/common/securityutil/git.go`.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-GITISOLATION-001 | Policy seam, Policy content, Driver enumeration, LFS objects on push, Agent boundary, Propagation to agentctl (capability) |
| REQ-AGENTS-GITISOLATION-002 | Hooks decision, Persistence and API, Propagation to agentctl |
| REQ-AGENTS-GITISOLATION-003 | Settings control, Public documentation |

## Policy seam

`subproc.NewGitCommand(ctx, args...)` stays the only production construction site; `raw_git_audit_test.go` fails the build when any other package constructs or looks up Git. It returns `*exec.Cmd` with no error and callers set `Dir` and `Env` afterwards, so it cannot apply the policy, which is applied in `startManagedGit` (`subproc/git_lifecycle.go`), which every classified `RunGit*` helper and streaming lifecycle reaches after `PrepareGitCommand`, with final `Dir`, `Env`, and context. There the seam rewrites `cmd.Args` once, before `cmd.Start`, and returns an error through the existing start path when the policy cannot be built (AC-001.8, AC-001.9).
The seam rewrites only a Git command (`cmd.Args[0]` is `git`, `cmd.Path` is what `GitExecutablePath` returns); any other command passes through unmodified.
`subproc.GitExecutablePath` returns only a path, so carries no policy. Its one user, `internal/task/gitinit/command_descriptor.go`, runs the Kandev binary as a helper through `RunGitCombinedOutputClass` (the caller holds the lifecycle slot); the helper `Fchdir`s into an inherited directory descriptor and `exec`s Git. The starter passes that command through unmodified and the helper applies the policy: after `Fchdir` it calls the exported `subproc.ManagedGitArgs(ctx, env, gitArgs) ([]string, error)`, which enumerates in the now-current directory through the non-acquiring start path and returns the full vector. On error the helper writes it to standard error and exits 1, which the caller reports from the combined output (AC-001.8, AC-001.9). A test pins that `startManagedGit` leaves the helper argv unchanged and that a planted filter and hook do not run in the initial commit; that caller never carries the hooks mark.

The rewritten vector is:

```text
git <caller global options, minus policy-owned keys> <policy -c pairs> <subcommand> <policy subcommand options> <rest>
```

Policy pairs follow the caller's global options because Git applies the last value (the recognized options are listed under Resolved details). A caller `-c`, `-ckey=value`, or `--config-env` whose key is policy-owned (section and variable matched case-insensitively) is dropped. Command-line `-c` outranks repository, global, and system files and `GIT_CONFIG_*` entries, and reaches child Git through `GIT_CONFIG_PARAMETERS`.
The pins for `core.hooksPath`, `core.fsmonitor`, and `protocol.ext.allow` in `submodule.go`, `local_repository_initialization.go`, and `preservation.go` become redundant and are removed. The `protocol.allow`, `protocol.https.allow`, and `protocol.ssh.allow` pins in `submodule.go` are not policy-owned and stay (AC-001.14).

The hooks decision travels on the context: `subproc.WithGitHooks(ctx, enabled bool)` stores an explicit value that replaces any inherited one, so each submodule caller or repository loop iteration sets its own. A context without a value is hardened, the safe default for a new caller.

## Policy content

Always applied, regardless of the hooks decision:

| Key or option | Value | Closes |
| --- | --- | --- |
| `core.fsmonitor` | `false` | AC-001.2 |
| `protocol.ext.allow` | `never` | AC-001.4 |
| `--no-textconv`, `--no-ext-diff` | inserted after the subcommand for `diff`, `log`, `show`, `format-patch`, `diff-tree`, `diff-index`, `diff-files`, and `range-diff`; `GIT_EXTERNAL_DIFF` removed from the child environment | AC-001.3 |
| `filter.<name>.clean`, `.smudge`, `.process` empty and `.required` false | every enumerated name that is not an allowlisted filter | AC-001.5 |
| `merge.<name>.driver` empty | every enumerated merge name; Git treats the empty command as a failed driver, so a merge that invokes the driver for a file stops with a conflict that keeps the current branch's content and no program starts, while a fast-forward or one-sided merge invokes none (the `merge` outcome is pinned by `TestManagedGitMergeDriverIsNeutralized`; the other driver-invoking commands are covered by the content-writing vector tests) | AC-001.6 |

`diff.<name>.textconv` and `diff.<name>.command` are not cleared (an empty textconv makes Git fail on commands lacking the option); `--no-textconv` and `--no-ext-diff` already stop both. Commands that apply text conversion without an option to turn it off (`blame`) are refused, as is any caller argument `--textconv`, `--filters`, or `--ext-diff` (AC-001.3). A Git-crypt-encrypted file therefore shows as a binary change in managed diff output; the guide says so (AC-001.3, AC-003.4).

Applied unless the context carries an enabled hooks decision: `core.hooksPath` set to `os.DevNull`, which resolves to a path with no executable entries on every supported platform. The same decision also governs configuration-defined hooks: every enumerated `hook.<name>` entry gets `hook.<name>.enabled=false`, and a name containing `=` fails the command like a driver name (AC-001.1, AC-001.8). The one exception is `push` in a repository where an allowlisted `lfs` filter is configured: its `core.hooksPath` is the Kandev-owned LFS hook directory (see LFS objects on push). `core.hooksPath=os.DevNull` also overrides a user-global or system `core.hooksPath` or template hooks, which the copy states (AC-003.1).

`-c` splits at the first `=`, so a driver name containing `=` cannot be neutralized. Enumeration fails the command on any such filter or merge name (AC-001.8); a test plants one and expects refusal and no program start.
Applied only when the context carries an enabled hooks decision: `submodule.recurse=false`, `fetch.recurseSubmodules=no`, and `push.recurseSubmodules=no`, because Git forwards these to submodule processes whose hooks and filters the parent-only determination never saw (AC-002.5). Without the hooks mark, `core.hooksPath` is inherited by those child processes and no submodule hook can run.

In a hooks-enabled repository a caller subcommand argument `--recurse-submodules`, or `--recurse-submodules=<value>` with a value other than `no`, is refused with an error naming it, because it would override the recursion pins; `--no-recurse-submodules` passes (AC-002.5).

Policy-owned keys are exactly the rows above, `core.hooksPath`, and the three recursion keys. `GIT_CONFIG_NOSYSTEM` is not set: system configuration may define the Git LFS filter and credential helpers.

Submodule commands never carry the hooks mark: the submodule tracker and `submodule.go` callers set `WithGitHooks(ctx, false)` (AC-002.5).

## Driver enumeration

Drivers can only be neutralized by name. Immediately before `cmd.Start`, inside the Git slot the caller already holds, `startManagedGit` runs one enumeration process through an internal start path that does not acquire a slot again (so it cannot deadlock on the throttle) and does not enumerate itself. It uses the final `Dir`, `Env`, the caller's repository selection options (`-C`, `--git-dir`, `--work-tree`, `--namespace`), and every caller `-c`, `-cKEY=value`, and `--config-env` entry that survives policy-owned-key stripping, so a driver the caller defines on the command line is enumerated and neutralized too (AC-001.9, AC-001.11), with the always-applied pairs:

```text
git <repository selection options> <surviving caller config options> <always-applied pairs> config --null --get-regexp ^((filter|merge)\..*\.(clean|smudge|process|required|driver|name|recursive)|hook\..*\.(command|event|enabled))$
```

It reads key and value pairs from that one process and parses names (`filter.<name>`, `merge.<name>`, `hook.<name>`; a name may contain dots, so the property is the last dot-separated component), deduplicates them, sorts them lexically within each family, and appends the neutralizing pairs to the vector. The enumeration reads included files, worktree configuration, and `GIT_CONFIG_*` entries as the command itself would. Limits: 5 seconds, 1 MiB of output, 256 characters per name. For `clone` the destination does not exist yet: enumeration runs in the command's `Dir` and sees global, system, and caller entries, the only configuration the new clone gets before its content is checked out. For `worktree add` the new worktree shares the source repository's configuration, which enumeration reads.

Canonical command forms (each tokenized as a shell word list; `%f` is literal):

| Driver name | clean | smudge | process |
| --- | --- | --- | --- |
| `lfs` | `git-lfs clean -- %f` or `git-lfs clean %f` | `git-lfs smudge -- %f`, `git-lfs smudge --skip -- %f`, `git-lfs smudge %f`, or `git-lfs smudge --skip %f` | `git-lfs filter-process` or `git-lfs filter-process --skip` |
| `git-crypt` | `git-crypt clean` | `git-crypt smudge` | not set |
| `git-crypt-<key>` | `git-crypt clean --key-name=<key>` | `git-crypt smudge --key-name=<key>` | not set |

For each filter name the seam takes the effective `clean`, `smudge`, `process`, and `required` values from that same output and decides whether the filter is allowlisted: the name is `lfs`, `git-crypt`, or `git-crypt-<key>`, and each configured command, tokenized as a shell word list, matches one canonical form in the requirements Terms (`--key-name=<key>` must equal the driver's suffix). The first word is the bare tool name or an absolute path resolving to the same file as `exec.LookPath(tool)` in the Kandev process. A required filter that is not allowlisted turns a content-writing command into a refusal naming the filter; a filter-reading command neutralizes it instead (AC-001.5). `worktree/gitcrypt.go` configuration is canonical; its locked-repository `cat` smudge override is neutralized.

Enumeration is default-deny by subcommand. Subcommands fall into the three requirements Terms lists. A filter-free command (no ref or index update, so no hook can start) never enumerates (AC-001.13). A filter-reading command enumerates and neutralizes every non-allowlisted driver, required or not, and on a hooks-off repository every configuration-defined hook. A content-writing command enumerates, and a required non-allowlisted filter refuses it. A subcommand on none of the lists enumerates and counts as content-writing, and a build-time test (`git_subcommand_classification_test.go`) collects every literal subcommand passed directly to `NewGitCommand` (the `RunGit*` helpers are not scanned) and fails when one is on no list (AC-001.15).
A non-zero exit other than "no matching keys" (exit 1 with empty output), a timeout, an output over the limit, or an unparsable name fails the managed command before it starts (AC-001.8). A failed background poll reports through its existing status error path. A refusal of any other background content-writing command (turn-checkpoint capture, launch-time `worktree add`, initial commit) is that command's error, naming the filter; the seam adds no retry, fallback, or notice.
Cost is one extra lightweight process per managed command that is not filter-free (the poll runs every 3 seconds). No result is memoized, because a memo would let a configuration change take effect late. The first work order records the measured median and 95th percentile added poll latency as a receipt, not a gate.

## LFS objects on push

With hooks off, Git LFS's own `pre-push` hook does not run, so LFS objects would not be uploaded. Rather than orchestrating `git lfs push` at each push site, the seam hands Git a Kandev-owned hooks directory for `push` only. Git then supplies the ref list on standard input and the remote arguments exactly as it does for any `pre-push` hook, so every refspec, force option, upstream option, and deletion is handled by Git LFS itself and the policy needs no refspec parsing (AC-001.7).

- The directory (see Resolved details) holds one `pre-push` file whose fixed content is `#!/bin/sh` followed by `exec '<absolute path of git-lfs matched by the allowlist>' pre-push "$@"`, single-quote escaped. It is selected only for `push`, only when the context carries no enabled hooks decision, and only when enumeration found an allowlisted `lfs` filter. `.gitattributes` is not consulted: where LFS is installed but unused the step is a cheap no-op.
- A missing `git-lfs` binary makes the `lfs` filter non-allowlisted (AC-001.5); there is no separate path.
- A failed upload exits non-zero, Git aborts the push before sending refs, and the error carries Git LFS's output.
- In a hooks-enabled repository the repository's own hooks run (including a Git LFS-installed `pre-push`), and the Kandev-owned directory is not used.

## Agent boundary

The policy is carried in the argument vector, never the process environment, so `AgentEnv` does not contain it (AC-001.10). `GIT_EXTERNAL_DIFF` is removed only from the managed child's environment copy. The seam rewrites only Git that Kandev starts directly; the `git` that `gh`, `glab`, and `az` start (`GitOperator.CreatePR`) is Out of scope, and no environment variable is injected for it (AC-001.10).

## Hooks decision

| Caller | Source of the mark |
| --- | --- |
| Backend worktree manager, repository services, `repoclone` (repository known by ID) | Repository record read at the point the command context is built |
| agentctl `GitOperator` and `WorkspaceTracker` | `Manager` map `GitHooks[repositoryName]`; key `""` is the workspace root, used by a single-repository workspace |
| Anything else (no repository resolvable), submodule commands | explicit `WithGitHooks(ctx, false)` or no value |

Lookup is exact and case-sensitive and never falls back to the `""` entry, unlike `lookupBaseBranch` (AC-002.4).

The map is built in the task service by a new `collectTaskGitHooks`, next to `collectTaskBaseBranches` and over the same branch-identity computation (`BuildBranchIdentityPlans`, `taskRepositoryWorkspaceTrackerKey`), so every path shares one key source. It does not skip a repository with an empty base branch, and `""` holds only the value of the repository whose checkout is the workspace root. By executor shape: a single-repository task writes `""` with that repository's value (non-remote executors also write the `baseBranchMetadataKey` form for it); on a remote executor the first repository writes `""` and later ones their materialized directory names; a non-remote executor with several repositories writes no `""` key.

## Persistence and API

`repositories.run_git_hooks` is `INTEGER NOT NULL DEFAULT 0` (0 or 1) in both SQLite and PostgreSQL, matching `pull_before_worktree`, added through the store's migration mechanism (one store package serves both dialects) with the upgrade fixtures under `internal/persistence/storeconformance/testdata/upgrades` extended for both. Existing rows read `false` (AC-002.1).
`models.Repository.RunGitHooks`, the response DTO in `internal/task/dto/dto.go` (`RepositoryDTO`, `FromRepository`), the service request structs in `internal/task/dto/requests.go`, and the REST request bodies in `internal/task/handlers/repository_handlers.go` (create and update bodies, beside `pull_before_worktree`) carry it as `run_git_hooks`; the update field is a `*bool`, so omission leaves the stored value, as for `pull_before_worktree`. In the store, `run_git_hooks` joins the intent-gated, `RETURNING`-projected path of `repositoryUpdateQuery` (`repository_checkout_defaults.go`) through `RepositoryCheckoutIntent`: written only when the intent carries it, never in the fixed column list, so a save that omits it cannot rewrite another client's change. Full-model writes (`store.UpdateRepository`, the exact-match `UpdateRepositoryIfUnchanged`, and any read-modify-write such as the one in `orchestrator/event_handlers_github.go`) never write the column: a stale read cannot turn hooks back on; their response and `repository.updated` event carry the stored value, re-read after the write (AC-002.7). A test races an omitting save with a setting save, and a full-model write with a REST toggle; the set value must remain in both. The WebSocket `repository.update` handler (`wsUpdateRepository`, request struct `wsUpdateRepositoryRequest`) does not declare the field, so it is ignored there; a test sends it and expects the stored value unchanged (AC-002.7).
`taskservice.UpdateRepositoryRequest` (`internal/task/service/service_requests.go`) is decoded by `decodePatch` with `DisallowUnknownFields` in `internal/backendapp/settings_domain_operations.go`, which is the MCP `update_settings` path. The hooks field on that service request carries the tag `json:"-"` and is set only by the REST handler after authentication, so `update_settings` with a `run_git_hooks` key fails as an unknown field (AC-002.7). `settingscatalog/defaults.go` lists `run_git_hooks` as a read-only descriptor, not through the writable repository list that holds `pull_before_worktree`. A test patches `run_git_hooks` through `update_settings` and expects an error and an unchanged stored value.
Clone (`workspace_clone.go`) copies the column as stored at clone time; that initialization is not a change under AC-002.7 (AC-002.8); export (`export_lookup.go`) omits it and import leaves the default (AC-002.9); the plugin host API (`host_exact_workspace.go`) does not expose it (AC-002.7).

## Propagation to agentctl

The setting reaches agentctl the way `base_branches` does at creation, read once at the last point before the create call (AC-002.6):

1. `(m *Manager) seedExecutionGitHooks(ctx, taskID, executorType, metadata)`, beside `seedExecutionBaseBranches`, is the only code that sets `MetadataKeyGitHooks` (`map[string]bool`, workspace-entry key to value; constant in `executor_backend.go`). It writes the metadata of the `ExecutorCreateRequest` as the last metadata step before the Manager's `CreateInstance` call, on both Manager paths that make one: launch (`manager_launch.go`) and on-demand or recovery (`prepareExecutionCreateRequest`). Docker, SSH, Sprites, and reconnect creates run under that call; each request builder only copies the key into the wire request, as for `base_branches` (`executor_standalone.go`, `buildContainerCreateInstanceRequest`, `executor_ssh_operations.go`, `executor_sprites_operations.go`, `executor_docker.go`). "The create call" in AC-002.6 is the Manager's call: the seed reads once at its start, and later preparation (image pull, SSH or Sprites setup, capability check) never re-reads. The seed first deletes any `git_hooks` already in the metadata (a retried request keeps no older value), then sets nothing for `k8s` and plugin-remote executors and otherwise the provider's map. `GitHooksProvider` is `func(ctx, taskID) (map[string]bool, error)`, wired to the task service beside `BaseBranchProvider`, bounded at 5 seconds, returning `collectTaskGitHooks` over stored repository records read at call time. An unwired provider, a nil map, an error (a partial map is discarded), or a timeout leaves no key (hooks off) and logs a warning; an empty non-nil map, or a key the map lacks, is off with no warning. `collectTaskGitHooks` returns an empty non-nil map for a task with no repositories, and the seed skips the provider for an empty `taskID` (no key, no warning). A key-derivation error for one repository turns that key off with a warning and keeps the others. `git_hooks` is not in `FilterPersistentMetadata`, so an older value cannot reach a new instance. Tests: no `git_hooks` in a Kubernetes launch, reconnect, and on-demand request, and one in a Docker request for a hooks-enabled repository (AC-002.2); a change made after the metadata was prepared and before the seed read; a retried request with a stale key; and the keys equal the launch path's base-branch keys for single-repository, multi-repository non-remote, and remote shapes.
2. The client wire request `CreateInstanceRequest` (`agent/runtime/agentctl/control.go`) and the agentctl create request carry `git_hooks` (`instance.go`), copied into `config.InstanceOverrides` and `cfg.GitHooks` (`config.go`, `instance/manager.go`). Each hand-off copies the map, so a caller that keeps a reference cannot change the creation-time value.
3. `process.Manager` exposes a copy-on-read accessor; trackers and operators receive the decision when they build each command context.

There is no runtime endpoint that changes a repository's value: agentctl is reachable from the agent's network namespace in several executors, and an endpoint that enables hooks would let the agent enable them (AC-002.7). The value an agentctl instance holds is fixed at creation; a change reaches agentctl only through instances created afterward, and **Reset Environment** is the user action that creates one (AC-002.6). Lowering a running instance is Out of scope.

Capability: agentctl adds `managed-git-policy.v1` to `SurvivalCapabilities` in `internal/agentctl/server/api/identity.go`, advertised on the existing `GET /identity`. `executionCapabilities()` appends it outside the Linux-only `sandbox-launch.v1` branch. Before `(*ControlClient).CreateInstance` (`internal/agent/runtime/agentctl/control.go`) honours any create request, host-utility probe instances included (one extra `/identity` call), it performs its own `/identity` subset check (not the launch-wrapper helper, whose errors differ) with a 5 second bound. A missing capability returns the error naming the missing managed Git policy; a timeout, transport error, non-success status, or malformed body returns an error naming the failed verification with its cause; a context cancelled or past its deadline returns that context error (only the 5 second bound is a failed verification); none is a launch-wrapper error (AC-001.12).
Remote and container adoption keeps this rule: `RequiredSurvivalCapabilities` stays `agent-survival.v1` (only `adoptSurvivingAgentctl` reads it), so a pre-change server, container, or host is still reconnected and then fails closed for new instances. The SSH, Sprites, and Kubernetes launch fails with the AC-001.12 error until a current agentctl runs there (upgrade and restart, or Reset Environment). Docker and remote_docker reconnect handling is unchanged: a failed reconnect create is handled as today (see Out of scope). The exception is the backend's own local server: `adoptSurvivingAgentctl` (`backendapp/agentctl.go`) passes `RequiredSurvivalCapabilities` plus `managed-git-policy.v1` to `AttemptAdoptControlServer`, so a surviving pre-change local server takes the existing `capability_incompatible` path: authenticated, stopped, its live standalone records are repaired (its agents are interrupted), and a fresh server starts. Test: `TestAdoptSurvivingAgentctlRefusesServerWithoutManagedGitPolicy`. Remote instances created before this change, and retained containers whose instance still exists (reused without a create), run Git without the policy until reset; the guide says so (AC-003.4).

## Concurrency, idempotency, and ordering

- Concurrent managed commands compute their own vectors; only the read-only map copy is shared.
- Instance creation is rejected for a duplicate instance ID as today, so a changed map on a retried create is not applied; a new value therefore needs a new instance.
- The vector is deterministic: policy pairs sorted by full key; enumerated names deduplicated and sorted lexically within each family.
- A setting change racing a command: the command uses the value captured when its context was built. A change racing an instance create: the seed's read decides, and a later change applies to later instances only (AC-002.6).
- A concurrent writer to the repository configuration can add a driver between enumeration and the command (AC-001.11). Enumeration runs just before `cmd.Start` inside the held slot, keeping the window small; no current specification closes it, and [sandbox policies](sandbox-policies.md) make no claim about the shared Git directory.

## Failure and recovery

Enumeration failure, a missing Git executable, an undecidable allowlist, and a context cancelled during enumeration fail the command; none falls back to an unhardened run. A failure to create the LFS hooks directory fails the push with an error naming it and never falls back to the null device. A persistent enumeration failure fails every status poll through the existing status error path. A repository record the backend cannot read gives hooks off for that command and a warning. The `--no-verify` refusal on a non-dry-run push is a design rule with its own test. A hook failure surfaces through the existing Git error path, which carries stderr, and a hook that does not exit stops when the command's context ends. Standard input is the null device; a hook receives the input Git supplies for its type, and the seam replaces none (AC-002.2).

## Resolved details


- **`--no-verify` and dry runs on push (AC-001.7, 001.9).** A `push` carrying `--dry-run` (the existing preflight is `push --dry-run --no-verify --porcelain`) never receives the Kandev hooks directory: its `core.hooksPath` is the null device, so it uploads nothing. Otherwise, with the Kandev hooks directory in force, a managed `push` carrying `--no-verify` is refused with an error naming the option. `-n` is Git's short form of `--dry-run`, not of `--no-verify`, and is not refused.
- **LFS hooks directory (AC-001.7).** One subdirectory per distinct resolved `git-lfs` path, named by the first 16 hex characters of its SHA-256, under the process's private `0700` directory. Before every use, under a process-wide mutex, the seam checks that `<subdirectory>/pre-push` exists and is executable; if not, it builds the subdirectory under a temporary name (with `pre-push` written, `chmod 0700`) and renames the whole directory into place, so a visible hooks directory is never empty.
- **Filter definitions (AC-001.5, 001.8).** The effective value of a key is its last occurrence (Git's rule); an empty value is unset. A filter is allowlisted when its name is allowlisted, at least one of `clean`, `smudge`, `process` is set, and every set one is canonical; otherwise it is not (and if none is set and `required` is true, it is a required non-allowlisted filter). `required` follows Git's boolean rules (a key with no value is true); any other value fails the command under AC-001.8.
- **Word-list grammar (AC-001.5, 001.7, 001.11).** One tokenizer on every platform: spaces and tabs separate words; single quotes are literal; in double quotes only backslash before `"` or `\` escapes; outside quotes a backslash escapes the next character. A shell metacharacter (dollar, backtick, semicolon, ampersand, pipe, angle bracket, parenthesis, glob or brace character, tilde, hash, exclamation mark) anywhere outside single quotes, including inside double quotes where a shell still expands dollar and backtick, a newline, an unbalanced quote, or an empty command is not canonical. A Windows path with unquoted backslashes is therefore not allowlisted (AC-001.5); the guide lists this.
- **Tool resolution (AC-001.9, 001.11).** A bare tool name is allowlisted only when resolving it with the managed child's `Env` `PATH` and with Kandev's own `PATH` give the same file after symbolic links, so a caller-supplied `PATH` cannot select another binary. The LFS hook script uses the absolute path from Kandev's own lookup.
- **Global options (AC-001.9, 001.11).** Recognized leading options: `-c`, `-ckey=value`, `--config-env`, `-C`, `--git-dir`, `--work-tree`, `--namespace` (each with its argument) and the flags `--no-pager`, `-P`, `--no-optional-locks`, `--no-replace-objects`, `--literal-pathspecs`, `--glob-pathspecs`, `--noglob-pathspecs`, `--icase-pathspecs`, `--bare`, `--no-lazy-fetch`. Any other option before the subcommand (`--exec-path`, `-p`, `--paginate`, `--super-prefix`, or an option missing its argument) is refused with an error naming it.
- **Classification (AC-001.15).** The guarantee is the run-time rule: the subcommand is the first argument after the recognized global options (the same parser as the vector); `worktree`, `branch` and `cat-file` are classified by their next word or options, and any unrecognised form (for example `worktree move`, `branch --list --no-list`) is content-writing. The build-time test `git_subcommand_classification_test.go` checks an explicit file inventory: each direct `NewGitCommand` call whose first non-option argument is a literal must name a listed subcommand, and a call that spreads arguments is accepted only in a file named in the inventory.
- **Explicit `null` (AC-002.7).** A JSON `null` for `run_git_hooks` decodes to a nil pointer and is treated as omitted.
- **Settings copy (AC-003.1, 003.4).** Activation wording: a change applies to agent instances created afterward; a running instance keeps its value, and a new session that reuses an instance keeps it too. Filters: Git LFS and git-crypt work only in standard configuration with the tool installed.

## Settings control

`apps/web/components/settings/repository-card.tsx` gains a `Switch` row beside the existing `pull_before_worktree` control, wired through `workspace-repositories-client.tsx`, `workspace-repositories-save.ts`, `workspace-repositories-dirty.ts`, and `app/actions/workspaces.ts` (`run_git_hooks`).
Layout: desktop shares the pull-before-worktree grid; phone is a full-width row with help text below and a touch target of at least 44 px. Copy uses `t()` in all seven locales (Traditional Chinese via `pnpm run i18n:zh-hant`), with no em dash.
A failed save restores the stored value through the existing dirty-state reset for the single field. A switch change made while a save is in flight stays an unsaved edit after the response arrives (the response is applied only to fields without a newer local edit), and the server applies the last write it receives (AC-003.3). The help text also says that a change applies to agent instances created afterward and not to running ones (Reset Environment starts a new one). It covers the other AC-003.1 points.

```text
UI-01: Repository card, Run Git hooks (desktop and phone share one composition)
+----------------------------------------------------------+
| my-repo                                                  |
| ...                                                      |
| Pull before creating worktree                    [ off ] |
| Run Git hooks                                    [ off ] |
| Help text per AC-003.1: permissions, who can change      |
| hooks, activation, global hooks, submodules, filters.    |
+----------------------------------------------------------+
```

The switch sits beside its label with the help text directly below, default off; spacing and wording are illustrative.

## Public documentation

`docs/public/git-operations.md` gets a section under "Everyday operations" covering AC-003.4, and the "Troubleshooting" section gains the symptom "my pre-commit hook did not run on a Changes-panel commit". It also says that remote environments started before this change run Git without the policy until Reset Environment, which also applies a changed setting, and that a surviving local agentctl that predates this change is replaced at the next start. `docs/public/meta.json` and the coverage manifest change only if the section list requires it.

## Related decisions

None recorded.
