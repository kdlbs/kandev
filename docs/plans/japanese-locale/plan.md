---
created: 2026-09-20
status: in_progress
requirements:
  - REQ-PLATFORM-JAPANESE-LOCALE-001
system_design:
  - ../../specs/platform/system-design/i18n.md
legacy_specs: []
---

# Implementation Plan: Japanese Locale

## Overview

Ship the `ja` (日本語) locale for the Kandev web UI and the Go-rendered browser
surfaces, following the same pattern already proven by `pt-pt`, `zh-cn`,
`zh-tw`, and `zh-hk`: commit human-maintained JSON catalogs mirroring `en` in
every namespace, register `ja` in the frontend runtime and the Go i18n
`supportedLocales`, let the existing drift/parity/lint gates discover `ja`
automatically from the directory listing, and prove the switch, cookie
persistence, `<html lang>`, Japanese formatting, and English restoration with
unit tests and a Playwright scenario.

Requirements: [`REQ-PLATFORM-JAPANESE-LOCALE-001`](../../specs/platform/requirements/japanese-locale.md)
(delivered as a sibling of the existing i18n requirement; `AC-PLATFORM-I18N-001.3`
in [`i18n.md`](../../specs/platform/requirements/i18n.md) gains `ja`).
System design: [`docs/specs/platform/system-design/i18n.md`](../../specs/platform/system-design/i18n.md).

The combined change touches only localization wiring and catalogs; no product
behavior, routing, or data flow changes. The user did not provide translations,
so every Japanese value is produced by the implementer and recorded in the work
orders; a later human/LLM review pass may tighten wording but is not a merge
blocker (real-locale parity gates structurally; the identical-to-English check
rejects keys left verbatim in English unless declared in `_verbatim.json`).

## Delivery waves

Wave 1 — Catalogs and frontend runtime (Task 01). This is the largest chunk
(~10,837 keys across 37 namespaces) and is split into three commits so each
stage is independently reviewable and CI-green:

1. mini showpiece: `common.json` + `settings.json` (~1,382 keys), frontend
   registration, unit tests — the first RED/GREEN loop proving the wiring.
2. high-traffic namespaces around common+settings.
3. remaining namespaces to full parity.

Wave 2 — Backend locale + negotiation tests + `docs/i18n.md` (Task 02).

Wave 3 — E2E scenario + screenshots (Task 03).

Wave 4 — Repository verification (`list-docs validate`, spec lint, optional
branch-level review) (Task 04).

## Dependencies

| Wave | Depends on |
| --- | --- |
| 2 | 1 (frontend registration is the reference runtime; backend mirrors it) |
| 3 | 1, 2 (E2E runs against integrated runtime, backend, and catalogs) |
| 4 | 1, 2, 3 (final all-gates pass) |

## Verification strategy

```bash
(cd apps && pnpm install --frozen-lockfile)     # once after worktree creation
(cd apps/web && pnpm exec vitest run lib/i18n)  # focused frontend suites (RED first)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)            # drift + parity + identical-to-en gate
(cd apps/web && pnpm run lint)
(cd apps/backend && make test)                  # backend i18n suite
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/i18n/language-switch.spec.ts)
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

Run one focused behavioral test in RED before the production/catalog edit, then
re-run the exact focused suite after the final relevant edit.

## Work orders

Wave 1:

- [x] [Task 01: Frontend ja catalogs and locale runtime integration](task-01-frontend-ja-locale-catalogs.md)

Wave 2:

- [x] [Task 02: Backend ja locale negotiation](task-02-backend-ja-locale-negotiation.md)

Wave 3:

- [x] [Task 03: Japanese locale E2E](task-03-japanese-locale-e2e.md)

Wave 4:

- [x] [Task 04: Repository verification](task-04-repository-verification.md)

## Risks

- **Translation volume (~10,837 keys).** Key parity alone cannot prove natural
  wording, preserved semantic tokens, or layout fit. Mitigation: the shared
  `_verbatim.json` registry covers technical terms that stay English; every
  value preserves `{{placeholders}}`, `_one`/`_other`, `<Trans>` tags, code
  tokens, shortcuts, and brand names (the parity gate enforces this
  structurally); high-traffic namespaces get priority.
- **Japanese avoids English loanwords where awkward** (`Create` → `作成`,
  `Delete` → `削除`) but dev tooling often keeps English technical terms
  (`repository`, `pull request`, `workflow`, `agent`, `terminal`) or uses
  katakana. The parity `identical-to-English` check would flag a key left
  verbatim; the implementer records such keys in `src/locales/ja/_verbatim.json`
  with a reason instead of English-casing a value that should be katakana.
- **Date/relative-time formatting** must pass `ja` into `Intl`/`date-fns`, not
  leave English data; existing `date-locale.ts` mappings must be extended.
- **E2E and manual `make dev` validation are environment-sensitive** and may
  expose baseline failures; focused locale evidence must remain separately
  recorded.

## Open Questions

None. The user supplied the goal ("GUIの日本語化"), which this plan follows as
the shipped-language precedent established by `pt-pt` / `zh-cn` /
`zh-tw` / `zh-hk`: locale id `ja`, endonym `日本語`, full catalogs, backend
negotiation, CI parity, and E2E proof, committed in one feature branch.
