---
id: "03-settings-presentation"
title: "Native check presentation, locales, E2E, and docs"
status: done
wave: 3
depends_on:
  - "02-status-integration"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-001
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.6
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-002.3
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4
system_design:
  - ../../specs/agents/system-design/runtime-native-update-checks.md
---

# Task 03: Native check presentation, locales, E2E, and docs

## Summary

Show native check results in the Settings runtime rows and update
dialog/drawer: latest version, channel, up to date, or a localized reason. Do
not show the stable-release reference. Cover desktop and phone flows with
Playwright, and update the public OMP update guide.

## In scope

- Optional `check_source`, `update_channel`, and `check_reason` fields, plus
  `AgentUpdateCheckReason`, in `agent-update-api.ts`. The reason type mirrors
  the closed backend set, including `cancelled`. Preview fields mirror
  Task 02.
- `RuntimeOverviewRow` channel label and reason-specific `rowDescription`.
- `RuntimeVersionSummary` native self-update branch with test ID
  `agent-update-native-check-<agent>` and without
  `agent-update-stable-reference-<agent>`.
- New `agents` locale keys in `en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`,
  and `ko`. Do not use an em dash.
- E2E fixture options for native status and preview responses. Add desktop and
  phone scenarios.
- `docs/public/agents-and-profiles.md` OMP update section: Kandev uses
  `omp update --check` for status and the configured channel. Remove the
  stable-reference wording for OMP.

## Out of scope

- Backend changes. Report any backend contract mismatch instead of patching it
  here.
- Runtime policy UI, bulk update, and other agents' rows.

## Acceptance

- Unit tests prove that a native available row shows the latest version and
  channel, an up-to-date row shows the up-to-date description, each check
  reason maps to its own description, and an unrecognized reason falls back to
  `agents:runtimeCheckUnknown`. The native dialog summary renders the
  native-check line and no stable-reference element.
- Desktop and phone Playwright tests pass and match UI-01 to UI-03 in the plan
  structurally. The phone test shows no horizontal overflow and a full-width
  action of at least 44px. Approval still sends `{}` and streams output.
- `pnpm run i18n:check`, `pnpm run typecheck`, `pnpm run lint`, and
  `node scripts/validate-public-docs.mjs` (which validates `docs/public`) pass.
  The specification catalog, specification lint, and scoped diff/status gates
  below pass. The Results section records actual output. The PR-documentation
  coverage check (`.github/scripts/pr-docs.cjs`) reads PR metadata from GitHub
  and cannot run before a PR exists. After the PR opens, record its CI result
  in the plan's verification results.

## ASCII UI preview

See [plan UI-01 to UI-03](plan.md#ascii-ui-preview). The required structure is
one native result line instead of the stable reference, a channel line only
when reported, and a reason sentence for unknown results. Existing controls and
test IDs remain unchanged.

```text
| omp   18.13.3 -> 18.22.0-canary.3                      [ Review update ]  |
|       Channel: canary                                                     |
```

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/agent-runtime-update-manager.test.tsx components/settings/agent-runtime-update-control.test.tsx lib/api/domains/agent-update-api.test.ts lib/agents/runtime-update-manager.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run lint)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-self-update.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-self-update.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs apps
git status --short -- docs/plans/native-agent-update-checks docs/specs/agents docs/decisions
```

## Files likely touched

- `apps/web/lib/api/domains/agent-update-api.ts`
- `apps/web/components/settings/agent-runtime-update-rows.tsx`
- `apps/web/components/settings/runtime-version-summary.tsx`
- `apps/web/components/settings/agent-runtime-update-manager.test.tsx`
- `apps/web/components/settings/agent-runtime-update-control.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/agents.json`
- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`
- `apps/web/e2e/tests/settings/agent-self-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-self-update.spec.ts`
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 02.

## Risks

- Translation completeness gates the build. Every new key needs all seven
  locales. `canary` may need a `_verbatim.json` declaration if a locale keeps
  the English word.
