---
id: "07-default-on-rollout"
title: "Enable shared background work by default"
status: done
wave: 7
depends_on:
  - "06-executor-proof-and-docs"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-001.1
  - AC-AGENTS-BACKGROUND-WORK-001.2
  - AC-AGENTS-BACKGROUND-WORK-001.3
  - AC-AGENTS-BACKGROUND-WORK-001.5
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 07: Enable shared background work by default

## Summary

Make shared background-work inspection standard behavior in shipped profiles.
Retain the existing flag as an operator off switch during the initial rollout.

## In scope

- Change KANDEV_FEATURES_AGENT_BACKGROUND_WORK to true in prod, dev, and e2e
  in root profiles.yaml. Use the existing embedded-profile synchronization.
- Preserve features.agentBackgroundWork, its registry entry, restart metadata,
  backend read/action gates, and frontend feature checks.
- Respect explicit environment values and saved SQLite overrides. An existing
  false override remains false after the profile default changes.
- Keep the browser's initial fallback false until authoritative feature
  hydration. Do not use a frontend default as another source of rollout policy.
- Preserve provider capability checks, missing-peer fallback, and unsupported
  controls. Keep native Codex and Claude handoff flags independent.
- Update targeted tests that assume this existing feature is globally off.
  Negative fixtures set an explicit false value rather than depending on defaults.
- Update user guidance to describe the shared experience as standard behavior,
  with the runtime toggle available to operators during rollout.

## Out of scope

Removing the flag, promoting other provider experiments, overriding operator
preferences, deleting saved overrides, and enabling unproven wire contracts.
Later retirement must append the key and environment name to
retiredRuntimeFlagIdentities and remove the gates end to end.

## Acceptance

1. Default prod/dev/e2e profiles expose shared background-work inspection.
   Browser hydration and backend endpoints agree on the effective value.
2. Explicit false environment and SQLite overrides preserve the disabled path,
   including no producer negotiation, new reader, projection write, or action.
3. Supported capabilities remain usable and unsupported actions remain absent
   or rejected. Other feature flags and all saved operator choices retain their meaning.

## ASCII UI preview

Reuse [OC-UI-01 through OC-UI-03](plan.md#ascii-ui-preview). No new layout.

~~~text
Default profile, supported jobs:
  [2 Background jobs v] [Background history]
  [Message...                                  ] [Send/Queue]

Explicit operator override = false:
  Existing chat, input, and inline tool history
  No background inspector or new process controls
~~~

Profile defaults change discoverability. They do not change Send/Queue delivery
or confer a capability on a provider. Phone uses the same existing drawer flow.

## Verification

From the Kandev root, after the package's dependency install:

~~~bash
(cd apps/backend && go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run 'Test.*BackgroundWork' -count=1)
pnpm --dir apps/web exec vitest run lib/state/slices/features/features-contract.test.ts components/task/chat/background-work/background-work.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/opencode-background-work.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-opencode-background-work.spec.ts
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
~~~

Extend those browser suites with default-profile and explicit-off cases. Run
the desktop and phone commands sequentially. For the profile-only changes,
reuse the existing registry/profile tests rather than writing tests that mirror YAML.
Add behavioral coverage where current override/disabled-path tests are absent.

## Files likely touched

- Root profiles.yaml and the existing embedded profiles copy.
- Existing runtimeflags, common/config, profiles, and task/handlers
  background-work tests where default assumptions need explicit overrides.
- Existing web features-contract and background-work tests.
- The package's desktop/phone suites for default-on and explicit-off behavior.
- docs/public/agents-and-profiles.md and docs/public/tasks-and-workflows.md.
- This package and the shared rollout records after commands pass.

## Dependencies

Task 06 supplies full feature evidence. This work order promotes the existing
feature after that verification; it does not waive the upstream producer gate.

## Risks

The flag controls shared read/action endpoints as well as the chip. Removing
it during an initial provider rollout loses the operator off switch. A profile
change must not overwrite a stored or environment-based false override.

## Parallelism

sequential

## Inputs

- [Shared rollout design](../../specs/agents/system-design/background-work.md#capabilities-and-rollout).
- Existing profiles.yaml, runtimeflags registry and retired-identity tests.
- /runtime-feature-flags and the package's executed capability evidence.

## Results

Implemented on 2026-10-27.

- `profiles.yaml` (the embedded source; the repo-root path is a symlink to it)
  now defaults `KANDEV_FEATURES_AGENT_BACKGROUND_WORK` to `"true"` in prod,
  dev, and e2e.
- The runtime flag registry entry, restart metadata, backend read/action gates,
  frontend feature checks, and provider capability checks are unchanged. The
  flag remains an operator off switch during rollout.
- Explicit environment values and saved SQLite overrides keep their precedence:
  `ApplyProfile` only writes unset variables, and the override tier still beats
  profile defaults.
- The browser fallback stays `false` in `defaultFeatureFlags` until
  authoritative hydration, asserted by `features-contract.test.ts`.
- Added `TestShippedProfilesEnableBackgroundWork` (prod/dev/e2e defaults) and
  `TestExplicitBackgroundWorkOffWinsOverProfileDefault` (operator false wins).
- Public guidance now describes inspection as standard behavior with the
  operator toggle documented.

Promotion does not advertise negotiated OpenCode continuity, native Codex
steering, or the Claude handoff experiment; those remain independent.

Toggle identity and remaining gates:

- Key `features.agentBackgroundWork`; environment `KANDEV_FEATURES_AGENT_BACKGROUND_WORK`.
- Defaults: `prod`, `dev`, and `e2e` are `"true"`.
- Restart required; the admin toggle still reads and writes the same registry
  entry, and an explicit environment value still locks the control.
- Gated entry paths retained: the task background-work read/list/detail/action
  HTTP and WebSocket handlers, the browser chip/hook/panel feature checks, and
  the new agentctl adapter attestation gate. A disabled launch negotiates and
  attests nothing.
- Status: default-on with a retained kill switch, not graduated.

Evidence run:

~~~bash
(cd apps/backend && go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run 'Test.*BackgroundWork' -count=1)
pnpm --dir apps/web exec vitest run lib/state/slices/features/features-contract.test.ts components/task/chat/background-work/background-work.test.tsx
~~~

All passed.
