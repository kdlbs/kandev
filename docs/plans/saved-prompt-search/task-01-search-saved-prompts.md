---
id: "01-search-saved-prompts"
title: "Search saved prompts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SAVED-PROMPT-SEARCH-001
acceptance_criteria:
  - AC-UI-SAVED-PROMPT-SEARCH-001.1
  - AC-UI-SAVED-PROMPT-SEARCH-001.2
  - AC-UI-SAVED-PROMPT-SEARCH-001.3
  - AC-UI-SAVED-PROMPT-SEARCH-001.4
  - AC-UI-SAVED-PROMPT-SEARCH-001.5
  - AC-UI-SAVED-PROMPT-SEARCH-001.6
  - AC-UI-SAVED-PROMPT-SEARCH-001.7
system_design:
  - ../../specs/ui/system-design/saved-prompt-search.md
---

# Task 01: Search saved prompts

## Summary

Implement the local saved-list projection and preserve existing settings flows.
Deliver mounted regression and focused real-browser evidence for desktop and
phone in the same sequential slice.

## In scope

- First add a meaningful mounted consumer test that fails because search is
  absent; run it and retain the actual RED result before production changes.
- Implement the design using existing settings `Input`, loaded saved entries,
  translations and list rendering. Keep search disabled throughout edit, create,
  delete confirmation and pending mutation, with matched editing row mounted.
- Retain the edited row and open delete target in list order through saved-field refresh while their IDs remain loaded; resume normal filtering after each action and never restore a removed row.
- Extend the existing consumer test without replacing its fixtures, assertions
  or meaning. Add the two dedicated E2E files described in the plan. Use only
  disposable prompts created by each test and clean up their IDs, never all
  non-builtins or live operator data.
- Add one concise instruction under Saved prompts in
  `docs/public/developer-tools.md`; preserve every other guide byte, including
  concurrent Files and Quick Chat work. Public page is a task-oriented how-to.

## Out of scope

Existing data-owner/action/test-setup migrations, hooks, permissions, CRUD,
reference delivery, load/error and async lifecycle semantics; remote search,
regex, extra controls, new translation catalogs, row redesign or generic helpers.

## Acceptance

- Mounted tests prove all seven criteria, including saved-object immutability,
  dirty-route and backend-write absence, disabled operations and draft continuity.
- Desktop and phone E2E prove rendered search and reset with disposable data;
  phone geometry meets the design and rendered composition matches UI-01/02.
- The single guide instruction describes the delivered search; all original
  consumer tests and adjacent guide content remain intact.

## ASCII UI preview

