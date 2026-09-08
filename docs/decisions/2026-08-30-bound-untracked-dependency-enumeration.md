# ADR-2026-08-30-bound-untracked-dependency-enumeration: Bound Untracked Dependency Enumeration

**Status:** accepted
**Date:** 2026-08-30
**Area:** backend

## Context

Workspace status currently runs `git status --porcelain --untracked-files=all`. Git emits every untracked file when a repository has no applicable ignore rule. Agentctl then parses and enriches each entry. The workspace monitor also lists and stats every untracked file.

This behavior exposed 4,889 pnpm dependency files for several minutes when `pnpm install` finished before the repository created `.gitignore`. The Changes panel received the oversized snapshot, and the monitor repeated work over the same tree. A later ignore rule removed the entries only after another refresh.

Git ignore rules remain the repository's source of truth for ordinary untracked files. Kandev also needs a narrow performance boundary that applies before a dependency manager can create an oversized status snapshot.

## Decision

Agentctl excludes untracked directories named `node_modules` at any repository depth before Git returns individual untracked paths. This exclusion also covers pnpm content under `node_modules/.pnpm`.

Full status uses separate tracked and untracked queries. The tracked query disables untracked enumeration and preserves every tracked change. The untracked query uses Git's standard ignore rules plus the `node_modules/` exclusion. It returns NUL-separated paths for direct insertion into the existing status model. Both queries read a temporary snapshot of the Git index, so a tracking-state change between the commands cannot produce a contradictory projection.

The workspace monitor uses the same untracked query definition. It does not stat excluded dependency files or refresh status when only those files change.

The first owned exclusion is `node_modules`. Other generated directories continue to follow Git ignore rules. Kandev does not filter tracked paths based on their directory name.

## Consequences

- Missing or late `.gitignore` files cannot make JavaScript dependency trees dominate status collection or the Changes panel.
- Tracked changes below `node_modules` remain visible because the tracked query has no generated-tree path filter.
- Ordinary untracked files remain visible and retain the existing diff limits.
- One full observation uses an extra Git subprocess to keep tracked and untracked policies separate.
- The temporary index snapshot adds a small per-observation filesystem operation while keeping concurrent user Git writes independent.
- The Changes panel no longer mirrors raw `git status --untracked-files=all` for untracked `node_modules` content.
- Future generated-tree exclusions require another reviewed contract change. They must not silently broaden this list.

## Alternatives Considered

### Rely only on repository and global ignore rules

This keeps exact Git status parity, but it does not protect a new repository before its ignore file exists. It also does not protect a repository with an incomplete ignore file.

### Remove dependency paths after parsing status output

This keeps the panel clean, but Git still enumerates the tree and agentctl still receives the large output. It does not solve the main performance problem.

### Stop after a fixed untracked-file limit

This bounds output, but it can hide ordinary source files. The result also depends on enumeration order and gives users an incomplete, unstable snapshot.

### Exclude a broad list of generated directory names

Names such as `dist`, `build`, and `.next` can contain files that a repository expects users to review or stage. Starting with `node_modules` provides the required protection without hiding those project-specific outputs.
