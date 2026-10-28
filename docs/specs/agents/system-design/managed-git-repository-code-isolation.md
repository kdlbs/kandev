---
status: draft
system: agents
created: 2026-10-25
requirements:
  - REQ-AGENTS-GITISOLATION-001
  - REQ-AGENTS-GITISOLATION-002
  - REQ-AGENTS-GITISOLATION-003
  - REQ-AGENTS-GITISOLATION-004
---

# Managed Git repository-code isolation system design

## Purpose and ownership

Agents owns the policy. `internal/common/subproc` owns the one place every managed Git process is built; the repository model in `internal/task` owns the stored setting; `internal/agentctl` and `internal/agent/runtime` carry the setting to the execution host.
The design generalizes patterns already in the tree: `internal/worktree/submodule.go` (`core.hooksPath` and `protocol.ext.allow`), `internal/worktree/preservation.go` (`core.fsmonitor=false`), and `internal/common/securityutil/git.go` (`--no-ext-diff --no-textconv`).

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-GITISOLATION-001 | Policy seam, Policy content, Driver enumeration, LFS objects on push, Agent boundary, Propagation to agentctl (capability) |
| REQ-AGENTS-GITISOLATION-002 | Hooks decision, Persistence and API, Propagation to agentctl |
| REQ-AGENTS-GITISOLATION-003 | Settings control, Public documentation |

## Policy seam

Every behavior below is the flag-on behavior. `startManagedGit` (through `applyManagedGitPolicy`), `ManagedGitArgs`, and `ManagedGitEnv` first test the process-level mode (see the [rollout design](managed-git-isolation-rollout.md)) and, when it is off, return the caller's command, arguments, and environment untouched, before any enumeration (AC-AGENTS-GITISOLATION-004.2).


