---
id: "05-declared-author-presentation"
title: "Label declarations and prove rendered attribution"
status: done
wave: 2
depends_on:
  - 04-curated-owner-attribution
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-001.4
  - AC-PLUGINS-MARKETPLACE-001.5
  - AC-PLUGINS-MARKETPLACE-001.10
system_design:
  - ../../specs/plugins/system-design/marketplace.md
  - ../../specs/plugins/system-design/native-plugin-attribution.md
---

# Task 05: Label declarations and prove rendered attribution

## Summary

Identify installed manifest authors as declarations on desktop and phone.
Prove that the browser shows curated catalog attribution and the original
installed declaration as separate values. Use real index-builder output.

## In scope

- The existing `plugins:manifestAuthor` translation in all eight catalogs.
- Manifest-card and marketplace-row component regressions.
- Desktop and phone flows through Browse, installation, and plugin detail.
- Shared test scenario setup, screenshots, and disposable fixture cleanup.
- Public marketplace documentation and package status/results.

## Out of scope

- New publisher badges, trust state, APIs, receipts, or installed metadata joins.
- Canvas attribution, custom-source policy, and installation behavior changes.
- Redesign of plugin settings, navigation, control sizes, or scroll ownership.

## Acceptance

1. Installed details show a localized `Declared author` label with the original
   manifest value. Raw JSON preserves that value. Missing author data keeps
   the existing marker. No declaration implies a verified publisher.
2. Desktop and phone consume a real rebuilt catalog for `acme` with manifest
   author `kandev`. Browse shows `by acme` and the canonical repository.
   Installation and reload preserve `Declared author: kandev` in details.
3. All translations pass the locale gates. Phone labels remain readable without
   document overflow. Focused screenshots and public documentation describe
   the result without claiming publisher verification.

## Implementation sequence

1. Read the work order, previews, source, and the nearest mobile exemplar.
2. Complete Task 04 before this order.
3. Mark this order `in_progress` and update the plan.
4. Add a manifest-card test that expects `Declared author` for a plugin whose
   manifest declares `kandev`. Run it before changing translation values.
5. Update `plugins:manifestAuthor` without renaming the existing key.
6. Add the raw-manifest and unavailable-author component cases.
7. Add curated author/repository assertions to the marketplace-row component test.
8. Add desktop and phone specs with shared scenario setup.
9. Update public marketplace documentation and run all final checks.
10. Record screenshots, actual results, and any preview differences.

The card already calls `t("plugins:manifestAuthor")`. A copy-only correction
does not require new component logic. Adjust only the affected row if localized
label wrapping fails rendered verification. Preserve the raw manifest and
existing values.

Use `PluginMarketplaceReleaseFixture.start()` and `rebuildIndex()` before
registering the fixture source. Its repository is `acme/kandev-plugin-e2e`.
The packaged declaration remains `kandev`. Reuse the existing real package and
verifier, without mocking a corrected marketplace response.

Install through the visible Browse action. Open details through the existing
Settings control or plugin row. Arm causal HTTP/event waits before mutations.
Reload and assert both API-backed values and rendered labels again.
Use `.tap()` for phone actions. Close the fixture server and remove the test
source and installed plugin in `finally`, including assertion-failure paths.

Edit English, pseudo, Portuguese, Simplified Chinese, Japanese, and Korean.
Generate Traditional Chinese catalogs with `pnpm run i18n:zh-hant` from
`apps/web`. Keep the label key and manifest values unchanged.

Public documentation remains a marketplace how-to guide. Add a short explanation
near Browse metadata. The official native catalog shows the curated owner.
Installed details show a declared author. Explain first-party `kdlbs`,
custom-source trust, and the absence of a publisher-verification claim.
Root README and screenshot catalog need no text change unless captures reveal
an inaccurate attribution claim.

## ASCII UI preview

