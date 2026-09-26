---
id: "04-attribution-ui"
title: "Show publisher attribution and document it"
status: done
wave: 4
depends_on:
  - "03-canvas-receipts"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PUBLISHER-002
  - REQ-PLUGINS-PUBLISHER-003
  - REQ-PLUGINS-PUBLISHER-004
acceptance_criteria:
  - AC-PLUGINS-PUBLISHER-002.3
  - AC-PLUGINS-PUBLISHER-002.4
  - AC-PLUGINS-PUBLISHER-002.5
  - AC-PLUGINS-PUBLISHER-002.6
  - AC-PLUGINS-PUBLISHER-003.1
  - AC-PLUGINS-PUBLISHER-003.2
  - AC-PLUGINS-PUBLISHER-003.3
  - AC-PLUGINS-PUBLISHER-003.4
  - AC-PLUGINS-PUBLISHER-003.5
  - AC-PLUGINS-PUBLISHER-004.3
  - AC-PLUGINS-PUBLISHER-004.4
  - AC-PLUGINS-PUBLISHER-004.5
system_design:
  - ../../specs/plugins/system-design/publisher-identity.md
---

# Task 04: Show publisher attribution and document it

## Summary

Show publisher, verification, source, and declared author distinctly on desktop and phone.
Send catalog selectors through installation and update callers, then document the trust limits.

## In scope

- Add a shared typed attribution component and backend projection types.
- Update native marketplace, installed rows, manifest details, canvas cards/details, and install review.
- Show candidate-publisher changes before manual updates and preserve installed attribution on errors.
- Add the native details Verify publisher action, progress, unavailable-version/mismatch explanations, retry, and persisted success state.
- Add all five locale catalogs and update existing affected tests.
- Update public marketplace, authoring, and canvas documentation plus registry guidance.

## Out of scope

New verification algorithms, overlays, trust settings, or production publication.

## Acceptance

- UI-01 through UI-04 show backend attribution consistently, including explicit existing-version verification, source errors, and update failures.
- Phone and desktop flows use the same selectors and retain readable trust labels, reachable actions, and existing permissions.
- Unit tests, fresh-build desktop/phone E2E, translations, and public-doc checks pass.

## ASCII UI preview

See the [combined previews](plan.md#ascii-ui-preview). Applicable criteria: AC-PLUGINS-PUBLISHER-003.1 through 003.5.

```text
UI-01 desktop
Example v1.2.3 [Install]
Publisher: acme [Verified publisher]
Source: Kandev Official
Declared author: Example contributors

UI-01 phone
Example v1.2.3
Publisher: acme
[Verified publisher]
Source: Kandev Official
Declared author:
Example contributors
[Install]

UI-02 shared
Unverified publisher
Source: Uploaded file
Declared author: kandev

UI-03 shared
Installed: acme [Verified publisher]
Candidate: new-owner [Verified publisher]
This update changes the publisher.
[Update]

UI-04 desktop details
Unverified publisher  [Verify publisher]
Source: Uploaded file

UI-04 phone details
Unverified publisher
Source: Uploaded file
[Verify publisher]

Success (both)
Publisher: acme [Verified publisher]
Source: Uploaded file
Matched against: Kandev Official
```

Values wrap on phones. Keep labels visible and use a separate action row.
Reuse the current settings route and canvas review scroll owner.
No hover is required. Source and repository link targets remain reachable by touch.

## Verification

Run from the repository root. Install dependencies once if this worktree lacks them.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/plugins lib/api/domains/plugins-api.test.ts lib/api/domains/marketplace-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/plugin-publisher-identity.spec.ts tests/canvas/canvas-marketplace.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-plugin-publisher-identity.spec.ts tests/settings/mobile-plugin-updates.spec.ts tests/canvas/mobile-canvas-marketplace.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The two settings E2E files named in the plan are implemented. Chromium and the
configured mobile-chrome project run with the managed host build. The tests
exercise UI-01 through UI-04 through rendered controls and text, including
click/tap verification, progress, missing evidence, retry, unchanged version,
and attribution after reload. The browser error path uses the actual backend
verification endpoint; backend tests cover the injected trusted transport
success path.
Do not treat mocked UI evidence as backend verification evidence.

## Files likely touched

- `apps/web/lib/types/plugins.ts` and canvas distribution DTOs.
- `apps/web/lib/api/domains/plugins-api.ts` and its tests.
- `apps/web/components/settings/plugins/`: attribution component, rows, cards, details, install/update hooks, and tests.
- `apps/web/src/locales/` including generated Traditional Chinese values.
- `apps/web/e2e/tests/settings/` and `apps/web/e2e/tests/canvas/`.
- `docs/public/plugins-marketplace.md`, `plugins-authoring.md`, `canvases.md`.
- `plugin-registry/README.md`.
- Search root README and docs/screenshots.md for affected terminology before deciding whether they need edits.

## Dependencies

Tasks 02 and 03 backend projections and installation paths.

## Risks

Do not derive badges from manifest strings, Signed, or source labels.
Do not replace an installed badge with the latest catalog's publisher.
Public docs must describe implemented behavior and must not promise code safety.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/plugins/requirements/publisher-identity.md).
- [System design](../../specs/plugins/system-design/publisher-identity.md).
- [Trust decision](../../decisions/2026-09-18-plugin-publisher-trust.md).

## Results

- RED: the first UI pass displayed declared author and source together as
  publisher attribution and sent URL-only installs; component and API tests
  failed before the shared identity projection and catalog selector were wired.
- `cd apps/web && pnpm exec vitest run components/settings/plugins lib/api/domains/plugins-api.test.ts lib/api/domains/marketplace-api.test.ts`: passed (21 files, 150 tests).
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm run lint`: passed.
- `cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet`: passed.
- Chromium settings and canvas runs with the managed host build: passed.
- Mobile-chrome settings, update, and canvas runs: passed (3 tests).
- `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs`: passed (62 tests and 46 pages).
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.
- Review fixes: marketplace and canvas surfaces now label declared author
  credit explicitly, publisher trust is visible to members, and verification
  responses update only a matching live installation snapshot. The registry
  pull-request gate now fails mixed native results, and authenticated desktop
  and mobile member detail flows cover the read-only trust presentation.
- `pnpm e2e:run --host --no-build --project=auth e2e/tests/auth/plugin-settings-member.spec.ts`:
  passed.
- `pnpm e2e:run --host --no-build --project=mobile-chrome e2e/tests/auth/mobile-plugin-settings-member.spec.ts`:
  passed.
