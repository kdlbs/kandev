---
id: "05-settings-and-support-docs"
title: "Verify native-authority settings and support guidance"
status: done
wave: 4
depends_on:
  - "04-qualified-process-loading"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-002.4
  - AC-AGENTS-COPILOT-PLUGINS-004.4
  - AC-AGENTS-COPILOT-PLUGINS-004.6
  - AC-AGENTS-COPILOT-PLUGINS-005.7
  - AC-AGENTS-COPILOT-PLUGINS-005.8
  - AC-AGENTS-COPILOT-PLUGINS-006.2
  - AC-AGENTS-COPILOT-PLUGINS-006.3
  - AC-AGENTS-COPILOT-PLUGINS-006.4
  - AC-AGENTS-COPILOT-PLUGINS-006.5
  - AC-AGENTS-COPILOT-PLUGINS-006.6
  - AC-AGENTS-COPILOT-PLUGINS-006.7
  - AC-AGENTS-COPILOT-PLUGINS-006.8
  - AC-AGENTS-COPILOT-PLUGINS-007.5
  - AC-AGENTS-COPILOT-PLUGINS-007.6
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 05: Settings and native-authority guidance

## Summary

Verify desktop/phone selection and accurate preparation eligibility after WO-04.
Publish the supported adapter versions, actual native observations, and native
policy ownership without claiming enterprise certification.

## In scope

- Supported discovery must not show the old policy-unverified startup warning.
  Unsupported runtime/context and missing entries remain actionable/removable.
- Preserve WO-02's legacy reason handling, saved model/options, stale-response
  fencing, ordinary Save/Reset, and unrelated drafts.
- Select, Save, reload, Reset, clear, and refresh/probe on desktop and phone.
- Keep native-enabled metadata, deferred preview, accessible/wrapping labels,
  44px phone targets, desktop density, and existing scroll/safe-area controls.
- Update all shipped catalogs plus generated Traditional Chinese/pseudo if
  copy changes. Never describe `mount_permitted` as enterprise policy approval.
- Update `docs/public/agents-and-profiles.md` (how-to): native whole-plugin
  loading, next-process semantics, version-bounded adapters, native policy and
  auth ownership, diagnostic limitations, unsupported context recovery.
- Search README/screenshots for conflicting claims; do not rewrite unrelated
  docs or imply native certification from mocked browser results.
- Primary reconciles companion statuses and evidence without erasing history.

## Out of scope

New activation controls, layout redesign, global native changes, screenshots,
publishing, and administrator/policy operations.

## Acceptance

1. Supported selection/save/probe has no artificial policy gate on desktop or
   phone. Actual unsupported and legacy responses retain clear recovery.
2. Drafts, model/options, keyboard behavior, touch geometry, and preview remain
   correct without a second scroll/save mechanism.
3. Public/internal guidance and actual results agree; no unsupported guarantee
   about enterprise policy, authentication, or exclusive plugin selection.

## ASCII UI preview

