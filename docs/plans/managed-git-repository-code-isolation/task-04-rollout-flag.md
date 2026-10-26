---
id: "04-rollout-flag"
title: "Rollout flag"
status: draft
wave: 4
depends_on:
  - "03-settings-switch-and-public-guide"
plan: "plan.md"
requirements:
  - REQ-AGENTS-GITISOLATION-004
acceptance_criteria:
  - AC-AGENTS-GITISOLATION-004.1
  - AC-AGENTS-GITISOLATION-004.2
  - AC-AGENTS-GITISOLATION-004.3
  - AC-AGENTS-GITISOLATION-004.4
  - AC-AGENTS-GITISOLATION-004.5
  - AC-AGENTS-GITISOLATION-004.6
  - AC-AGENTS-GITISOLATION-004.7
  - AC-AGENTS-GITISOLATION-004.8
  - AC-AGENTS-GITISOLATION-004.9
system_design:
  - ../../specs/agents/system-design/managed-git-repository-code-isolation.md
  - ../../specs/agents/system-design/managed-git-isolation-rollout.md
---

# Task 04: Rollout flag

## Goal

Gate tasks 01 to 03 behind `features.managedGitIsolation` so that the off path is
byte-for-byte today's behavior, and the on path is the reviewed isolation.

## Work

- Write the flag-off tests first (red against the ported change): argument vector,
  environment and process count equal to a golden fixture captured from the baseline
  (`origin/main` at the porting base) for each helper category; no `git_hooks` map derived, no `git_isolation` sent and no capability check;
  an absent `git_isolation` in an agentctl create request is off.
- Register the flag (`runtimeflags/registry.go`, `config.Features`, `profiles.yaml`:
  off in every profile (prod, dev, e2e), restart-required) following `/runtime-feature-flags`.
- `subproc.SetManagedGitIsolation`, called once at startup; a distinct helper
  argument for `gitinit`; `git_isolation` on the create request, adopted by the
  agentctl instance manager (`admitCreate` adopts, checks published plus provisional instances, and clears the
  mode when the live set empties, in the same critical section as the removal; a differing mode is refused with `ErrGitIsolationMismatch`).
- Make the capability check conditional on the mode; it covers probe instances too.
- Tests for AC-004.8: mismatch refusal text, two concurrent opposite-mode creates, two same-mode
  creates where the first fails, a last-instance failure clearing the mode, a cleanup-failed
  bundle keeping the other mode refused, published and provisional duplicate IDs, a port-allocation failure on an empty process leaving the
  mode off, a probe instance holding the other mode.
- Web: disabled switch with `aria-describedby` to the explanation while the flag is off; the
  explanation copy names the restart and the environment variable, in all seven languages.
- Public guide: flag, default, how to enable, migration consequence.
- Keep the pre-existing pins in `submodule.go`, `local_repository_initialization.go`,
  `preservation.go` (do not remove them).

## Out of scope

Promoting the flag to `prod: true`; migration tooling.

## Acceptance

`AC-AGENTS-GITISOLATION-004.1` through `004.9`.

## Verification

```bash
cd apps/backend && go test ./internal/common/subproc/... ./internal/runtimeflags/... ./internal/profiles/... ./internal/agentctl/... ./internal/agent/runtime/...
cd ../web && pnpm run typecheck && pnpm run i18n:check && pnpm e2e:run -- --grep "Run Git hooks"
cd ../.. && python3 scripts/lint-spec-files.py --all && python3 scripts/list-docs.py validate
```

## Likely files and risks

`apps/backend/internal/runtimeflags/registry.go`, `apps/backend/internal/common/config`,
`profiles.yaml`, `apps/backend/internal/common/subproc/git_lifecycle.go`,
`apps/backend/internal/task/gitinit/command_descriptor.go`, the agentctl instance create path,
`apps/web/components/settings/repository-card.tsx`. Risk: a code path that reaches the
policy without the mode test, which would change flag-off behavior.

## Results

Implemented. Flag-off tests: `subproc/git_policy_off_test.go` (argument vector
and environment unchanged, no enumeration process, hooks still run),
`agentctl/managed_git_policy_test.go` (no identity probe and no `git_isolation`
or `git_hooks` in the create request), `lifecycle/git_hooks_map_test.go` (no
hooks map), `instance/git_isolation_adopt_test.go`, `gitinit` helper argument,
and the web switch test. Test packages that describe flag-on behavior enable the
mode in a `git_isolation_mode_test.go` `init`.

Re-applies when kdlbs/kandev#4769 lands: nothing in this change references
sandbox code. A sandbox launch path that runs Git must reach `startManagedGit`
like every other managed command; no change here is required for that.
