---
id: "01-align-profile-mode"
title: "Align profile mode discovery and application"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-REPOSITORY-LOCAL-MODE-001
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-001
acceptance_criteria:
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.1
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.2
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.3
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.4
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.5
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.6
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.7
  - AC-AGENTS-UTILITY-AGENT-PROFILES-001.7
system_design:
  - ../../specs/agents/system-design/repository-local-session-modes.md
  - ../../specs/agents/system-design/utility-agent-profiles.md
---

# Task 01. Align profile mode discovery and application

## Summary

Correct the discovery catalog and the ACP methods used by scratch and utility
sessions. Prove that the existing picker saves the exact value and a task uses
it at its first prompt.

## In scope

- Start with `TestIssue5364ProbePublishesConfigModeValues`,
  `TestIssue5364ProbeSendsExactConfigModeValue`, and
  `TestIssue5364UtilityUsesTypedProfileMode`. Reproduce with the disjoint
  or config-only carrier before editing production code.
- In `applySessionProbeFields`, publish the first mode config select when one
  exists. Reuse `findSelectConfigOption` and `selectOptionsUngrouped`. Match
  that precedence when checking whether the scratch session offers a value.
- Extend the existing mode request and reuse it in `executeACPSession`. Use
  the existing config setter and snapshot wait for the exact value. Keep the
  current call order in each path. Apply the legacy rules in the
  [system design](../../specs/agents/system-design/repository-local-session-modes.md#probe-and-session-lifecycle).
- Reuse custom ACP registration and one fixture for desktop and phone. Give
  the two advertisements distinct IDs and have the fixture report the mode
  it sees at the first prompt. Use the existing profile save and launch flows.
- Document the profile recovery flow in the public guide.

## Out of scope

- Provider-specific aliases, profile migrations, ACP protocol upgrades.
- Changes to task mode precedence, auto-approval, or shared provider settings.

## Acceptance

- A mode select supplies the advertised choices and default to both settings
  capability paths. Grouped, legacy-only, and empty catalogs retain the behavior
  in the [compatibility matrix](plan.md#acp-compatibility).
- Wire tests assert the exact option and value, the mode/model order, the
  allowed fallback, and fail-closed typed errors. An unknown selection is
  skipped in a scratch probe with an advertisement. A utility prompt needs
  confirmation when it uses a typed mode RPC.
- The desktop and phone E2E tests assert saved value and effective mode at the
  first task prompt. Verify that the old ID remains inspectable until save.
  The phone flow also checks touch activation and horizontal containment.

## ASCII UI preview

**UI-01. Profile settings with discovery ready**

The **Start mode** list in Settings > Agents > agent > profile.

```text
Desktop: Start model [Fixture Model v]   Start mode [Ask v]   [Refresh icon]
Phone:   Start model [Fixture Model v] [Refresh icon]
         Start mode  [Ask                        v]
Open:    [Search modes...]
         Ask (Default)
         Accept Edits
         Don't Ask
```

See the [full preview](plan.md#ascii-ui-preview) for the saved legacy and
phone states. Covers AC-AGENTS-REPOSITORY-LOCAL-MODE-001.1, .5, .6, and .7.

## Verification

Run from the repo root. Run this block before and after the fix.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -run '^TestIssue5364' ./internal/agentctl/server/utility)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/settings/agent-profile-acp-mode.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-agent-profile-acp-mode.spec.ts
```

After the fix, run the surrounding checks.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/utility)
(cd apps/backend && go test -trimpath -tags fts5 -run 'Test(SetMode|SetConfigOptionMode|LegacyConfirmedModeLifecycle|ApplyExplicitSessionMode|InitializeAndPromptWithLayers_.*Mode)' ./internal/agentctl/server/adapter/transport/acp ./internal/agent/runtime/lifecycle)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/utility/acp_executor.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

On a fresh worktree, use `pnpm --dir apps install --frozen-lockfile` first.
If Chromium is absent, use
`pnpm --dir apps/web exec playwright install --with-deps chromium`.

## Files likely touched

- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agentctl/server/utility/acp_profile_mode_test.go`
- `apps/backend/internal/agentctl/server/utility/acp_probe_capabilities_test.go`
- `apps/web/e2e/helpers/profile-mode-config-acp-fixture.mjs`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/settings/profile-acp-mode-helpers.ts`
- `apps/web/e2e/tests/settings/agent-profile-acp-mode.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-acp-mode.spec.ts`
- `docs/public/agents-and-profiles.md`

## Dependencies

None.

## Risks

Test both carriers with the same choice labels and different IDs. Matching
display labels alone can make a catalog assertion pass with the wrong saved
value. Check the saved value and the fixture's first prompt state.

## Parallelism

`sequential`

## Inputs

- [Repository-local mode settings and probe design](../../specs/agents/system-design/repository-local-session-modes.md)
- [Utility permissions](../../specs/agents/system-design/utility-agent-profiles.md#permissions)
- [Live adapter mode application](../../../apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_mode.go)
- [Profile editor picker](../../../apps/web/components/settings/mode-combobox.tsx)
- [Custom ACP session fixture pattern](../../../apps/web/e2e/tests/session/custom-acp-session-restore.spec.ts)

## Results

Implemented config-select-first mode discovery and exact-value application for
ACP profiles. Legacy `session/set_mode` is used only when the typed method is
unavailable and the exact ID is advertised. Utility calls now require
confirmation of the selected mode before prompting. Added regression coverage,
desktop and mobile profile flows, and public recovery guidance.

Verification passed:

- `go test -trimpath -tags fts5 ./internal/agentctl/server/utility`
- Focused ACP adapter and lifecycle mode tests.
- `go test -race -trimpath -tags fts5 -count=1 -run '^TestIssue5364' ./internal/agentctl/server/utility`
- Changed-file Go lint.
- Desktop Chromium and mobile Chrome ACP profile E2E tests.
- Public docs tests and validation, specification catalog validation and lint,
  and `git diff --check`.

Static review follow-up also passed the focused ACP race check, the desktop
Chromium and mobile Chrome E2E tests, and changed-file Go lint. Regressions cover
foreign-session notifications, model-driven mode catalog replacement through a
notification, empty config snapshots, and legacy ACK without a config update.
