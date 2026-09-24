---
created: 2026-09-24
status: draft
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs: []
---

# Implementation Plan: Isolate Managed npm Runtime Configuration

## Overview

Issue [#3902](https://github.com/kdlbs/kandev/issues/3902) reports that a
recent managed ACP runtime starts successfully in other repositories but fails
in a task workspace whose `.npmrc` sets `min-release-age`. The agent process
uses that workspace as its working directory, so npm applies the repository's
project policy to Kandev's runtime package. Agentctl also drops npm's
date-qualified `notarget` line, leaving the failure as a generic ACP disconnect.

First isolate npm's project configuration for every managed runtime command.
Then classify any remaining release-date policy failure and show the correct
recovery explanation. These are sequential work orders so the first result can
be verified independently of the new error presentation.

## Scope

### In scope

- Managed npm preparation, probes, one-shot prompts, task launches, cache
  discovery, and online-preferred retries on supported execution hosts.
- Strict date-qualified `ETARGET` classification, no automatic cache repair for
  that class, and localized desktop, phone, and Office error presentation.
- Public troubleshooting text for repository `.npmrc` and remaining npm policy.

### Out of scope

- Native binaries, passthrough commands, dependency resolution policy inside
  the agent's own shell, version rollback, or changing the npm registry.
- The broader managed installation redesign in issue #2419.

## Confirmed root cause and reproduction

- `process.Manager.buildFinalCommand` assigns `m.cfg.WorkDir` to the child. The
  managed command from `ManagedNPMRuntimeSpec` is `npx --yes --prefer-offline
  package@version`, so npm reads the workspace's project `.npmrc`.
- npm's date-qualified line does not match `npmresolution.MatchesExactPackage`.
  `safeManagedNpmStderrLine` drops that line before the lifecycle manager reads
  bounded stderr. The online retry therefore does not run; it would also be
  ineffective while the age policy remains in force.
- The issue records the failing exact command and successful prefixed command.
  A local npm 11.13.0 check from a project with an invalid registry found that
  `--prefix` selects the other project's configuration. A real `npx` launch of
  `cowsay@1.6.0` succeeded from that project with the prefix, retained the
  workspace cwd, and used the same expected `_npx` key. A missing prefix
  directory failed with `ENOENT`; provisioning is part of Task 01.

## Technical approach

### Managed command and executor boundary

- `apps/backend/internal/agent/agents/managed_npm_runtime.go` supplies the
  trusted `--prefix ~/.kandev/managed-npm-runtime` argument for
  `ACPCommandWithNpmPreference` and `CacheUpdateCommand`. Keep exact
  package/version and ACP argument order.
- Create that directory under the effective npm child home before invoking it.
  Cover the backend host update runner,
  `agentctl/server/process.Manager`, and `agentctl/server/utility.ACPInferenceExecutor`
  probe and prompt paths. The `~` must resolve on the npm execution host, never
  to the backend host's absolute home for a remote task. Preserve the child
  `cmd.Dir` and session cwd.
- Update `onlineManagedRuntimeArgs`, `managedRuntimeProbeRetry`, and
  `managedRuntimeProbePackageSpec` to accept only the new trusted command shape.
  Continue rejecting unversioned, foreign, native, and passthrough commands.
- `process.Manager.RepairManagedRuntimeCacheWithEnvironment` resolves npm cache
  with the same prefix and effective environment as the failed launch. The
  deterministic key stays derived from the exact package spec.

### Date-qualified failure

- `agentctl/server/process/safeManagedNpmStderrLine` retains a canonical,
  bounded release-date marker without copying the raw date or unrelated stderr.
- `common/npmresolution` distinguishes ordinary exact-package `ETARGET` from
  date-qualified `ETARGET`. Lifecycle and agentctl probe classification use the
  same shared matcher. A policy failure bypasses cache invalidation and the
  online retry.
- Add a stable policy failure code to `routingerr` and the probe contract.
  Carry sanitized details into the existing failure record and recovery entry.
  Kanban and Office render a policy-specific explanation through their current
  inline recovery surfaces. Keep the current retry request for use after a
  policy change or after the version becomes old enough.
- Add new copy to all required locale catalogs. Update
  `docs/public/agents-and-profiles.md` as a how-to/troubleshooting section.

## ASCII UI preview

`UI-01: Managed runtime startup error`, entry point: task or Office chat after
an exact-package npm release-date failure. The existing inline card owns the
transcript position and details disclosure on both viewports. Text below is
illustrative; the policy cause, lack of cache-repair claim, and recovery action
are required by AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.3 and .4.

```text
Before (desktop and phone)
+--------------------------------------------------+
| Agent startup failed: The agent could not start. |
| [Retry]                                          |
+--------------------------------------------------+

After (desktop)
+-------------------------------------------------------------+
| ! npm blocked the selected runtime version                  |
| Check min-release-age or before in npm settings. Wait for   |
| the version to become eligible or select an older version.  |
| Technical details  >                                        |
| [Retry runtime]                                             |
+-------------------------------------------------------------+

After (phone)
+-------------------------------------+
| ! npm blocked this runtime          |
| Check min-release-age or before.    |
| Wait or select an older version.    |
| Technical details  >               |
| [ Retry runtime (touch target) ]    |
+-------------------------------------+
```

The phone card stays in the existing chat scroll owner. Its action remains at
least 44 px high and technical details stay collapsed until selected. No new
overlay or navigation is introduced. The nearest mobile exemplar is the
existing `mobile-managed-runtime-npm-recovery.spec.ts` card.

## Tests

| Acceptance | Regression evidence |
| --- | --- |
| 003.1, 003.2 | `TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc` plus command, prefix-provisioning, cache-key, and executor-local command tests in the managed runtime, process, utility, lifecycle, hostutility, and update-runner packages. This test must fail before the correction. |
| 003.3 | `TestManagedRuntimeReleaseAgePolicySkipsCacheRepair` in lifecycle and hostutility; exact-package/date matcher and safe stderr tests. |
| 003.4 | Orchestrator recovery metadata tests, Kanban/Office component tests, localization checks, and focused desktop/phone Playwright flows. |

## E2E tests

- AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.4: extend
  `apps/web/e2e/tests/session/managed-runtime-npm-recovery.spec.ts` and
  `mobile-managed-runtime-npm-recovery.spec.ts` with the policy failure's
  distinct title/body, collapsed details, and retry action. Extend the Office
  managed-runtime recovery spec for the same classification.
- AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.1: targeted Go process integration
  evidence exercises the actual npm project-config selection. UI seeding alone
  cannot prove npm's resolution behavior.

## Work orders

- [x] [Task 01: Isolate managed npm commands](task-01-isolate-managed-npm-commands.md)
- [x] [Task 02: Report release-age policy failures](task-02-report-release-age-policy-failures.md)

## Verification results

Task 01 and Task 02 targeted Go package suites passed, and
`make -C apps/backend build` passed. Focused web tests, typecheck, changed-file
ESLint, i18n check, production build, and desktop, mobile, and Office E2E checks
passed. The review correction was revalidated with the Task 02 Go suite and
backend build. Public documentation/spec validation and `git diff --check`
passed. Run `pnpm --filter @kandev/web build` so its prebuild hooks generate
release notes and the changelog before Vite runs.

## Risks

- npm returns `ENOENT` for a missing prefix; every execution path must provision
  it before invoking npm, including remote and container paths.
- Command-shape guards, cache lookup, and update preparation must agree on the
  same prefix. A mismatch could disable stale-metadata recovery or target the
  wrong cache.
- Explicit user/global npm policy remains effective. Its date-qualified
  failure must not be mistaken for stale metadata.
