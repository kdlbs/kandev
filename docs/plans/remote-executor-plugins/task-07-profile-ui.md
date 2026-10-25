---
id: "07-profile-ui"
title: "Configure and select providers on desktop and phone"
status: done
wave: 7
depends_on:
  - "03-profile-admission"
  - "06-plugin-administration"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-EXECUTORS-PLUGIN-007
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-001.1
  - AC-EXECUTORS-PLUGIN-001.2
  - AC-EXECUTORS-PLUGIN-001.3
  - AC-EXECUTORS-PLUGIN-002.2
  - AC-EXECUTORS-PLUGIN-007.1
  - AC-EXECUTORS-PLUGIN-007.2
  - AC-EXECUTORS-PLUGIN-007.3
  - AC-EXECUTORS-PLUGIN-007.4
  - AC-EXECUTORS-PLUGIN-007.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 07: Configure and select providers on desktop and phone

## Summary

Render provider profile fields and availability from the host catalog. Preserve normal executor selection and profile routing with an intentional phone composition.

## In scope

- Add schema-driven scalar fields, redacted secrets, validation, save/reload and unavailable-provider states to existing executor settings routes.
- Use host descriptors in task, agent-profile and workflow selectors. Preserve selected unavailable profiles; prevent incompatible choices without changing saved values.
- Implement UI-01 and UI-02, all six supported language catalogs and generated pseudo locale. Test desktop, phone, loading, validation, no providers and provider loss.

## Out of scope

- Custom provider JavaScript forms, new settings navigation architecture, lifetime status drawer.

## Acceptance

- Desktop and phone users save/reload a profile and select it through the real task flow; secret values never appear in browser reads.
- Provider loss or stale catalogs cannot silently select another executor; backend rejection is visible and preserves form state.
- Touch targets, page/drawer scroll containment, keyboard focus, 767/768px behavior and localized copy match the previews.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

### UI-01: Provider profile settings

Entry: Settings > Executors > provider > profile. State: editable bounded profile.

```text
Desktop
Executors / Provider / Profile
[Name                      ]
[Image                     ]  [Region                ]
[Credential: configured    ]  [Replace] [Clear]
Workspace ends with compute. Maximum lifetime: 8 hours.
[field validation error, when present]
                                           [Save]

Phone: dedicated settings page
< Executors       Profile
Name
[                       ]
Image
[                       ]
Region
[                       ]
Credential: configured
[Replace] [Clear]
Workspace ends with compute.
Maximum lifetime: 8 hours.
-------------------------
[          Save         ]
```

The phone page body scrolls; the Save region clears the safe area and keyboard.
The desktop layout may group short fields; the phone form has one column.
The 8-hour value is fixture data, not a host constant. Save errors preserve edits.
Provider absent: keep fields and selection visible, disable Save, and show a reason.
Loading: show status without replacing saved values. No providers: link to plugin settings.
Host strings use translations; provider strings use the plugin localization contract.
Covers AC-EXECUTORS-PLUGIN-001.1 through 001.3 and 007.1 through 007.5.

### UI-02: Executor selection

Entry: task creation or existing executor profile selector. State: available and unavailable choices.

```text
Desktop: existing picker
Executor [Build profile v]
  Build profile       Ready
    Workspace ends with compute; maximum 8 hours
  Previous profile    Provider unavailable (disabled)

Phone: temporary inset picker drawer
+-----------------------------+
| Choose executor       Close |
| [Search                   ] |
| Build profile               |
| Workspace ends with compute |
| Maximum 8 hours             |
| Previous profile            |
| Provider unavailable        |
+-----------------------------+
```

Use the current selector's shared model. Unavailable choices are identifiable but cannot launch.
An already selected unavailable profile remains selected until the user chooses another.
The drawer body scrolls independently; header and safe-area padding stay fixed.
Covers AC-EXECUTORS-PLUGIN-001.1, 001.3, 002.2 and 007.1 through 007.5.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
(cd apps/web && pnpm exec vitest run components/settings/plugin-executor-profile.test.tsx components/settings/executor-profile-navigation.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/plugin-executor-profiles.spec.ts tests/settings/executor-profile-routing.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-plugin-executor-profiles.spec.ts tests/settings/mobile-executor-profile-routing.spec.ts)
```

Required evidence:

- `components/settings/plugin-executor-profile.test.tsx: schema validation, secret replacement, unavailable selection`
- `e2e/tests/settings/plugin-executor-profiles.spec.ts and mobile-plugin-executor-profiles.spec.ts: profile save/reload and task selection`

## Files likely touched

- `apps/web/components/settings/executor-profiles-card.tsx and profile-edit/`
- `apps/web/lib/settings/executor-settings-routes.ts`
- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/components/ task and profile selectors`
- `apps/web/components/settings/ (new plugin-executor-profile.test.tsx)`
- `apps/web/e2e/tests/settings/plugin-executor-profiles.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-plugin-executor-profiles.spec.ts (new)`
- `apps/web/src/locales/`

## Dependencies

[Task 03](task-03-profile-admission.md), [Task 06](task-06-plugin-administration.md).

## Risks

Boot payload and live catalog updates can race. Use shared projection and preserve user edits. Do not mount hidden duplicate forms.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Implemented the schema-driven provider profile editor with shared settings save coordination, secret redaction and clearing, retention details, unavailable states, dynamic provider selection, and phone drawer composition. The packaged fixture exercises profile create/edit/read and task-flow selection.

Validation passed:

- `(cd apps/web && pnpm exec vitest run components/settings/plugin-executor-profile.test.tsx components/settings/executor-profile-navigation.test.tsx components/settings/executor-profiles-card.test.tsx components/task-create-dialog-computed.test.ts components/task-create-dialog-options.test.tsx components/task-create-dialog-selectors.test.tsx lib/agent-executor-compat.test.ts)` — 7 files, 88 tests.
- `(cd apps/web && pnpm run typecheck)`.
- Targeted ESLint on changed UI, selectors, helpers, and E2E files.
- `(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)`.
- `(cd apps/web && pnpm e2e:run --project chromium tests/settings/plugin-executor-profiles.spec.ts tests/settings/executor-profile-routing.spec.ts)` — 2 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-plugin-executor-profiles.spec.ts tests/settings/mobile-executor-profile-routing.spec.ts)` — 2 passed.
- `(cd apps/backend && go test -race ./cmd/plugin-fixture ./internal/plugins/manifest ./internal/plugins/... -run 'TestPluginExecutor|TestValidateExecutorProviders' -count=1)`.
- `make -C apps/backend e2e-plugin-package`.
