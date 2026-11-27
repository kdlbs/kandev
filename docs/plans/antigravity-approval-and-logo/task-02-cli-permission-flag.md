---
id: "02-cli-permission-flag"
title: "Expose the CLI permission flag"
status: complete
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-001
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-001.8
  - AC-AGENTS-ANTIGRAVITY-ACP-003.13
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 02 Expose the CLI permission flag

## Summary

Surface `--dangerously-skip-permissions` in the Antigravity profile's existing CLI Flags card. The user can enable the flag for new eligible `agy acp` launches.

## In scope

- Use a curated `cli_flag` entry with default false. Use a key distinct from the legacy Claude passthrough boolean. The editor treats all exact matches as one switch and saves one effective row; backend profile writes also collapse duplicates.
- Use the same runtime support evidence for preview and launch. Reuse the existing bounded CLI help check to qualify support. Apply the enabled flag only to supported CLI launches whose session policy permits blanket approval. Strip it from unsupported, standalone, non-local, utility, inference, and scoped launch commands while retaining unrelated arguments.
- Localize the new control label and description. Cover the stored flag change from both settings layouts. Document that the CLI approves all tool requests when the flag applies.

## Scope limits

Use the existing `cli_flags` list and agent catalog. The flag applies to launches whose session policy permits blanket provider approval. Coordinator and managed tool enforcement depend on the existing approval path.

## Acceptance

- Fresh and existing profiles offer one accessible switch. It starts off unless any existing exact match is enabled. Save, reload, and switch off through the existing editor. A saved disabled first match with an enabled duplicate turns off and persists as one disabled choice.
- Preview, probes, utilities, actual launch, and context reset apply the same CLI compatibility and session-policy rules. Off, unsupported CLI, standalone, non-local, and scoped permission launches omit the bypass flag. The CLI worker option and unrelated flags still apply.
- The phone settings row supports touch and remains within the viewport. The backend retains its separate Kandev auto-approval setting.

## ASCII UI preview

### UI-01 Profile CLI flag

```text
Agent CLI flags
  Skip Antigravity CLI tool prompts             [Off]
  --dangerously-skip-permissions
```

Use the existing Settings card, save action, and phone scroll behavior. [Full preview](plan.md#ui-01-profile-cli-flag)

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/settings/controller ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run '^Test(AntigravityACP_PermissionAndBillingDefaults|AntigravityCommandPreviewUsesSelectedLocalRuntime|AntigravityLocalLaunchUsesCLIAndPreservesProfileArgs|AntigravityHostLaunchPolicy|AntigravityProfileCollapsesDuplicateSkipFlagAndAllowsTurningItOff)$' -count=1)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-cli-flags.spec.ts -- --grep 'Antigravity CLI permission flag')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-antigravity-runtime-setup.spec.ts -- --grep 'Antigravity CLI permission flag')
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/settings/agent-profile-cli-flags.spec.ts -- --grep 'Antigravity switch turns off a saved enabled duplicate')
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/agent/agents/antigravity_acp.go` and `antigravity_acp_test.go`
- `apps/backend/internal/agent/antigravityruntime/local.go` and CLI launch tests
- `apps/backend/internal/agent/settings/controller/agent_config.go` and command preview tests
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`, `apps/backend/internal/agent/hostutility/manager.go`, and related CLI policy tests
- `apps/web/components/settings/cli-flags-field.tsx`, `apps/web/src/locales/*/agents.json`, and the two settings E2E files in the plan
- `docs/public/agents-and-profiles.md`

## Dependencies

None. This switch controls provider CLI approval independently of work order 01.

## Risks

Profile data can predate the catalog. Use the current fallback switch state and preserve the explicit saved entry. Validate the affected preview, probe, and session launch paths against the same runtime selection.

## Parallelism

`sequential`

## Inputs

- [Antigravity profile and launch design](../../specs/agents/system-design/antigravity-acp-agent.md#existing-identity)
- [CLI settings preview](plan.md#ui-01-profile-cli-flag)
- [CLI flag reference](https://antigravity.google/docs/ide/extensions/acp#flags)

## Results

The Antigravity-specific CLI flag is curated and disabled by default. Exact
duplicates are reconciled into one effective profile choice on settings and
backend writes, including quoted and bundled flag tokens while retaining
neighboring arguments. Preview checks CLI support. Profile inference and
host-utility launches always strip the provider-wide bypass; initial spawn and
context reset retain it only for a supported Antigravity CLI and an ordinary
task surface without coordinator or managed-tool policy. Desktop and phone
settings tests cover default state, touch, save, reload, and disable.

Verification passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/antigravityruntime ./internal/agent/settings/controller ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run '^Test(AntigravityACP_PermissionAndBillingDefaults|SeedCLIFlags_AntigravitySkipPermissionsOffByDefault|ResolveLocalReportsDangerousPermissionFlagSupport|FilterDangerouslySkipPermissions|AntigravityCommandPreviewUsesSelectedLocalRuntime|AntigravityLocalLaunchUsesCLIAndPreservesProfileArgs|AntigravitySkipPermissionsLaunchPolicy|AntigravityHostLaunchPolicy|AntigravityProfileCollapsesDuplicateSkipFlagAndAllowsTurningItOff|MockAgent_AntigravityIdentityUsesCuratedProfileSettings)$' -count=1)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-cli-flags.spec.ts -- --grep 'Antigravity CLI permission flag')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-antigravity-runtime-setup.spec.ts -- --grep 'Antigravity CLI permission flag')
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/settings/agent-profile-cli-flags.spec.ts -- --grep 'Antigravity switch turns off a saved enabled duplicate')
node scripts/validate-public-docs.mjs
```

After rebasing, the full affected backend package suite passed under
`-race -trimpath -tags fts5`. The separate duplicate-profile E2E passed and
confirmed one saved disabled flag plus the unrelated enabled profile flag.
