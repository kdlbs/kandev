# 0002: E2E git shim for simulating slow git operations

**Status:** accepted
**Date:** 2026-04-08
**Area:** infra

## Context

Several real-world bugs only manifest when `git fetch` or `git pull` is slow — e.g. large monorepos with many tags where a fetch can take 30–60s. These slow operations serialise the entire worktree preparation path (`worktree.Manager.pullBaseBranch` → `env_preparer_worktree.Prepare` → `runEnvironmentPreparer` → `lifecycle.Launch`) and delay agentctl readiness, exposing races in the frontend around WS subscribe timing, `agentctlStatus.isReady` gating, and panel mount ordering. The E2E harness runs against a local standalone backend where `git fetch` is normally instantaneous (or fails fast with no remote), so these races can't be reproduced by any combination of fixtures, executor profiles, or task options alone.

## Decision

Install a `git` shim in the E2E backend fixture (`apps/web/e2e/fixtures/backend.ts`) that can be made to sleep on `fetch` and `pull` before delegating to the real `git` binary:

- On worker startup, write `${tmpDir}/bin/git` as a POSIX shell script that:
  1. Checks for a delay file at `$KANDEV_E2E_GIT_DELAY_FILE` (absolute path, set by the fixture).
  2. If the file exists and contains a positive integer, and `$1` is `fetch` or `pull`, sleeps that many milliseconds (rounded up to 1s minimum).
  3. Restores `PATH` from `$KANDEV_E2E_ORIGINAL_PATH` and `exec`s the real `git` with the original arguments.
- Prepend `${tmpDir}/bin` to the backend process's `PATH`, and set `KANDEV_E2E_ORIGINAL_PATH` and `KANDEV_E2E_GIT_DELAY_FILE` in its env.
- Tests that want to simulate slow git write a millisecond value to `${backend.tmpDir}/git-delay-ms` before creating the task, and delete the file in their `finally` block so subsequent tests in the same worker run with fast git.

The shim is always installed. When the delay file is absent it's a transparent passthrough — other tests pay no cost. Tests that need slow git depend on the `backend` worker fixture directly to access `backend.tmpDir`.

## Consequences

**Easier:**
- Reproducing real-world slow-git bugs deterministically in E2E without touching backend code.
- Future tests can reuse the same file-based toggle to simulate any timing-dependent scenarios involving git (auth prompts, network stalls, etc.) by extending the shim.
- The shim leaves the real git flow intact after the sleep, so all downstream git state (refs, lockfiles, error codes) behaves exactly as production.

**Harder:**
- Adds a small amount of fixture complexity; anyone debugging "why is git slow in my E2E" needs to know to check `${tmpDir}/git-delay-ms` and the shim at `${tmpDir}/bin/git`.
- The file-based toggle is per-worker, so tests that want to scope the slowness must clean up after themselves. If they don't, sibling tests in the same worker will observe the delay. This is mitigated by the `finally`-cleanup convention but not enforced.
- Only affects `git fetch`/`pull`. Other slow-network scenarios (e.g. `git clone`) would need the shim to be extended.

## Alternatives Considered

**Adding a test-only env var to backend code** (e.g. `KANDEV_E2E_PREPARE_DELAY_MS` read in `manager_launch.runEnvironmentPreparer` or `manager_startup.waitForAgentctlReady`) — rejected. It pollutes production code with test concerns, exercises a fake path that doesn't match the real race (artificially delaying `AgentctlReady` while agentctl is actually up), and sets a precedent for sprinkling test hooks through the lifecycle manager.

**Creating a custom executor type that delays prepare** — rejected. Requires a new preparer, registry entry, and fixture plumbing for minimal gain; the shim is fewer lines and more faithful to reality.

**Enabling Docker in the E2E worker and running agentctl in a container with a slow prepare** — rejected. Heaviest lift, flaky in CI, requires docker-in-docker or a daemon on runners, and doesn't port to environments without Docker.

**Adding `prepare_script: "sleep 30"` to a worktree executor profile** — rejected after investigation. The profile plumbing works and the script runs, but in the local standalone runtime the script runs in parallel with agentctl instance creation rather than blocking it, so the frontend-visible race (`isReady=false` while panels are mounted) never manifests. `git fetch` in `pullBaseBranch` is the one operation on the worktree path that genuinely serialises Launch behind it.
