---
id: "03-guard-validation-entry-points"
title: "Route enabled heavy checks through the shared runner"
status: done
wave: 3
depends_on:
  - "02-isolated-validation-runner"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-VALIDATION-001
acceptance_criteria:
  - AC-EXECUTORS-K8S-VALIDATION-001.2
  - AC-EXECUTORS-K8S-VALIDATION-001.3
  - AC-EXECUTORS-K8S-VALIDATION-001.6
system_design:
  - ../../specs/executors/system-design/kubernetes-validation-isolation.md
---

# Guard supported validation entry points

## Summary

Make enabled worker Make/package checks use the runner before any heavy build,
lint or browser work; document the exact enforcement boundary.

## Scope and exclusions

Guard backend lint/test/build and their root Make delegates; guard both unified
and raw E2E runners before preparation/build. Add recursion protection, browser
host-mode selection and unsafe-override/unsupported-project rejection. Preserve
disabled host/CI behavior, check selection, arguments and output. Update operator
recipe and agent resource-safety guidance. Exclude universal direct-shell hooks,
changes to output-only run-quiet, and automatic production enablement.

## Implementation acceptance

1. Enabled supported commands use one runner invocation before compiling/linting
   or starting browsers, including sibling commands and explicit --host calls.
2. Missing runner/daemon/accounting or unsupported nested project fails before
   work; an inner marker prevents recursion without enabling silent fallback.
3. Disabled host/CI behavior remains unchanged; guidance names guarded paths,
   unsupported direct commands, sequential resource policy and rollout checks.

## Likely files

Existing: `Makefile`, `apps/backend/Makefile`, `scripts/worker-check` (from 02),
`apps/web/e2e/scripts/run-e2e.sh`, `run-raw-e2e.sh`, `run-e2e.test.ts`,
`resource-guard.sh`, `resource-guard.test.ts` in that directory;
`k8s/worker-images/full/README.md`, `apps/web/e2e/README.md`, `AGENTS.md`,
`.agents/skills/e2e/references/resource-safety.md`.
Future outputs: `scripts/worker-check.test.sh` and
`apps/web/e2e/scripts/worker-isolation.test.ts`.

## Verification

Use command-recording shims to prove ordering, argument preservation, no double
wrapping and disabled compatibility. Check enabled `make` dry/real fixture paths,
raw/unified runner worker overrides and unsupported Docker/Kind/SSH projects.
In a fresh worktree install dependencies from apps before package commands:

```bash
(cd apps && pnpm install --frozen-lockfile)
bash scripts/worker-check.test.sh
(cd apps/web && pnpm exec vitest run e2e/scripts/run-e2e.test.ts e2e/scripts/resource-guard.test.ts e2e/scripts/worker-isolation.test.ts)
(cd apps/web && pnpm exec eslint e2e/scripts/worker-isolation.test.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Dependencies, risks and parallelism

Depends on 02; sequential only due runner and recipe file ownership. Existing
E2E docker mode builds on the host before its container: dispatch must happen
before that step. Native-provider direct subprocesses bypass repository wrappers;
document this without claiming universal prevention or intercepting agentctl only.

## Inputs

[Plan](plan.md), [validation design](../../specs/executors/system-design/kubernetes-validation-isolation.md),
[requirements](../../specs/executors/requirements/kubernetes-validation-isolation.md),
runner from 02 and current E2E resource guards.

## Results

Completed 2026-10-07. Make dispatch precedes every guarded prerequisite; managed and raw browser runners dispatch before preparation/build. The enabled runner preserves Make options without forwarding jobserver descriptors, and rejects unsafe worker/shard and daemon-dependent project overrides. Disabled host/CI and inside-child paths retain existing behavior. Command-recording Make checks passed; 42 focused Vitest tests passed (7 isolation scenarios plus retained runner/resource-guard regressions). Targeted ESLint and Prettier passed; 21 worker recipe/runner tests passed. Public documentation validator regressions passed (62 tests), and 47 published pages validated. Catalog/spec lint and diff whitespace passed. Live Docker/Kind containment remains work order 04.