[Full preview](plan.md#ascii-ui-preview), UI-01/02/03; AC .1-.7. The existing
settings region owns scrolling; this inline control adds no overlay or fixed
area. Desktop and phone share ordering; phone input has a 44px minimum.

```text
Custom prompts                         [+ Add]
[ Search...                            ]
[ matching prompt     existing actions ]
```

```text
[ missing                              ]
  No results.
```

During edit/create/delete/busy the query is disabled; the matched editing row
and its draft stay mounted.

## Verification

Run only after the later explicit execution and exclusive global heavy release.
From repository root, use the pinned paths and sequential commands below. Every
`quiet` call creates a distinct external log directory, including reruns. Join
all actual native command handles to exit; do not infer results from duplicate
runs. Run the first focused test immediately after writing the RED test, before
production code; rerun the focused test after implementation. Stage new frontend
paths before the staged i18n ratchet; do not commit until all checks pass.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2
quiet() { KANDEV_RUN_QUIET_DIR="$(mktemp -d /home/jcfs/.kandev/saved-prompt-search-quiet.XXXXXX)" scripts/run-quiet "$@"; }
quiet install -- pnpm --dir apps install --frozen-lockfile
quiet test -- pnpm --dir apps/web exec vitest run components/settings/prompts-settings.test.tsx
# After the RED and implementation, repeat the focused command above.
quiet typecheck -- env NODE_OPTIONS=--max-old-space-size=8192 pnpm --dir apps/web run typecheck
quiet lint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/settings/prompts-settings.tsx components/settings/prompts-settings.test.tsx e2e/tests/settings/saved-prompt-search.spec.ts e2e/tests/settings/mobile-saved-prompt-search.spec.ts
quiet i18n -- pnpm --dir apps/web run i18n:check
quiet i18n -- pnpm --dir apps/web run i18n:ratchet
quiet e2e-sleep -- pnpm --dir apps/web run e2e:sleep-ratchet
quiet docs -- node --test scripts/validate-public-docs.test.mjs
quiet docs -- node scripts/validate-public-docs.mjs
quiet specs -- python3 scripts/list-docs.py validate
quiet specs -- python3 scripts/lint-spec-files.test.py
quiet specs -- python3 scripts/lint-spec-files.py --all
quiet diff -- git diff --check
quiet backend-build -- make -C apps/backend build build-e2e-delivery-fixture
quiet web-build -- pnpm --dir apps/web run build:e2e
quiet fixture-build -- make -C apps/backend e2e-plugin-ui e2e-plugin-package
quiet e2e --summary -- env CAPTURE_PR_ASSETS=true pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium -- tests/settings/saved-prompt-search.spec.ts --workers=1 --retries=0
quiet e2e --summary -- env CAPTURE_PR_ASSETS=true pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome -- tests/settings/mobile-saved-prompt-search.spec.ts --workers=1 --retries=0
```

If the permitted optional extracted component/test is created, include both in
the focused Vitest and ESLint commands. Record exact command exits and all
handles. Preserve owned synthetic screenshot originals, manifest and hashes
outside managed output before any subsequent managed run. Do not run broad
suites or share dependencies/builds. Use normal active hooks and full logs,
without bypass or amend, only after checks. Refresh actual main/source gap and
fully paginated open-PR inventory of every actual owned path before publication;
reassess changed overlaps and preserve concurrent edits. Follow the user's
separate publication, screenshot, heavy-return and single-observer instructions
only after release; do not merge or delete this task.

## Files likely touched

- `apps/web/components/settings/prompts-settings.tsx`
- `apps/web/components/settings/prompts-settings.test.tsx`
- Optional `apps/web/components/settings/prompt-list-search.tsx` and its test.
- `apps/web/e2e/tests/settings/saved-prompt-search.spec.ts`
- `apps/web/e2e/tests/settings/mobile-saved-prompt-search.spec.ts`
- `docs/public/developer-tools.md` (one Saved prompts instruction only).
- This design package for lifecycle and result updates after successful work.

## Dependencies

None within this package. Later explicit execution and exclusive heavy release
are required; ROOT first reads all four artifacts.

## Risks

Filtering drafts or confusing no matches with an empty collection breaks ordinary
actions. Concurrent source and guide edits must be preserved. Screenshots can be
cleared by the managed runner unless preserved externally first.

## Parallelism

Sequential; no delegates or extra sessions.

## Inputs

- [Requirements](../../specs/ui/requirements/saved-prompt-search.md) and
  [system design](../../specs/ui/system-design/saved-prompt-search.md).
- Existing `prompts-settings.tsx`, consumer tests, mobile prompt settings E2E,
  settings manual-save contract and saved-prompt delivery contract.

## Results

Mounted RED: exit 1, missing accessible searchbox (native 38315). Final consumer suite: 19 passed; focused lint passed (native 90987). Typecheck, i18n, staged ratchet, E2E sleep ratchet, public-doc validators, spec validators and diff checks passed (native 36217). Backend, E2E web and fixture builds passed (native 24046). After restoring the original scroll effect, focused lint, 19 consumer tests, typecheck and the web rebuild passed (native 1763). Final desktop chromium and mobile-chrome each passed one focused test (native 69486 and 81651); phone, narrow fine-pointer and coarse-pointer geometry passed. Original screenshots and manifests are preserved externally with hashes.

Corrective validation: causal same-ID editor-refresh RED (native 12190, exit 1) and delete-confirmation-refresh RED (33176, exit 1) each failed because the row disappeared. Final 23 mounted tests, focused lint, typecheck, spec catalog/lint, diff and matching web build passed (86326, exit 0). Both focused desktop and phone browser cases passed with one worker and zero retries (36472, exit 0). Existing action and saved-data owners remain unchanged; the predicate retains each active ID only while present in the current list.
