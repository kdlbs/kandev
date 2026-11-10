---
id: "01-local-runtime"
title: "Install and resolve the official local runtime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-001
  - REQ-AGENTS-ANTIGRAVITY-ACP-002
  - REQ-AGENTS-ANTIGRAVITY-ACP-007
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-015
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-001.1
  - AC-AGENTS-ANTIGRAVITY-ACP-001.2
  - AC-AGENTS-ANTIGRAVITY-ACP-001.3
  - AC-AGENTS-ANTIGRAVITY-ACP-001.4
  - AC-AGENTS-ANTIGRAVITY-ACP-001.5
  - AC-AGENTS-ANTIGRAVITY-ACP-001.6
  - AC-AGENTS-ANTIGRAVITY-ACP-001.8
  - AC-AGENTS-ANTIGRAVITY-ACP-001.9
  - AC-AGENTS-ANTIGRAVITY-ACP-002.3
  - AC-AGENTS-ANTIGRAVITY-ACP-002.8
  - AC-AGENTS-ANTIGRAVITY-ACP-002.9
  - AC-AGENTS-ANTIGRAVITY-ACP-007.1
  - AC-AGENTS-ANTIGRAVITY-ACP-007.4
  - AC-AGENTS-ANTIGRAVITY-ACP-009.1
  - AC-AGENTS-ANTIGRAVITY-ACP-009.2
  - AC-AGENTS-ANTIGRAVITY-ACP-009.3
  - AC-AGENTS-ANTIGRAVITY-ACP-009.4
  - AC-AGENTS-ANTIGRAVITY-ACP-015.1
  - AC-AGENTS-ANTIGRAVITY-ACP-015.2
  - AC-AGENTS-ANTIGRAVITY-ACP-015.3
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 01: Install and resolve the official local runtime

## Summary

Make the official server and harness explicitly installable through existing local install jobs.
Resolve one complete runtime consistently and expose installed versus ready without overwriting external binaries.

## In scope

- Add a provider-specific artifact table, filesystem resolver, bounded downloader, safe extractor, and atomic directory publication.
- Recheck official release digests during implementation. Pin reviewed values; never resolve mutable latest at job execution.
- Reuse the current install job store with a typed Go action, status/output/cancel, maintenance exclusion, and operator guards.
- Add `supports_local_install` to discovery DTOs and web types. Keep `InstallScript()` empty and remote bootstrap unchanged.
- Update catalog filtering and Install button gating for the capability without fabricating a shell install command.
- Wire local lifecycle and hostutility command resolution to the same complete bundle. Preserve pure base constructors and manual PATH fallback only when no managed destination exists.
- Validate manual harness overrides. Separate filesystem discovery from subprocess readiness probes.
- Preserve identity, logos, profiles, skill metadata, remote declarations, and universal permission defaults.
- Capture an unmodified native startup result on the current Linux host. Record SIGILL as a platform limitation if still present.

## Out of scope

Google sign-in, native auto-updates, a version selector, remote installation, credential changes, and automatic CPU affinity.

## Acceptance

1. An authorized Install creates a verified complete bundle; unsafe archives, cancellation, and failed publication preserve previous usable state.
2. Settings can install despite an empty script, show progress/failure, and refresh discovery. Unsupported/crashing hosts have specific guidance.
3. Discovery, local launch, and hostutility choose the same runtime. Invalid managed state cannot silently select PATH; remote launch never receives a host path.

## ASCII UI preview

