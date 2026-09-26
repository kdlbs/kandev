---
id: "01-explain-secret-bindings"
title: "Explain secret bindings in Settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.13
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
---

# Task 01: Explain secret bindings in Settings

## Summary

Update the two existing Secrets descriptions to explain the binding step. Verify that Global and Workspace guidance remains visible before and after creation on desktop and phone.

## In scope

- Add the desktop and phone browser regressions first; run them against the current copy and record the expected failure.
- Update the two existing settings description keys in English, Portuguese, Simplified Chinese, Japanese, and pseudo-locale catalogs; generate the Traditional Chinese pair.
- Keep the existing page, group, add, and list composition.

## Out of scope

- Runtime or API changes, automatic injection, binding editors, and the optional per-secret “used by” list.

## Acceptance

- Global guidance explicitly says saving alone does not add the secret to a session and names agent profiles, executor profiles, and repositories as binding locations.
- Workspace guidance says saving alone does not add the secret to a session, directs users to a repository in that workspace, and excludes shared profiles.
- Both descriptions remain visible with zero or one secret on desktop and phone. Secret values stay hidden, and phone content has no horizontal overflow.

## ASCII UI preview

`UI-01` and `UI-02` are excerpts of the [plan preview](plan.md#ascii-ui-preview). They cover AC-WORKSPACES-REPOSITORY-SECRETS-001.13.

```text
UI-01 Global:    Saving stores the secret; bind it in an agent profile,
                 executor profile, or repository environment.
                 [Add secret] [empty state or secret list]

UI-02 Workspace: Saving stores the secret; bind it to a repository here.
                 Shared profiles cannot use Workspace secrets.
                 [Add secret] [empty state or secret list]
```

On phones the existing Settings content owns scrolling. The descriptions wrap above the add action and list; no new drawer or fixed control is added. Exact words and line breaks are illustrative.

## Verification

```bash
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/secret-binding-guidance.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-secret-binding-guidance.spec.ts)
```

Before the first package command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` once.

## Files likely touched

- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/settings.json`
- `apps/web/e2e/tests/settings/secret-binding-guidance.spec.ts`
- `apps/web/e2e/tests/settings/mobile-secret-binding-guidance.spec.ts`

## Dependencies

None.

## Risks

The Global description currently names only executor profiles. Keep its translation aligned with all three supported binding sources. Check the rendered phone layout because the longer guidance wraps.

## Parallelism

`sequential`

## Inputs

- [Repository secrets requirements](../../specs/workspaces/requirements/repository-secrets.md), AC-WORKSPACES-REPOSITORY-SECRETS-001.13.
- [Secret reference design](../../specs/workspaces/system-design/repository-secrets.md), Settings feedback and mobile parity.
- Existing `SecretsSettings` descriptions and repository secret browser tests.

## Results

The desktop and mobile regressions failed on the missing session-binding sentence before the catalog updates, then passed after implementation. A follow-up assertion reproduced the removal of the Global encryption-at-rest assurance; both browser specs pass with that statement restored. Secret values remain hidden and phone content has no horizontal overflow.

- `pnpm run i18n:zh-hant` passed and generated the Traditional Chinese pair.
- `pnpm run i18n:pseudo` passed.
- `pnpm run i18n:check` passed.
- `pnpm run typecheck` passed.
- `pnpm run build` passed.
- `make build` and `make e2e-plugin-package` passed to refresh artifacts after locale generation.
- `pnpm e2e:run --no-build --project chromium tests/settings/secret-binding-guidance.spec.ts` passed (1 test).
- `pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-secret-binding-guidance.spec.ts` passed (1 test).