[UI-01 in the plan](plan.md#ui-01-native-authority-profile-selection) owns this
shared desktop/phone inline composition:

```text
Plugins                         [Refresh plugins]
[x] fixture-plugin
    Native status: disabled
Whole plugin. Local/worktree. Next process.
                              [Reset] [Save]
```

No policy-certification warning appears for supported preparation. Actual
unsupported/legacy states add wrapping inline recovery. Keep direct profile
navigation, one settings scroller, safe-area Save/Reset, 44px phone/coarse
targets, and desktop density. No new layout/control. Copy is illustrative and
localized. Criteria: 002.4, 004.6, 005.8, 006.2 through 006.8, 007.5/007.6.

## Verification

Run from root. Reuse the installed workspace dependencies; install frozen deps
only after a targeted command fails for missing dependencies. Managed E2E runs
sequentially with fresh assets and disposable identity, never real credentials.

```bash
(cd apps/web && pnpm exec vitest run components/settings/copilot-plugin-profile-selection.test.tsx hooks/domains/settings/use-copilot-plugin-discovery.test.tsx components/settings/copilot-plugin-selection.test.ts 'app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.test.tsx')
(cd apps/web && pnpm exec vitest run components/settings/profile-capability-status.test.tsx components/settings/model-config-resolution-status.test.tsx components/settings/profile-model-fields.test.tsx hooks/domains/settings/use-profile-capability-discovery.test.tsx hooks/domains/settings/use-profile-model-capabilities.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 e2e/helpers/copilot-plugin-fixture.ts e2e/tests/settings/copilot-profile-plugins.spec.ts e2e/tests/settings/mobile-copilot-profile-plugins.spec.ts)
(cd apps/web && pnpm e2e:run --host --project chromium tests/settings/copilot-profile-plugins.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-copilot-profile-plugins.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

For production TS/TSX or locale changes, include every changed path in ESLint
and run:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

When catalogs change, run the generators before those checks:

```bash
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo)
```

Record exact expanded commands/counts. Browser mocks prove settings behavior,
not native enforcement; cite actual WO-03 diagnostics.

## Files likely touched

- Existing Copilot discovery/picker tests and desktop/mobile helper/spec files.
- Settings copy/catalogs only where admission wording needs correction.
- `docs/public/agents-and-profiles.md`; README/screenshots only for conflicts.
- Companion delivery records through the primary conversation.

## Dependencies

WO-04's actual successful caller-wiring results, not privileged qualification.

## Risks

A disappearing warning proves preparation eligibility, not authentication or
policy approval. Older backends can still emit the legacy reason. Mocked
discovery must not hide a broken production default.

## Parallelism

`sequential`: depends on shared WO-04 contracts and final results.

## Inputs

- [Native discovery projection](../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md#discovery-projection-and-settings).
- Plan UI-01, shipped Copilot picker/direct phone route, `/mobile-parity`,
  `/e2e`, `/docs-maintainer`, and actual caller/native evidence.

## Results

Completed after WO-04 caller-boundary delivery. Changes are tests, fixtures,
and the existing public how-to only; no production frontend behavior, copy,
catalog, backend, or layout changed in this slice.

### Delivered coverage

The default discovery fixture and hook tests now describe supported `1.0.88`
preparation with `mount_permitted=true`, rather than the historical policy-only
gate. Native-disabled metadata stays informational. Component coverage retains
unsupported runtime/context and missing-selection removal.

Both desktop and phone cover selected-plugin Save/reload, Reset/clear,
supported discovery refresh and capability/model-option probes, preserved saved
models/options, and legacy blocked-probe recovery. Desktop also exercises
failed refresh and missing saved selection. Phone exercises unsupported-runtime
removal, wrapping labels, zero horizontal overflow, and actual touch geometry.
Existing inline/direct navigation, one scroller, and ordinary Save/Reset remain
unchanged. Tests measure 28px desktop actions and at least 44px phone actions/
rows/refresh, using taps on phones.

Six paths changed in this slice:

```text
apps/web/components/settings/copilot-plugin-profile-selection.test.tsx
apps/web/hooks/domains/settings/use-copilot-plugin-discovery.test.tsx
apps/web/e2e/helpers/copilot-plugin-fixture.ts
apps/web/e2e/tests/settings/copilot-profile-plugins.spec.ts
apps/web/e2e/tests/settings/mobile-copilot-profile-plugins.spec.ts
docs/public/agents-and-profiles.md
```

The hook test was newly modified; the other five paths already contained
earlier work-order edits, which were preserved.

### Final verification

| Check | Result |
| --- | --- |
| First exact Vitest command above | 4 files, 29 tests passed |
| Second exact Vitest command above | 5 files, 38 tests passed |
| Changed frontend ESLint, 16 inputs below | exit 0, zero warnings/errors |
| Desktop discovery | 7 tests in one file |
| Final desktop host E2E | 7/7 passed, exit 0 |
| Mobile discovery | 5 tests in one file |
| Final mobile host E2E | 5/5 passed, exit 0 |
| `node --test scripts/validate-public-docs.test.mjs` | 63 passed |
| `node scripts/validate-public-docs.mjs` | 54 published pages validated |
| `git diff --check` | exit 0 |

Final browser runs were sequential, headless, one worker/one shard, each with
fresh backend, Vite E2E assets, and fixture-plugin builds. Every final source
and spec change was present; no code changed afterward. Exact runner arguments:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/settings/copilot-profile-plugins.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-copilot-profile-plugins.spec.ts
```

