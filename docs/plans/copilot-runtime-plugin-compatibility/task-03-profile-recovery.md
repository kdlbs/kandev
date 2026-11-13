---
id: "03-profile-recovery"
title: "Correct profile compatibility recovery"
status: done
wave: 2
depends_on:
  - "01-runtime-compatibility-preview"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.4
  - AC-AGENTS-COPILOT-PLUGINS-001.6
  - AC-AGENTS-COPILOT-PLUGINS-002.4
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-006.2
  - AC-AGENTS-COPILOT-PLUGINS-006.3
  - AC-AGENTS-COPILOT-PLUGINS-006.5
  - AC-AGENTS-COPILOT-PLUGINS-006.6
  - AC-AGENTS-COPILOT-PLUGINS-006.7
  - AC-AGENTS-COPILOT-PLUGINS-006.8
  - AC-AGENTS-COPILOT-PLUGINS-006.9
  - AC-AGENTS-COPILOT-PLUGINS-006.10
  - AC-AGENTS-COPILOT-PLUGINS-007.4
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 03: Correct profile compatibility recovery

## Summary

Explain the actual runtime rejection and show unknown native status without
changing saved selections. Preserve the profile editor's desktop/phone
recovery flow and update existing support guidance.

## In scope

- Project the shared `supported_runtime_versions` into concrete Copilot
  discovery responses and the browser type. Keep cache copies and runtime
  request fencing correct for the new slice.
- Use current version/reason metadata for localized runtime recovery. Cover
  old responses without metadata, stale requests, unknown reasons, wrapped
  launches, and legacy policy responses without inventing a supported version.
- Change the row view model to `nativeEnabled: boolean | null`. Fresh rows
  preserve true/false, even with unavailable roots. Missing/stale/unobserved
  identities use null and localized unknown status.
- Extend row, selector, and discovery tests. Preserve missing-row removal,
  dirty state, unrelated drafts, model selections, Save/Reset, and reload behavior.
- Extend the existing desktop/phone profile E2E fixtures and all shipped
  catalogs. Generate Traditional Chinese and pseudo translations.
- Update the Copilot plugin section of `docs/public/agents-and-profiles.md`.
  This is existing how-to guidance: explain the runtime warning and accurate
  recovery. List 1.0.105 only after task 02 actually qualifies it.

## Out of scope

Native activation changes, profile-storage migrations, hot reload, automatic
selection removal, new probe transports, and another UI settings framework.

## Acceptance

1. A saved identity without inventory fails the new unknown-status assertion
   before correction. Fresh true/false and unavailable-root controls retain their values.
2. Unsupported-runtime recovery shows current metadata and server-qualified
   alternatives. Stale or old-server responses use safe fallback guidance.
3. Desktop and phone users can remove, Reset, Save, and reload the selection
   without losing unrelated drafts. Localization and public-doc gates pass.

## ASCII UI preview

UI-02 from the [combined preview](plan.md#ui-02-profile-recovery-with-an-unobserved-saved-identity),
for 001.4/001.6, 002.4, and 006.2/006.3/006.5 through 006.10:

```text
Plugins [Refresh]
Copilot <current version>: plugin adapter unavailable
Qualified: <server list>
Select a qualified runtime, or uncheck plugins and Save.
[x] example
    Native status: unknown
                                  [Reset] [Save]
```

The phone uses the same direct profile route with wrapping labels and at least
44px hit targets. Keep one settings scroller and existing safe-area Save/Reset.
Fresh observed disabled rows still say disabled. Copy is illustrative.

## Verification

From the repository root, with the task 01 workspace bootstrap complete:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers -run '^TestCopilotPlugin' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/copilot-plugin-selection.test.ts components/settings/copilot-plugin-profile-selection.test.tsx hooks/domains/settings/use-copilot-plugin-discovery.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/settings/copilot-plugin-selection.ts components/settings/copilot-plugin-profile-selection.tsx hooks/domains/settings/use-copilot-plugin-discovery.ts lib/types/copilot-plugin-discovery.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/copilot-profile-plugins.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-copilot-profile-plugins.spec.ts)
bash scripts/lint-go-changed apps/backend/internal/agent/settings/dto/copilot_plugin_discovery.go apps/backend/internal/agent/settings/controller/copilot_plugin_discovery.go apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_review_test.go apps/backend/internal/agent/settings/handlers/copilot_plugin_discovery_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run new causal regressions alone for Red before the correction. Run the listed
changed suites for final Green. Extend exact lint/test inputs for any additional
changed files. Record measured target sizes, wrapping/overflow, and actual
counts, not inherited results from companion packages.
Run the repository PR-documentation coverage preflight against actual changed
paths and referenced contents before marking the package complete.

## Files likely touched

- `apps/backend/internal/agent/settings/dto/copilot_plugin_discovery.go`.
- `apps/backend/internal/agent/settings/controller/copilot_plugin_discovery.go` and `copilot_plugin_discovery_review_test.go`.
- `apps/backend/internal/agent/settings/handlers/copilot_plugin_discovery_test.go`.
- `apps/web/lib/types/copilot-plugin-discovery.ts`.
- `apps/web/components/settings/copilot-plugin-selection.ts`, `copilot-plugin-profile-selection.tsx`, and their tests.
- `apps/web/hooks/domains/settings/use-copilot-plugin-discovery.test.tsx` and the hook if response handling needs correction.
- `apps/web/e2e/helpers/copilot-plugin-fixture.ts`.
- `apps/web/e2e/tests/settings/copilot-profile-plugins.spec.ts` and `mobile-copilot-profile-plugins.spec.ts`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/agents.json`.
- `docs/public/agents-and-profiles.md` and the owning plugin designs only for final implementation reconciliation.

## Dependencies

Task 01 supplies the shared qualified-version helper. Task 02 is not a
prerequisite for accurate failure feedback. Keep support claims conditional
on its actual native result.

## Risks

The current hook can retain same-context stale rows after a failed refresh.
Do not mistake retained booleans for a current observation or let earlier
runtime responses overwrite the new warning. Missing server metadata must not
cause a fabricated compatibility claim.

## Parallelism

`sequential`

## Inputs

- Requirements 001, 002, 005, 006, and 007.
- Native-authority runtime amendment and parent Editor state/phone composition.
- Existing selector, discovery, and profile E2E fixtures.
- Public Copilot plugin how-to and the sole compatibility ledger.

## Results

Implemented `supported_runtime_versions` on the concrete Copilot discovery DTO
and browser type, version/reason-aware recovery copy with safe fallbacks, and
`nativeEnabled: boolean | null` for fresh versus unobserved rows. Saved
selections remain removable; fresh disabled observations remain disabled;
missing and stale rows render unknown status. Public Copilot plugin guidance now
lists `1.0.105` and explains the runtime warning and recovery.

Evidence:

- Controller discovery tests cover the platform-scoped shared supported list and
  an unsupported runtime without native commands.
- Selector/profile tests cover fresh unavailable roots, stale observations,
  missing identities, unknown native status, current-version recovery,
  missing metadata, wrapped launch context, legacy policy, and unknown-reason
  fallbacks. Stale or loading contexts take precedence over retained rejection
  metadata, and a definite start-blocked warning requires a current completed
  observation.
- Chromium profile tests passed 8 tests, including the new runtime recovery and
  unknown-status case. Mobile-chrome passed 6 tests, including the same recovery
  with touch targets and no horizontal overflow.
- Public-doc validation, specification validation, all-spec lint, typecheck,
  i18n check/ratchet, and changed-file Oxlint passed.