This is the affected excerpt from [UI-01 and UI-02](plan.md#ascii-ui-preview).

```text
UI-01, Browse after rebuild (desktop and phone):
  Fixture plugin v1.0.0
  by acme       Repo: acme/kandev-plugin-e2e

UI-02, installed Manifest:
  Desktop: Declared author    kandev
  Phone:   Declared author          kandev
  Missing: Declared author                -
```

The desktop grid and phone single-column detail remain the shipped composition.
The existing settings container owns vertical scrolling. Grouping and labels
are required. Spacing is illustrative. These views map to the criteria in
frontmatter. Inspect both views with localized labels and raw JSON expanded.

The nearest mobile exemplar is
`e2e/tests/plugins/mobile-plugin-settings-row.spec.ts`. This content-only change
uses its direct detail navigation and scroll model. It needs no new drawer,
toolbar, fixed action, or mobile-only business logic.

## Verification

From the repository root, bootstrap once before the first pnpm command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Before the copy change, this RED test must fail for the missing declaration label:

```bash
(cd apps/web && pnpm exec vitest run components/settings/plugins/plugin-manifest-card.test.tsx)
```

After the correction, run these commands sequentially from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/settings/plugins/plugin-manifest-card.test.tsx components/settings/plugins/marketplace-entry-row.test.tsx)
(cd apps/web && pnpm exec eslint components/settings/plugins/plugin-manifest-card.tsx components/settings/plugins/plugin-manifest-card.test.tsx components/settings/plugins/marketplace-entry-row.test.tsx e2e/tests/plugins/plugin-attribution.spec.ts e2e/tests/plugins/mobile-plugin-attribution.spec.ts e2e/tests/plugins/plugin-attribution-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/plugin-attribution.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-plugin-attribution.spec.ts)
node --test plugin-registry/build-index.test.mjs plugin-registry/check-releases.test.mjs plugin-registry/workflow-contract.test.mjs
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/plugin-marketplace-attribution
```

Use the managed E2E runner's fresh build for both viewports. Confirm discovery
and record the actual counts. Do not overlap runners or override their worker
budget. Use a screenshot of each changed region as the focused visual check.
Run the [coverage preflight](plan.md#verification-commands) before completion.

## Files likely touched

- `apps/web/src/locales/en/plugins.json`
- `apps/web/src/locales/pseudo/plugins.json`
- `apps/web/src/locales/pt-pt/plugins.json`
- `apps/web/src/locales/zh-cn/plugins.json`
- `apps/web/src/locales/zh-hk/plugins.json`
- `apps/web/src/locales/zh-tw/plugins.json`
- `apps/web/src/locales/ja/plugins.json`
- `apps/web/src/locales/ko/plugins.json`
- New `apps/web/components/settings/plugins/plugin-manifest-card.test.tsx`
- `apps/web/components/settings/plugins/marketplace-entry-row.test.tsx`
- `apps/web/components/settings/plugins/plugin-manifest-card.tsx`, only if affected label wrapping needs adjustment.
- New `apps/web/e2e/tests/plugins/plugin-attribution.spec.ts`
- New `apps/web/e2e/tests/plugins/mobile-plugin-attribution.spec.ts`
- New `apps/web/e2e/tests/plugins/plugin-attribution-helpers.ts`
- `docs/public/plugins-marketplace.md`
- This work order and `plan.md`, for results and status.

## Dependencies

Task 04 supplies corrected builder output. Reuse
`apps/web/e2e/helpers/plugin-marketplace-release.ts` and
`apps/web/e2e/helpers/plugin-fixture.ts` without expanding their public behavior.
The managed runner builds their existing fixture packages and verifier.

## Risks

- A component test alone cannot prove a corrected builder reaches the browser.
  The E2E fixture must rebuild its index and expose that real output.
- The initial fixture index currently contains legacy attribution. Rebuild it
  before the source becomes visible to the browser.
- A long translated label can exceed the existing fixed label width.
  Inspect the affected phone row with the final locale values.
- PR 3793 changes the same label and adds publisher components. Reconcile the
  current branch before implementation rather than importing its stale contract.

## Parallelism

`sequential`

## Inputs

- [Marketplace requirements](../../specs/plugins/requirements/marketplace.md).
- [Native attribution design](../../specs/plugins/system-design/native-plugin-attribution.md).
- [Marketplace design](../../specs/plugins/system-design/marketplace.md).
- [Full previews and evidence](plan.md).
- `apps/web/AGENTS.md`, `/mobile-parity`, and `/e2e`.
- Existing manifest card, marketplace row tests, and local release fixture.
- `apps/web/e2e/tests/plugins/plugins.spec.ts`, curated-release scenario.
- `apps/web/e2e/tests/plugins/mobile-plugin-settings-row.spec.ts`.
- `docs/i18n.md` and `docs/public/README.md` before catalog/public-doc edits.

## Results

Status: done.

The `plugins:manifestAuthor` value is now `Declared author` in English, pseudo,
Portuguese, Simplified Chinese, Japanese, and Korean. The Traditional Chinese
pair was generated with `pnpm run i18n:zh-hant` (`聲明作者` for Hong Kong,
`宣告作者` for Taiwan). The key and the underlying manifest values are
unchanged.

The new `plugin-manifest-card.test.tsx` covers the declaration label, the
unchanged value, raw-manifest preservation, and the missing-author marker.
`marketplace-entry-row.test.tsx` now asserts the curated author and canonical
repository presentation. The component folder passed 18 files / 111 tests.
`pnpm run i18n:check`, `pnpm run i18n:ratchet`, ESLint, and the web typecheck
passed.

The desktop and phone E2E flows start the local release fixture, rebuild its
index with the real builder before registering the source, install from Browse,
open installed details through visible controls, and assert `by acme`, the
canonical repository, `Declared author: kandev`, raw-manifest preservation, and
the same result after reload.

```text
pnpm e2e:run --host --project chromium tests/plugins/plugin-attribution.spec.ts
1 passed (6.6s)

pnpm e2e:run --host --project mobile-chrome tests/plugins/mobile-plugin-attribution.spec.ts
1 passed (8.6s)
```

Screenshots were captured through `CAPTURE_PR_ASSETS=true` for both affected
regions: Browse shows `by acme`; desktop and phone manifests show the
`Declared author` row with `kandev`. The phone capture confirms the label fits
on one line inside the existing settings scroll container without horizontal
document overflow. The shared fixture helper also tolerates the prebuilt
archive's root-level `manifest.yaml` as well as repackaged `./manifest.yaml`
entries, so a rebuild at the initial fixture version can read the manifest.
A review correction restored the explicit type-only `Locator` import in the
shared E2E helper so the annotated `browseEntry` return type resolves in
isolation.

`docs/public/plugins-marketplace.md` explains curated-owner attribution,
first-party `kdlbs`, declared authors, custom-source trust, and the absence of a
publisher-verification claim. Public-docs validation, catalog validation,
specification lint, and the coverage preflight passed.

The public `docs/screenshots/plugin-browse.png` still reflects the currently
published index, which shows the previous `by kandev` attribution until the
registry workflow rebuilds and publishes. Regenerate that catalog capture after
publication; the focused E2E captures prove the corrected projection in the
meantime.
