---
id: "00-verification-cache-safeguard"
title: "Verification cache safeguard"
status: done
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/workspace-git-status.md"
---

# Task 00: Verification cache safeguard

## Acceptance

- Root-level `.verify-cache` and `.tmp` are ignored without hiding similarly named nested fixture paths.
- Verification guidance preserves shared Go and lint caches while requiring scratch and command output to live outside the repository and all Git worktrees.
- The safeguard is documented in the Workspace Git Status capability spec.

## Verification

```bash
rtk git check-ignore -q .verify-cache/go-cache/probe
rtk git check-ignore -q .tmp/gocache/probe
rtk rg -n 'outside the repository|/\.verify-cache/' .agents/skills/verify/SKILL.md .gitignore
```

## Files touched

- `.gitignore`
- `.agents/skills/verify/SKILL.md`
- `docs/specs/platform/workspace-git-status.md`
- `docs/plans/workspace-git-status-scalability/plan.md`

## Dependencies

None.

## Inputs

- Spec requirement that verification caches remain outside worktrees.
- Existing disk-constrained verification guidance using an external `mktemp` scratch root.
- The `.verify-cache` incident containing 13,869 untracked Go cache files.

## Output contract

Implemented directly as an incident safeguard. The root ignore rule and external-scratch guidance are present, shared caches remain reusable, their targeted checks pass, and no production API behavior changed.