`KANDEV_RUN_QUIET_DIR` pointed to an owned private log directory. Final raw
logs are retained there; no Docker or explicit clean invocation was used for
these successful runs.

### Initial failures and correction

No production Red/Green is claimed: WO-04 already supplied the production
behavior, and this work expanded contract coverage. The first desktop Docker
attempt exited 1 without recoverable test counts in its returned output.
The first mobile host attempt passed three and failed two tests on stale
assertions. These failures are not presented as product defects or passing
evidence.

Saved-profile probes carry `profile_id`, with saved settings resolved by the
backend; corrected assertions verify that identity and separately verify the
selected plugin in explicit-refresh launch settings. Removing an unsupported/
missing saved entry detaches its row, so the assertion now expects zero rows
instead of querying a detached checkbox. Persisted-empty-selection and
explicit-refresh checks remain intact. The final 7/7 and 5/5 runs above verify
these corrected specs, superseding the initial failed attempts.

### Expanded ESLint inputs

The changed-path command ran from `apps/web`:

```bash
{ git diff --name-only -- .; git ls-files --others --exclude-standard .; } | grep -E '\.(ts|tsx)$' | sed 's#^apps/web/##' | sort -u | xargs pnpm exec eslint --max-warnings 0
```

Its exact 16 inputs were:

```text
components/settings/copilot-plugin-profile-selection.test.tsx
components/settings/model-config-resolution-status.test.tsx
components/settings/model-config-resolution-status.tsx
components/settings/profile-capabilities-row.tsx
components/settings/profile-capability-status.test.tsx
components/settings/profile-capability-status.tsx
components/settings/profile-model-fields.test.tsx
components/settings/profile-model-fields.tsx
e2e/helpers/copilot-plugin-fixture.ts
e2e/tests/settings/copilot-profile-plugins.spec.ts
e2e/tests/settings/mobile-copilot-profile-plugins.spec.ts
hooks/domains/settings/use-copilot-plugin-discovery.test.tsx
hooks/domains/settings/use-profile-model-capabilities.test.tsx
hooks/domains/settings/use-profile-model-capabilities.ts
hooks/domains/settings/use-profile-model-options.ts
lib/types/http-agents.ts
```

Typecheck/i18n/generators were not repeated: this slice added no production
TS/TSX, user copy, or catalogs; WO-02 retains the prior checks for its unchanged
production changes. The primary owns final catalog, all-spec lint, actual-diff
documentation coverage, relative-link/anchor, size, and whitespace reconciliation
in the plan. Its independent linter regression suite passed all 36 tests.

### Documentation and integrity

Public docs updated: `docs/public/agents-and-profiles.md`, retaining its how-to
structure. It now explains whole-plugin/additive/nonexclusive loading,
supported adapters, selected managed interactive runtime, local/worktree and
next-process semantics, deferred preview, native authentication/policy
ownership, unsupported repair, and legacy recovery as an older-backend state.
The native observations remain bounded: eight nonzero attempts, partial
1.0.104 coexistence, no authoritative disable denial, uncompleted 1.0.88 paired
disable, and no managed-policy test or enterprise certification. README and
screenshot-catalog searches found no conflicting claims, so neither changed.

Browser mocks prove settings behavior, not native enforcement. At work-order
completion, no staging, commit, push, live instance, credentials, native policy,
or runtime pin changed. PR publication is a subsequent user-authorized step.