- Existing self-update E2E tests assert the stable reference for OMP. Update
  them to native fixtures instead of keeping both assertions.

## Parallelism

`sequential`

## Inputs

- Task 02 DTO fields and reason set.
- [System design: Web presentation](../../specs/agents/system-design/runtime-native-update-checks.md#web-presentation)
- `/mobile-parity`, `/e2e`, `/docs-maintainer` skills.

## Results

Done on 2026-11-17.

Implementation notes:

- `nativeCheckReasonKey` in `lib/agent-runtime-update.ts` maps the closed
  reason set (own-property lookup) to `agents:nativeCheckReason*` keys and falls
  back to `agents:runtimeCheckUnknown`. Rows and the dialog share it.
- Row channel test ID: `runtime-update-channel-<agent>`. Dialog test IDs:
  `agent-update-native-check-<agent>` and `agent-update-channel-<agent>`.
- The native dialog unit tests live in the new
  `components/settings/agent-runtime-update-native-check.test.tsx`, because
  appending them to `agent-runtime-update-control.test.tsx` exceeded the Oxlint
  600-line file limit. That file is unchanged.
- E2E fixtures gained a `native` option. The desktop spec covers the native
  canary row (UI-01), the dialog without a stable reference (UI-02), empty
  approval, streamed output, and a new unknown-reason scenario. The phone spec
  covers the drawer native result, channel, full-width 44px action, and no
  horizontal overflow (UI-03 structure, with an available result).
- New `agents` keys exist in all seven locales; `zh-hk`/`zh-tw` come from
  `pnpm run i18n:zh-hant` and `pseudo` from `pnpm run i18n:pseudo` (unrelated
  pseudo drift in `system.json` and `task.json` was reverted).
- `docs/public/agents-and-profiles.md` "Update Oh My Pi on the Kandev host"
  now describes `omp update --check`, channel display, reason explanations,
  cache and recheck behavior, and drops the stable-reference wording.
- E2E was written after the component change; red evidence for the
  presentation came from the unit tests (missing
  `agent-update-native-check-omp-acp` before implementation).

| Command | Result |
| --- | --- |
| `(cd apps && pnpm install --frozen-lockfile)` | The recorded store (`~/.kandev/.pnpm-store`) is off the cache drive and `codeload.github.com` refused connections, so the install ran as `pnpm install --frozen-lockfile --offline --store-dir ~/.cache/pnpm-store --config.confirm-modules-purge=false`: `Done in 20.4s using pnpm v9.15.9` |
| `(cd apps/web && pnpm exec vitest run components/settings/agent-runtime-update-manager.test.tsx components/settings/agent-runtime-update-control.test.tsx lib/api/domains/agent-update-api.test.ts lib/agents/runtime-update-manager.test.ts)` plus `components/settings/agent-runtime-update-native-check.test.tsx` | `Test Files 5 passed (5)`, `Tests 41 passed (41)` |
| `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run lint)` | typecheck exit 0; `i18n keys OK ... ja, ko, pt-pt, zh-cn, zh-hk, zh-tw complete`, no em dashes, no non-JSX copy; `oxlint ... --deny-warnings` exit 0 |
| `(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-self-update.spec.ts)` | `4 passed (22.6s)` |
| `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-self-update.spec.ts)` | `2 passed (18.9s)` |
| `node scripts/validate-public-docs.mjs` | `Validated 55 published docs pages.` |
| `python3 scripts/list-docs.py validate` | `Validated 456 decisions and 2032 specifications.` |
| `python3 scripts/lint-spec-files.py --all` | `All specification files passed.` |
| `git diff --check -- docs apps` | exit 0, no output |
| `git status --short -- docs/plans/native-agent-update-checks docs/specs/agents docs/decisions` | Only the design package files (four modified, four untracked), as expected |
| PR documentation coverage (`.github/scripts/pr-docs.cjs`) | Pending: needs PR metadata |