Excerpt of [UI-01](plan.md#ui-01-agent-settings-installation), covering local installation under 009.1, 009.4, and 015.1-015.3.

```text
Desktop card                          Phone stacked card
+--------------------------------+    +----------------------------+
| Antigravity: ACP not installed  |    | < Agents                   |
| Separate from the agy CLI.      |    | Antigravity                |
| [Install ACP]                  |    | ACP not installed          |
+--------------------------------+    | [       Install ACP      ] |
                                      +----------------------------+
```

Installing shows existing progress and Cancel; failure shows its cause and Retry.
The phone primary action is full width, at least 44px, and remains reachable while details wrap.
The full plan owns the combined state and layout preview.

## Verification

Run from the repository root. Install pnpm dependencies once in a fresh worktree before web commands.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/agent/antigravityruntime ./internal/agent/agents ./internal/agent/settings/controller -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run 'TestAntigravity' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/agents/agent-install-catalog.test.ts)
(cd apps/web && pnpm exec eslint components/settings/install-agent-card.tsx components/settings/agents/agent-install-catalog.tsx lib/types/http-agents.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/antigravity-setup.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-antigravity-setup.spec.ts)
git diff --check
```

Named new tests: `TestInstallAntigravityBundle`, `TestResolveAntigravityBundle`, `TestAntigravityLocalInstallJob`, and `TestAntigravityCommandLocality`.
Exercise each archive/platform/cancellation case from the plan, including no writes or spawn during discovery.
Run changed-Go lint against every changed/new Go file before recording completion.
The native startup observation is evidence, not a requirement to solve a vendor binary defect inside this work order.
Do not label a diagnostic `taskset` run as an unmodified native pass.

## Files likely touched

- `apps/backend/internal/agent/antigravityruntime/` (new provider-specific files and tests).
- `apps/backend/internal/agent/agents/antigravity_acp.go`, `antigravity_acp_test.go`.
- `apps/backend/internal/agent/settings/controller/agent_install.go`, `install_job.go`, `agent_discovery.go`, and install tests.
- `apps/backend/internal/agent/settings/dto/dto.go`.
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` and local command selection helpers/tests.
- `apps/backend/internal/agent/hostutility/manager.go` and focused resolver tests.
- `apps/web/lib/types/http-agents.ts`, `components/settings/install-agent-card.tsx`, `components/settings/agents/agent-install-catalog.tsx` and tests.
- `apps/web/e2e/tests/settings/antigravity-setup.spec.ts`, `mobile-antigravity-setup.spec.ts`, and a shared synthetic setup helper.
- `apps/web/src/locales/` for all seven locales; generate Traditional Chinese through the repository tool.

## Dependencies

None.

## Risks

- ACP 1.4.5 crashes on the investigated Linux host. Installation completeness does not establish readiness.
- The large archive needs meaningful bounded extraction; fake ZIP fixtures must not include vendor executables.
- A corrupt existing managed destination must not be overwritten while live users might hold it.

## Parallelism

`sequential`

## Inputs

- Design sections Local runtime resolution, Installation, Existing identity, and Qualification.
- Existing install job and `agent-install-streaming.spec.ts` patterns.
- The requirement revision table, which retires PATH-only and empty-script-only local behavior.

## Results

Implemented.

- Added `internal/agent/antigravityruntime` with the reviewed 1.4.5 artifact table, filesystem resolver, bounded download, safe extraction, digest/size checks, manifest validation, and atomic publication.
- Added `agents.LocalInstallAgent` and `agents.LocalRuntimeAgent`; `AntigravityACP` now resolves the managed bundle or a validated manual PATH install for local launch, discovery, and host utility.
- Extended `JobStore` with a server-owned Go install action; `EnqueueInstall` selects it when the agent supports local installation. `InstallScript()` remains empty.
- Added `supports_local_install` to discovery DTOs and web types, catalog filtering, and install-button gating with localized guidance.
- Added `TestInstallAntigravityBundle`, `TestResolveAntigravityBundle`, `TestAntigravityLocalInstallJob`, and catalog capability coverage.

Verification: `go build ./...`; `go test -trimpath ./internal/agent/antigravityruntime ./internal/agent/agents ./internal/agent/settings/controller -count=1`; web Vitest, typecheck, and i18n checks; changed-Go lint 0 issues.

Native startup on the investigated Linux host was not run to completion. Task 07 remains the readiness gate; this work order does not claim live qualification.
