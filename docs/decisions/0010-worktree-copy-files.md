# 0010: Worktree copy-files with per-entry materialization modes

**Status:** accepted (amended 2026-07-13)
**Date:** 2026-05-19
**Area:** backend, frontend

## Context

Git worktrees do not contain gitignored files. Users commonly need `.env`, local configuration, or certificates before a task's setup script can run. Issue #946 introduced a per-repository `copy_files` setting, following vibe-kanban's comma-separated format.

Copying isolates each task but does not propagate later source changes. Some files instead need one centrally managed source whose changes are immediately visible in every host worktree. PR #1650 added a symlink mode, but its first parser treated every final colon segment as a keyword. That broke previously valid POSIX paths such as `config:dev` and `.env:` and required an explicit compatibility rule.

## Decision

`repositories.copy_files` remains a per-repository, comma-separated list of repository-relative paths or doublestar patterns. Entries are materialized during worktree creation, before the setup script, and existing destinations are never overwritten.

Each entry has one of two modes:

- **Copy**, the default: copy file contents or recursively copy a matched directory.
- **Symlink**: an exact terminal `:symlink` suffix, for example `.env:symlink`, creates a relative symlink at `.env` pointing to the source repository's `.env`.

The grammar is deliberately narrow:

1. Only the exact terminal `:symlink` suffix is reserved. Other colons are literal path characters, so `config:dev`, `.env:`, and `file:hardlink` retain their pre-mode meaning.
2. A doubled colon escapes the reserved suffix. `config::symlink` copies the literal path `config:symlink`.
3. A reserved suffix without a path, such as `:symlink`, is malformed and repository create/update rejects it.
4. Entries are normalized and deduplicated by path. The first entry wins for duplicate paths and for distinct patterns that overlap the same file. Ordering therefore selects the mode for an overlapping match.
5. Commas inside brace alternation remain part of the pattern. `*`, `?`, character classes, `**`, and brace alternation are provided by `doublestar`.

This suffix-plus-escape grammar is less extensible than structured JSON, but it preserves the established string field and all legacy colon-bearing paths except the newly reserved exact `:symlink` form. That exact form must stay reserved because it shipped in PR #1650; the doubled-colon escape keeps a literal filename ending in `:symlink` representable. Future modes must not reinterpret arbitrary trailing colon segments.

## Host And Remote Behavior

Host worktrees materialize through `copyfiles.Copy`:

- Copy mode snapshots bytes when the worktree is created. Later source changes do not propagate.
- Symlink mode creates a relative link to the source repository. Reads and writes through the worktree link immediately affect the shared source and are visible through every link to it.
- A symlink creation failure is warned and skipped; it does not silently change to copy mode.

Remote executors cannot use a link to the host repository. `copyfiles.Parse` strips the recognized mode while preserving literal colon-bearing paths, `copyfiles.Plan` reads the source bytes, and agentctl `WriteEntries` writes those bytes remotely. Symlink entries therefore fall back to copy mode for local Docker, Sprites, SSH, and future remote executors using this path. The repository settings UI states this fallback without requiring hover. Remote payloads are capped at 5 MiB per file; oversized matches are warned and skipped.

Windows copy mode is supported. Host symlink mode is best-effort because Windows symlink creation can require Developer Mode or elevated privileges; failures remain warnings so task creation continues. Relative symlink behavior is covered on Unix and macOS, while Windows tests skip when the platform cannot create links.

## Security And Containment

- Source roots are canonicalized with `EvalSymlinks`; matches that resolve outside the source repository are rejected.
- Host destinations must remain under the worktree. Existing symlinked destination parents are rejected before directories or links are created, preventing writes through a parent link outside the worktree.
- Symlink targets are relative and point only to already-contained source matches.
- Remote `WriteEntries` canonicalizes both the workspace containment root and target repository. Every untrusted relative entry is rejected if it is absolute, traverses outside the target, or reaches a symlinked parent outside containment.
- Existing destinations are skipped for idempotency. Missing or rejected matches produce visible warnings rather than blocking task creation.

## Consequences

- Configuration is per repository rather than per task or workspace, so all future tasks for that repository share the policy.
- Changing `copy_files` affects only worktrees created afterward. The exception is content reached through an existing symlink, which is live by definition.
- Copy mode can consume unbounded host disk for broad patterns; remote memory and wire use are bounded by the per-file cap.
- Prepare progress reports copied/materialized files and warnings. The `worktree.RepositoryAdapter` remains the boundary from task repository models to worktree configuration.

## Alternatives Considered

- **Setup scripts.** Rejected because they require platform-specific shell commands, can expose secret paths in output, and duplicate containment logic.
- **Per-task overrides.** Rejected because repeated task configuration defeats the repository-level default.
- **Overwrite or watch copied files.** Rejected because agents may modify worktree copies and automatic propagation would destroy task-local changes.
- **JSON/YAML structured entries.** More extensible and unambiguous, but changing the persisted/API shape would impose migration and UI complexity on an established simple field. The exact suffix plus escape solves the current two-mode requirement while preserving legacy paths.
- **Treat every final colon segment as a mode.** Rejected because POSIX permits colons in filenames and existing valid settings would change meaning or fail validation.
