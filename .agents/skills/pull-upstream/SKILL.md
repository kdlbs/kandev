---
name: pull-upstream
description: Pull latest changes from the public kandev repo, rebase onto them, and fix any conflicts or issues.
---

# Pull Upstream

Pull latest changes from the public kandev repo, rebase onto them, and fix any conflicts or issues.

## Context

This repo (`kdlbs/kandev-orchestrate`) is a private fork of `kdlbs/kandev` (public).

```
origin   → git@github.com:kdlbs/kandev-orchestrate.git (private, our code)
upstream → git@github.com:kdlbs/kandev.git (public, community changes)
```

The public repo's `main` receives bug fixes, features, and community PRs independently.

## Steps

### 1. Fetch upstream

```bash
git fetch upstream
```

### 2. Check what's new

```bash
git log --oneline main..upstream/main
```

If empty, nothing to do. Report and stop.

### 3. Update main

Fast-forward `main` to match upstream, then push:

```bash
git checkout main
git merge --ff-only upstream/main
git push origin main
```

### 4. Rebase feature branch

Switch back to the feature branch and rebase onto updated main:

```bash
git checkout feature/orchestrate
git rebase main
```

### 5. Handle conflicts

If the rebase has conflicts:

1. For each conflicting file, read both sides and resolve correctly
2. Our office feature code takes priority over upstream when the same area is modified
3. Upstream changes to shared infrastructure (agent lifecycle, MCP, task system, workflow engine) should be preserved — integrate both sides
4. After resolving each file: `git add <file>`
5. Continue: `git rebase --continue`
6. Repeat until rebase completes

Common conflict zones:
- `cmd/kandev/main.go` — we wire office services here
- `cmd/kandev/helpers.go` — we register office routes here
- `internal/mcp/server/server.go` — we added ModeOffice
- `internal/mcp/handlers/handlers.go` — we added office MCP tools
- `apps/web/lib/state/` — we added office store slices
- `docs/specs/INDEX.md` — both sides add spec entries
- `CLAUDE.md` — may have parallel edits

### 6. Verify

Run the full verification pipeline. Use subshells so the second command
doesn't resolve `cd apps` relative to the first command's cwd (which
would look for a non-existent `apps/backend/apps/`):

```bash
(cd apps/backend && go build ./... && make test && make lint)
(cd apps && pnpm --filter @kandev/web lint)
```

Fix any issues:
- **Import conflicts**: upstream may have renamed/moved packages (e.g. `agentctl/server/mcp` → `mcp/server`)
- **Interface changes**: upstream may have changed interfaces we implement
- **Test failures**: upstream tests may depend on state we changed
- **Lint errors**: upstream may have added stricter rules

### 7. Push

Force-push the rebased feature branch:

```bash
git push --force-with-lease
```

### 8. Report

Summarize:
- How many upstream commits were pulled
- Any conflicts resolved and how
- Any fixes applied post-rebase
- Build/test/lint status
