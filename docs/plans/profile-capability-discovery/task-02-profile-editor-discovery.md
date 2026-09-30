---
id: "02-profile-editor-discovery"
title: "Profile editor refresh and end-to-end delivery"
status: done
wave: 2
depends_on: ["01-profile-probe-contract"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
  - REQ-AGENTS-PROFILE-DISCOVERY-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-DISCOVERY-001.1
  - AC-AGENTS-PROFILE-DISCOVERY-001.2
  - AC-AGENTS-PROFILE-DISCOVERY-001.3
  - AC-AGENTS-PROFILE-DISCOVERY-001.4
  - AC-AGENTS-PROFILE-DISCOVERY-001.5
  - AC-AGENTS-PROFILE-DISCOVERY-001.6
  - AC-AGENTS-PROFILE-DISCOVERY-002.1
  - AC-AGENTS-PROFILE-DISCOVERY-002.2
  - AC-AGENTS-PROFILE-DISCOVERY-002.3
  - AC-AGENTS-PROFILE-DISCOVERY-002.4
  - AC-AGENTS-PROFILE-DISCOVERY-002.5
  - AC-AGENTS-PROFILE-DISCOVERY-003.1
  - AC-AGENTS-PROFILE-DISCOVERY-003.2
  - AC-AGENTS-PROFILE-DISCOVERY-003.3
  - AC-AGENTS-PROFILE-DISCOVERY-003.4
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
---
# Task 02: Profile editor refresh and end-to-end delivery

## Summary

Connect concrete profile editors to profile discovery for both baseline models and dependent options.
Deliver explicit draft refresh, stale-response protection, desktop/phone coverage, and public guidance.
Use the real backend and a deterministic mock subprocess for the regression.

## In scope

- Pass profile identity and the complete launch-setting draft through the form, API client, and discovery hooks.
- Discover saved profiles on open and require explicit Refresh after draft launch changes.
- Show stale, loading, ready, and retryable failure states without selecting models or discarding draft values.
- Preserve existing save rules, with no new environment-only save deadlock.
- Preserve context-free workflow and agent-wide callers and the gateway bypass.
- Add fixture catalog variants from environment and CLI inputs, plus prefix invocation evidence.
- Add desktop and phone E2E, translations in all six catalogs, and public refresh guidance.

## Out of scope

- New picker architecture, new navigation, automatic package installation, and runtime updater redesign.
- Remote capability discovery and changes to live-session model identity.

## Acceptance

1. Saved and unsaved profile launch settings drive the visible model list and option snapshot, with late results rejected.
2. Failure, refresh, and context changes preserve selections and edits; keyboard and phone users can refresh, retry, select, and save.
3. Real-route E2E proves env/flag/prefix forwarding and profile isolation; context-free consumers pass their existing regression suites.

## ASCII UI preview

See [UI-01 through UI-03](plan.md#ascii-ui-preview) for the full state set.
This excerpt is structural, not a pixel specification.

UI-01 desktop:

```text
Start model                             [Refresh]
[6 Sol / High                              v]
Launch settings changed. Refresh models.
```

UI-01 phone:

```text
Start model
[6 Sol / High              v]
Launch settings changed.
[Refresh models]
```

UI-02 matching result retains the selected model and adds the new advertised choices.
UI-03 uses the same status region for progress or a sanitized retry message.
Reuse the existing picker scroll region and focus behavior.
Phone actions have at least 44px touch targets; status text wraps without page overflow.
These previews cover AC-AGENTS-PROFILE-DISCOVERY-003.1 through 003.4.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree before package commands.
The E2E runner rebuilds and tears down its isolated application. Run the commands sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/settings/use-dynamic-models.test.ts hooks/domains/settings/use-profile-model-capabilities.test.tsx components/settings/profile-form-fields.test.tsx components/settings/profile-model-options.test.tsx components/settings/workflow-session-config-editor.test.tsx lib/api/domains/settings-api.test.ts)
(cd apps/web && pnpm exec eslint 'app/settings/agents/[agentId]/agent-setup-parts.tsx' components/settings/agent-profile-page.tsx components/settings/mode-combobox.tsx components/settings/profile-capability-helpers.tsx components/settings/profile-capability-status.tsx components/settings/profile-form-fields.test.tsx components/settings/profile-form-fields.tsx components/settings/profile-model-fields.tsx components/settings/profile-capabilities-row.tsx components/settings/profile-model-options.test.tsx hooks/domains/settings/use-profile-model-capabilities.ts hooks/domains/settings/use-profile-model-capabilities.test.tsx hooks/domains/settings/use-profile-model-options.ts lib/api/domains/profile-capability-api.ts lib/api/domains/settings-api.ts lib/api/domains/settings-api.test.ts lib/types/http-agents.ts e2e/helpers/profile-capability-discovery.ts e2e/tests/settings/profile-capability-discovery.spec.ts e2e/tests/settings/mobile-profile-capability-discovery.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/backend && go test -tags fts5 ./cmd/mock-agent)
(cd apps/web && pnpm e2e:run --host --project chromium tests/settings/profile-capability-discovery.spec.ts tests/settings/agent-profile-acp.spec.ts tests/settings/agent-profile-cli-flags.spec.ts tests/workflow/workflow-settings.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-profile-capability-discovery.spec.ts tests/settings/mobile-agent-profile-config-selector.spec.ts tests/workflow/mobile-workflow-settings.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The profile hook and the two discovery E2E files above are new files delivered by this task.
The remaining named test paths exist in the current repository.
Add tests to the exact command list if implementation changes another suite.
Extend the focused ESLint command when implementation touches additional TypeScript files.
Run the repository documentation-coverage preflight on the resulting diff.

Desktop E2E covers saved profile isolation, draft refresh before save, reload after selection,
ordered flags and a supported wrapper. Hook and component tests cover stale response rejection and retryable model-option failures.
Phone E2E repeats refresh and selection through touch controls, verifies target size and containment,
and confirms that dependent options remain associated with the latest launch context.
Use the test-only mock child catalog mode through real probe routes. Never call a paid model or read developer credentials.
A browser-intercepted response alone does not satisfy the environment-forwarding regression.

## Files likely touched

- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/lib/types/http-agents.ts`
- `apps/web/hooks/domains/settings/{use-dynamic-models.ts,use-profile-model-capabilities.ts}`
- `apps/web/components/settings/{profile-form-fields.tsx,profile-model-fields.tsx,profile-capability-helpers.tsx,profile-capability-status.tsx}`
- Profile route/editor callers that hold the environment draft and saved identity
- Adjacent hook, component, and API-client tests
- `apps/backend/cmd/mock-agent/` fixture handler, flags, and tests (read its scoped AGENTS.md)
- New `apps/web/e2e/tests/settings/profile-capability-discovery.spec.ts`
- New `apps/web/e2e/tests/settings/mobile-profile-capability-discovery.spec.ts`
- Existing settings/workflow E2E when request assertions change
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/` affected namespace catalogs
- `docs/public/agents-and-profiles.md`
- This package and its paired draft specs for final lifecycle/result updates

## Dependencies

Task 01. Use its validated API and launch identity; do not reconstruct secret values in the browser.

## Risks

- Environment fields can live above the model form. A narrowed prop type must not silently drop them again.
- Shared frontend cache state can mix two profiles even when backend results are correct.
- An automatic model-options request can accidentally execute a dirty draft before explicit refresh.
- Existing save contributors can block repairs if stale discovery is treated as pending model reconciliation.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/profile-capability-discovery.md), all requirements.
- [Design](../../specs/agents/system-design/profile-capability-discovery.md), editor state and responsive behavior.
- Existing `mobile-agent-profile-config-selector.spec.ts`, `agent-profile-cli-flags.spec.ts`, and mock-agent patterns.
- [Historical dynamic options delivery](../dynamic-provider-options/plan.md), for preserved model-resolution semantics.

## Results

Done. Saved profiles discover their own models and options on open. Draft changes remain stale until explicit refresh;
both baseline and dependent-option requests carry the same complete launch snapshot and profile identity.
Equivalent option-value objects no longer restart dependent discovery on rerender, and the test suite covers that regression.
The editor preserves selected models and draft values on failure, retains existing save coordination, and supports retry.
Desktop and phone flows use localized status, keyboard and touch controls, and retain the existing picker behavior.
The deterministic mock subprocess records actual environment, flags, and supported wrapper invocation. Public guidance is in
[`agents-and-profiles.md`](../../public/agents-and-profiles.md).

Verification passed:

- Focused Vitest command above: 6 files and 65 tests passed.
- Focused ESLint command above: passed with zero warnings.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- `pnpm run build:vite` passed. The managed E2E runner also built the backend and pseudo-locale web assets.
- `go test -tags fts5 ./cmd/mock-agent -count=1` passed.
- Desktop E2E command above: 27 tests passed, including the profile refresh and existing ACP, CLI-flag, and workflow tests.
- Mobile E2E command above: 10 tests passed, including profile refresh, picker, and workflow tests.
- Public documentation, specification catalog/lint, and PR documentation-coverage checks passed.
- Final `git diff --check` passed.