`subproc.NewGitCommand(ctx, args...)` stays the only production construction site; `raw_git_audit_test.go` fails the build when any other package constructs or looks up Git. It returns `*exec.Cmd` with no error and callers set `Dir` and `Env` after it returns, so it cannot apply or fail the policy.
The policy is therefore applied in `startManagedGit` (`subproc/git_lifecycle.go`), which every classified `RunGit*` helper and streaming lifecycle reaches after `PrepareGitCommand`, with final `Dir`, `Env`, and context. There the seam rewrites `cmd.Args` once, before `cmd.Start`, and returns an error through the existing start path when the policy cannot be built (AC-001.8, AC-001.9).
The seam rewrites only a Git command: `cmd.Args[0]` is `git` and `cmd.Path` is the executable `GitExecutablePath` returns. Any other command passes through `startManagedGit` unmodified.
`subproc.GitExecutablePath` returns only a path, so it cannot carry policy. Its one user, `internal/task/gitinit/command_descriptor.go`, builds a command whose executable is the Kandev binary (argv `helperArgument, gitPath, git args...`, with the target directory in an inherited descriptor), runs it through `RunGitCombinedOutputClass` (so the caller already holds the lifecycle slot), and the helper `Fchdir`s into the descriptor and then `exec`s Git. The process starter therefore passes that command through unmodified, and the helper applies the policy itself: after `Fchdir` it calls the exported `subproc.ManagedGitArgs(ctx, env, gitArgs) ([]string, error)`, which enumerates in the now-current directory through the non-acquiring start path (the parent's slot covers the helper and its enumeration) and returns the full vector. On error the helper writes the error to standard error and exits 1, which the caller reports from the combined output (AC-001.8, AC-001.9). A test pins that the helper argv is unchanged by `startManagedGit` and that a planted filter and hook do not run during the initial commit. That caller never carries the hooks mark.

The rewritten vector is:

```text
git <caller global options, minus policy-owned keys> <policy -c pairs> <subcommand> <policy subcommand options> <rest>
```

Policy pairs follow the caller's global options because Git applies the last value (the recognized options are listed under Resolved details). A caller `-c`, `-ckey=value`, or `--config-env` whose key is policy-owned (section and variable matched case-insensitively) is dropped. Policy pairs are sorted by full key name so the vector is deterministic. Command-line `-c` outranks repository, global, and system files and `GIT_CONFIG_*` entries; Git hands it to child Git processes through `GIT_CONFIG_PARAMETERS`.
Existing pins for `core.hooksPath`, `core.fsmonitor`, and `protocol.ext.allow` in `submodule.go`, `local_repository_initialization.go`, and `preservation.go` stay in place: they are redundant with the policy when the flag is on and are the existing behavior when it is off (AC-AGENTS-GITISOLATION-004.2). The `protocol.allow`, `protocol.https.allow`, and `protocol.ssh.allow` pins in `submodule.go` are not policy-owned and stay (AC-001.14).

The hooks decision travels on the context: `subproc.WithGitHooks(ctx, enabled bool)` stores an explicit value that replaces any inherited one, so each submodule caller or repository loop iteration sets its own. A context without a value is hardened, so the default is the safe value for any new caller.

## Policy content

Always applied, regardless of the hooks decision:

| Key or option | Value | Closes |
| --- | --- | --- |
| `core.fsmonitor` | `false` | AC-001.2 |
| `protocol.ext.allow` | `never` | AC-001.4 |
| `--no-textconv`, `--no-ext-diff` | inserted after the subcommand for `diff`, `log`, `show`, `format-patch`, `diff-tree`, `diff-index`, `diff-files`, and `range-diff`; `GIT_EXTERNAL_DIFF` removed from the child environment | AC-001.3 |
| `filter.<name>.clean`, `.smudge`, `.process` empty and `.required` false | every enumerated name that is not an allowlisted filter | AC-001.5 |
| `merge.<name>.driver` empty | every enumerated merge name | AC-001.6 |

`diff.<name>.textconv` and `diff.<name>.command` are not cleared (an empty textconv makes Git fail on commands lacking the option); `--no-textconv` and `--no-ext-diff` already stop both. Commands that apply text conversion without an option to turn it off (`blame`) are refused, as is any caller argument `--textconv`, `--filters`, or `--ext-diff` (AC-001.3). Kandev's managed callers use none of them today. As a consequence a Git-crypt-encrypted file shows as a binary change in managed diff output, because its `diff.git-crypt*.textconv` converter is not started; the guide says so (AC-001.3, AC-003.4).

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

For each filter name the seam takes the effective `clean`, `smudge`, `process`, and `required` values from that same output (no second process) and decides whether the filter is allowlisted: the name is `lfs`, `git-crypt`, or `git-crypt-<key>`, and each configured command, tokenized as a shell word list, matches one canonical form in the requirements Terms (including the Git LFS `--skip` and legacy forms, and `--key-name=<key>` for a named git-crypt key whose suffix equals `<key>`). The first word is the bare tool name or an absolute path whose symbolic links evaluate to the same file as `exec.LookPath(tool)` in the Kandev process, because git-crypt writes a quoted absolute path. A required filter that is not allowlisted turns a content-writing command into a refusal naming the filter; a filter-reading command neutralizes it instead (AC-001.5). The existing `worktree/gitcrypt.go` configuration is canonical; its `cat` smudge override for locked repositories is neutralized.

Enumeration is default-deny by subcommand. Subcommands fall into three lists defined in the requirements Terms. A filter-free command (a pure read or object write such as `rev-parse`, `ls-tree`, `commit-tree`, `worktree list|prune`; no ref or index update, so no hook can start) never enumerates (AC-001.13). A filter-reading command (`symbolic-ref`, `remote`, `fetch`, `update-ref`, `write-tree`, `status`, `diff` and the other diff-style commands, `log`, `show`, `format-patch`, `range-diff`, `ls-files`, `worktree remove`, `push`) enumerates and neutralizes every non-allowlisted driver, required or not, and on a hooks-off repository every configuration-defined hook. A content-writing command enumerates, and a required non-allowlisted filter refuses it. A subcommand on none of the lists enumerates and counts as content-writing, and a build-time test (`git_subcommand_classification_test.go`) collects every literal subcommand passed directly to `NewGitCommand` (the `RunGit*` helpers are not scanned) and fails when one is on no list (AC-001.15). A required non-allowlisted filter therefore does not break `push`, `fetch`, `worktree remove`, or cleanup, but refuses turn-checkpoint capture and other content-writing commands, naming the filter.
A non-zero exit other than "no matching keys" (exit 1 with empty output), a timeout, an output over the limit, or an unparsable name fails the managed command before it starts (AC-001.8). A failed background poll reports through its existing status error path. A refusal of any other background content-writing command (turn-checkpoint capture, launch-time `worktree add`, initial commit) is returned as that command's error through its existing error path, naming the filter; the seam adds no retry, fallback, or notice.
Cost is one extra lightweight process per managed command that is not filter-free (the poll runs every `DefaultGitPollInterval`, 3 seconds). No result is memoized, because a memo would let a configuration change take effect late. The first work order records the measured median and 95th percentile added poll latency as a receipt, not a gate; the follow-up if unacceptable is a memo keyed on a fingerprint of the configuration files, not a time-based one.
Drivers a concurrent writer adds between enumeration and the command are outside the guarantee (AC-001.11); nothing on main closes that window.

## LFS objects on push

With hooks off, Git LFS's own `pre-push` hook does not run, so LFS objects would not be uploaded. Rather than orchestrating `git lfs push` at each push site (each with its own refspec shape), the seam hands Git a Kandev-owned hooks directory for `push` only. Git then supplies the ref list on standard input and the remote arguments exactly as it does for any `pre-push` hook, so every refspec, force option, upstream option, and deletion is handled by Git LFS itself and the policy needs no refspec parsing (AC-001.7).

- The directory is created once per backend or agentctl process under a private temporary directory with mode `0700`, containing one file `pre-push` with mode `0700` and the fixed content `#!/bin/sh` followed by `exec '<absolute path of git-lfs matched by the allowlist>' pre-push "$@"`, with the path single-quote escaped. It is recreated if the directory is missing at use and removed at process exit. It is selected only for `push`, only when the context carries no enabled hooks decision, and only when enumeration found an allowlisted `lfs` filter configured. `.gitattributes` content is not consulted: where LFS is installed but unused the step is a cheap no-op. Git for Windows runs the file through its bundled shell.
- Because the allowlist requires a resolvable `git-lfs` executable, a missing binary makes the `lfs` filter non-allowlisted, which AC-001.5 already covers; no separate missing-binary path exists.
- A failed upload exits non-zero, Git aborts the push before sending refs, and the error carries Git LFS's output through the existing Git error path.
- In a hooks-enabled repository the repository's own hooks run (including a Git LFS-installed `pre-push`), and the Kandev-owned directory is not used.

## Agent boundary

The policy is carried in the argument vector, never the process environment, so `AgentEnv` does not contain it (AC-001.10). `GIT_EXTERNAL_DIFF` is removed only from the managed child's environment copy.

## Hooks decision

| Caller | Source of the mark |
| --- | --- |
| Backend worktree manager, repository services, `repoclone` (repository known by ID) | Repository record read at the point the command context is built |
| agentctl `GitOperator` and `WorkspaceTracker` | `Manager` map `GitHooks[repositoryName]`; key `""` is the workspace root, used by a single-repository workspace |
| Anything else (no repository resolvable), submodule commands | explicit `WithGitHooks(ctx, false)` or no value |

Lookup is exact and case-sensitive and never falls back to the `""` entry, unlike `lookupBaseBranch` (AC-002.4).

The map is built by a new `collectGitHooks(req)` in `manager_launch.go`, next to `collectBaseBranches`, from a key-derivation helper extracted from it (`workspaceEntryKey(req, index, spec)`), so both maps use one derivation. It differs from `collectBaseBranches` in that it does not skip a repository with an empty `BaseBranch`, never copies `req.BaseBranch` into `""`, and `""` holds only the value of the repository whose checkout is the workspace root. By executor shape: a single-repository task on any executor writes `""` with that repository's value (non-remote executors also write the `baseBranchMetadataKey` form for the same repository); on a remote executor the first repository writes `""` and later repositories write their materialized directory names; on a non-remote executor with more than one repository no `""` key is written. The plugin-remote executor writes no map (hooks off, AC-002.2). The map is a launch-time snapshot: a later change to base branches (`UpdateBaseBranches`, `PushBaseBranchesForTask`) rekeys nothing in it.

The agentctl map is read under the same lock discipline as `Manager.getBaseBranches` and copied on read.

## Persistence and API

`repositories.run_git_hooks` is `INTEGER NOT NULL DEFAULT 0` (0 or 1) in both SQLite and PostgreSQL, matching `pull_before_worktree`, added through the store's migration mechanism (one store package serves both dialects) with the upgrade fixtures under `internal/persistence/storeconformance/testdata/upgrades` extended for both. Existing rows read `false` (AC-002.1).
`models.Repository.RunGitHooks`, the response DTO in `internal/task/dto/dto.go` (`RepositoryDTO`, `FromRepository`), the service request structs in `internal/task/dto/requests.go`, and the REST request bodies in `internal/task/handlers/repository_handlers.go` (the create body near the `pull_before_worktree` field and the update body near it, which carry JSON tags there) carry it as `run_git_hooks`; the update field is a `*bool`, so omission leaves the stored value, as for `pull_before_worktree`. The WebSocket `repository.update` handler (`wsUpdateRepository`, request struct `wsUpdateRepositoryRequest`) does not declare the field, so it is ignored there, and a test sends it over that action and expects the stored value unchanged (AC-002.7).
`taskservice.UpdateRepositoryRequest` (`internal/task/service/service_requests.go`) is decoded by `decodePatch` with `DisallowUnknownFields` in `internal/backendapp/settings_domain_operations.go`, which is the MCP `update_settings` path. The hooks field on that service request carries the tag `json:"-"` and is set only by the REST handler after authentication, so `update_settings` with a `run_git_hooks` key fails as an unknown field (AC-002.7). `settingscatalog/defaults.go` lists `run_git_hooks` as a read-only descriptor, not through the writable repository list that holds `pull_before_worktree`. A test patches `run_git_hooks` through `update_settings` and expects an error and an unchanged stored value.
Workspace clone (`workspace_clone.go`) copies the column (AC-002.8); export (`export_lookup.go`) omits it and import leaves the default (AC-002.9); the plugin host API (`host_exact_workspace.go`) does not expose it (AC-002.7).

## Propagation to agentctl

The setting reaches agentctl the way `base_branches` does at creation:

1. The lifecycle layer builds `MetadataKeyGitHooks` (`map[string]bool`, workspace-entry key to value) in `manager_launch.go` next to `collectBaseBranches` (the constant lives in `executor_backend.go`), and passes `git_hooks` into the create request where `base_branches` is passed today: `executor_standalone.go`, `executor_docker.go`, `docker_launch.go` (which also serves the remote Docker executor, `executor_remote_docker.go`), `executor_ssh_operations.go`, and `executor_sprites_operations.go`. Kubernetes and plugin-remote executors do not carry `base_branches` today and do not carry `git_hooks`; their managed Git runs with hooks off.
2. The agentctl instance create request carries `git_hooks` (`instance.go`), copied into `config.Overrides` and `cfg.GitHooks` (`config.go`, `instance/manager.go`).
3. `process.Manager` exposes a copy-on-read accessor; trackers and operators receive the decision when they build each command context.

There is deliberately no runtime endpoint that changes a repository's value: agentctl is reachable from the agent's network namespace in several executors, and an endpoint that enables hooks would let the agent enable them (AC-002.7). The value an agentctl instance holds is fixed at creation; a change to the stored setting reaches agentctl only through instances created afterward (AC-002.6). Lowering a running instance is Out of scope.

Capability (checked only while the backend flag is on, AC-AGENTS-GITISOLATION-004.9): agentctl adds `managed-git-policy.v1` to `SurvivalCapabilities` in `internal/agentctl/server/api/identity.go`, advertised on the existing `GET /identity`. Before the create request of a new instance is honoured, the agentctl client performs the same `/identity` subset check `checkLaunchWrapperCapability` does, on every create request including host-utility probe instances; a missing capability, a timeout of 5 seconds, a transport error, a non-success status, or a malformed body fails the launch with an error that names the missing managed Git policy (AC-001.12). `RequiredSurvivalCapabilities` and the retained-instance adoption path are unchanged.

## Concurrency, idempotency, and ordering

- Concurrent managed commands each compute their own vector; shared state is the read-only map copy.
- Instance creation is rejected for a duplicate instance ID as today, so a changed map on a retried create is not applied; a new value therefore needs a new instance.
- The vector is deterministic: policy pairs sorted by full key; enumerated names deduplicated and sorted lexically within each family.
- A hooks setting change racing a command: the command uses the value captured when its context was built.
- A concurrent writer to the repository configuration can add a driver between enumeration and the command (AC-001.11). The enumeration runs immediately before `cmd.Start` inside the held slot to keep the window small; closing it needs write protection of the Git directory, which has no owner on main.

## Failure and recovery

Enumeration failure, a missing Git executable, an undecidable allowlist, and a context cancelled during enumeration fail the command; none falls back to an unhardened run. A hook failing in a hooks-enabled repository surfaces through the existing Git error path, which already carries stderr (`runGitCommandWithEnvironment`). A hook that does not exit stops when the command's context ends. The managed Git process's standard input is the null device; a hook receives the input Git itself supplies for its hook type (for example the ref list for `pre-push`), and no code in the seam replaces it (AC-002.2).

## Resolved details

Each rule closes a gap a builder would otherwise fill by guesswork.

- **`--no-verify` and dry runs on push (AC-001.7, 001.9).** A `push` carrying `--dry-run` (the existing preflight is `push --dry-run --no-verify --porcelain`) never receives the Kandev hooks directory: its `core.hooksPath` is the null device, so it uploads nothing. Otherwise, with the Kandev hooks directory in force, a managed `push` carrying `--no-verify` is refused with an error naming the option. `-n` is Git's short form of `--dry-run`, not of `--no-verify`, and is not refused.
- **LFS hooks directory (AC-001.7).** One subdirectory per distinct resolved `git-lfs` path, named by the first 16 hex characters of its SHA-256, under the process's private `0700` directory. Before every use, under a process-wide mutex, the seam checks that `<subdirectory>/pre-push` exists and is executable; if not, it builds the subdirectory under a temporary name (with `pre-push` written, `chmod 0700`) and renames the whole directory into place, so a visible hooks directory is never empty. Removal is at process exit. Concurrent pushes matching different binaries use different subdirectories.
- **Filter definitions (AC-001.5, 001.8).** The effective value of a key is its last occurrence (Git's rule); an empty value is unset. A filter is allowlisted when its name is allowlisted, at least one of `clean`, `smudge`, `process` is set, and every set one is canonical; otherwise it is not (and if none is set and `required` is true, it is a required non-allowlisted filter). `required` follows Git's boolean rules (a key with no value is true); any other value fails the command under AC-001.8.
- **Word-list grammar (AC-001.5, 001.7, 001.11).** One tokenizer on every platform: spaces and tabs separate words; single quotes are literal; in double quotes only backslash before `"` or `\` escapes; outside quotes a backslash escapes the next character. A shell metacharacter (dollar, backtick, semicolon, ampersand, pipe, angle bracket, parenthesis, glob or brace character, tilde, hash, exclamation mark) anywhere outside single quotes, including inside double quotes where a shell still expands dollar and backtick, a newline, an unbalanced quote, or an empty command is not canonical. A Windows path with unquoted backslashes is therefore not allowlisted and its filter is neutralized or, when required and the command is content-writing, the command is refused (AC-001.5); the guide lists this.
- **Tool resolution (AC-001.9, 001.11).** A bare tool name is allowlisted only when resolving it with the managed child's `Env` `PATH` and with Kandev's own `PATH` give the same file after symbolic links, so a caller-supplied `PATH` cannot select another binary. The LFS hook script uses the absolute path from Kandev's own lookup.
- **Global options (AC-001.9, 001.11).** Recognized leading options: `-c`, `-ckey=value`, `--config-env`, `-C`, `--git-dir`, `--work-tree`, `--namespace` (each with its argument, separate or `=` form) and the flags `--no-pager`, `-P`, `--no-optional-locks`, `--no-replace-objects`, `--literal-pathspecs`, `--glob-pathspecs`, `--noglob-pathspecs`, `--icase-pathspecs`, `--bare`, `--no-lazy-fetch`. Any other option before the subcommand (`--exec-path`, `-p`, `--paginate`, `--super-prefix`, or an option missing its argument) is refused with an error naming it.
- **Classification (AC-001.15).** The guarantee is the run-time rule: the subcommand is the first argument after the recognized global options (the same parser as the vector); `worktree`, `branch` and `cat-file` are classified by their next word or options, and any unrecognised form (for example `worktree move`, `branch --list --no-list`) is content-writing. The build-time test `git_subcommand_classification_test.go` is a check over an explicit file inventory in the test: each direct `NewGitCommand` call whose first non-option argument is a literal must name a listed subcommand, and a call that spreads arguments is accepted only in a file named in the inventory. The `RunGit*` helpers take a built `*exec.Cmd` and are not scanned.
- **Hooks map edge cases (AC-002.4).** Two repositories deriving one key give off for that key and a warning. A repository with no ID gives off for its key. A repository record that cannot be read gives off for its key.
- **Explicit `null` (AC-002.7).** A JSON `null` for `run_git_hooks` decodes to a nil pointer and is treated as omitted.
- **Settings copy (AC-003.1, 003.4).** Activation wording: a change applies to agent instances created afterward; a running instance keeps its value, and a new session that reuses an instance keeps it too. Filters: Git LFS and git-crypt work only in standard configuration with the tool installed.

## Settings control

`apps/web/components/settings/repository-card.tsx` gains a `Switch` row beside the existing `pull_before_worktree` control, wired through `workspace-repositories-client.tsx`, `workspace-repositories-save.ts`, `workspace-repositories-dirty.ts`, and `app/actions/workspaces.ts` (`run_git_hooks`).
Layout: desktop shares the pull-before-worktree grid; phone is a full-width row with help text below, touch target at least 44 px. Copy uses `t()` in the settings namespace in all seven locales (Traditional Chinese through `pnpm run i18n:zh-hant`); the help text contains no em dash.
A failed save restores the stored value through the existing dirty-state reset for the single field. A switch change made while a save is in flight stays an unsaved edit after the response arrives (the response is applied only to fields without a newer local edit), and the server applies the last write it receives (AC-003.3). The help text also says that a change applies to agent instances created afterward and not to running ones. It also says that hooks from user-global and system configuration are skipped while the setting is off, that turning the setting on disables submodule recursion for Kandev's pull, fetch, and push in that repository, and that Git LFS and git-crypt filters keep working (AC-003.1).

```text
UI-01: Repository card, Run Git hooks (desktop and phone share one composition)
+----------------------------------------------------------+
| my-repo                                                  |
| ...                                                      |
| Pull before creating worktree                    [ off ] |
| Run Git hooks                                    [ off ] |
| Hooks run with Kandev's permissions and can be changed   |
| by anyone or any agent that can write to the repository. |
| A change applies to agent instances created afterward,   |
| not to running ones. Global hooks are skipped while off; |
| turning it on disables submodule recursion. File-        |
| system monitors, text converters, merge drivers and      |
| filters other than Git LFS and git-crypt stay disabled.  |
+----------------------------------------------------------+
```

The structural requirements are the switch beside its label with the help text directly below and the default off; spacing and exact wording are illustrative.

## Public documentation

`docs/public/git-operations.md` gets a section under "Everyday operations" covering AC-003.4, and the "Troubleshooting" section gains the symptom "my pre-commit hook did not run on a Changes-panel commit". `docs/public/meta.json` and the coverage manifest are updated only if the section list requires it. CHANGELOG.md is generated by git-cliff and is not edited.

## Related decisions

None recorded. The no-runtime-endpoint rule and the default-deny enumeration may deserve a durable decision record; create one with `/record decision` if review asks.
