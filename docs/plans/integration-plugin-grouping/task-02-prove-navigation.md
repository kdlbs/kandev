---
id: "02-prove-navigation"
title: "Prove desktop and phone behavior"
status: done
wave: 2
depends_on: ["01-restore-grouping"]
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-002
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.10
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.11
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.12
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.7
---

# Task 02: Prove desktop and phone behavior

## Summary

Verify grouped integration navigation with the installed fixture and real saved
settings. Explain the corrected customization behavior in the public guide.

## In scope

Desktop/phone browser regressions, rendered captures, focused regressions for
existing customization, public guide clarification, and final package results.
Use the existing `e2e-integration` fixture. No YouTrack credentials are required.

## Out of scope

Broad E2E suites, live-user mutation, new plugin packages, commits, and publishing.

## Acceptance

1. Desktop and phone tests prove ancestry, uniqueness, route activation, collapse,
   individual visibility after reload, parent hide/show, reset, and custom-shortcut
   independence. Desktop rail access remains functional.
2. Phone controls measure at least 44px and fit the drawer. Selecting the link
   closes navigation and reaches the fixture route. Captures match UI-01/UI-02.
3. Public guidance describes grouping and sibling moves. Record exact command
   results and update both work orders and plan without marking unrun checks passed.

## UI preview

These labelled tables replace ASCII sketches to respect the session's explicit
preview-format instruction. Hierarchy and control order are requirements;
provider names illustrate available entries, not a hardcoded provider list.

### UI-01: Desktop sidebar, expanded Integrations

| State | Top-level entries | Integrations children, in order |
| --- | --- | --- |
| Before (source-confirmed) | Integrations, YouTrack | GitHub, GitLab, Integration settings |
| After | Integrations | GitHub, GitLab, YouTrack, Integration settings |
| Folded | Integrations | Hidden until expanded |
| YouTrack hidden | Integrations | GitHub, GitLab, Integration settings |

The existing rail launcher exposes the same child list when the sidebar is
collapsed. The existing navigation region owns scrolling. Header fast actions
keep their current first-party-only policy. No new icon, motion, or sizing rule.

### UI-02: Phone app menu, Integrations disclosure

| State | Menu row | Disclosure contents |
| --- | --- | --- |
| Initial | Integrations, collapsed | None visible |
| Expanded | Integrations, expanded | Provider rows, YouTrack, Integration settings |
| Activation | Tap YouTrack | Open its registered route and close navigation |

Reuse AppNavSheet and MobileIntegrationsSection. Rows retain 44px touch targets,
one drawer scroller, safe-area clearance, and existing focus-return behavior.
No top-level duplicate appears. Settings remains reachable without any providers.

### UI-03: Customization and Settings preview

| Level | Entry | Allowed actions |
| --- | --- | --- |
| Top level | Integrations | Visibility, move among top-level entries |
| Child | YouTrack | Visibility, move among integration-plugin siblings |
| Child | Another integration plugin | Visibility, move among integration-plugin siblings |
| Separate custom group | Explicit YouTrack shortcut | Existing shortcut actions |

Settings preview uses UI-01 grouping. Phone customization keeps its existing
inset drawer and explicit move controls. A hidden parent does not erase the
child's checked state. Controls do not offer cross-group detachment.

UI-01 through UI-03 cover AC-UI-SIDEBAR-CUSTOMIZATION-001.10 through 001.12,
004.2, 005.2 through 005.4, and 006.7.

Full context: [plan previews](plan.md#ui-preview).

## Test setup

Use `e2e/fixtures/test-base`, `helpers/api-client.ts`, and the installation flow
in `tests/plugins/mobile-plugin-nav.spec.ts`. The fixture bundle already registers
`e2e-integration` in Integrations. Wait for registration readiness before asserting
placement. Use real settings reads/writes, not a mocked final navigation array.
Clean up only the fixture installed by the test. Scope locators to the active
sidebar or phone drawer, and separate original links from intentional custom pins.

Task 01 supplies the behavioral RED gate. These tests prove integration against a
fresh managed build. Compare screenshots with the plan and inspect phone sizing.

## Verification

From repository root, after Task 01 checks pass. Run mobile first to build current assets, then desktop sequentially against that build.

```bash
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/plugins/mobile-integration-plugin-grouping.spec.ts tests/settings/mobile-sidebar-customization.spec.ts tests/settings/mobile-sidebar-direct-customization.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/plugins/integration-plugin-grouping.spec.ts tests/settings/sidebar-customization.spec.ts tests/settings/sidebar-direct-customization.spec.ts)
(cd apps/web && pnpm exec eslint e2e/tests/plugins/integration-plugin-grouping.spec.ts e2e/tests/plugins/mobile-integration-plugin-grouping.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The two plugin grouping specs are new files owned by this work order. Confirm test
project discovery. Managed runs rebuild frontend/backend and package fixtures.
Do not overlap these suites or override worker limits. Record the lint result with the browser results.

## Files likely touched

- `apps/web/e2e/tests/plugins/integration-plugin-grouping.spec.ts` (new).
- `apps/web/e2e/tests/plugins/mobile-integration-plugin-grouping.spec.ts` (new).
- Existing four settings suites in Verification only if fixture or old assertions
  need reconciliation. Their original flows remain covered.
- `docs/public/use-kandev.md`: Customize the sidebar (how-to guidance).
- This package's plan/work orders and their results sections.

## Dependencies

Task 01.

## Risks

Plugin readiness and responsive hidden mounts can produce false duplicate counts.
Use active surface scope and real registration readiness. Do not alter the
fixture's main/footer registrations to make this narrow scenario pass.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-customization.md).
- [Design](../../specs/ui/system-design/sidebar-customization.md#integration-plugin-grouping).
- Existing mobile plugin navigation and sidebar customization E2E patterns.
- Public docs README and `/docs-maintainer` guidance.

## Results

Completed on 2026-10-26:

- Mobile managed E2E command above: 11 passed, including the new grouped-plugin
  flow and existing customization suites. Fresh frontend/backend/fixture build.
  Log: `/tmp/kandev-run.e2e.MSPoa0sK.log`.
- Desktop managed E2E command above reused that production build: 11 passed.
  Includes the new visibility, custom-pin independence, reset, route activation,
  and collapsed-rail regression. Log: `/tmp/kandev-run.e2e.lXHOuXfD.log`.
- Changed/new TS and TSX ESLint, typecheck, and i18n checks passed.
- Public-doc validator: 48 pages. Validator tests: 63 passed.
- Specification catalog and lint passed; whitespace checks passed.
- Actual YouTrack v0.2.0 was installed through the UI on a separate fresh
  instance. Desktop and touch-phone grouping, uniqueness, reload, route
  activation, 44px phone sizing, and menu dismissal passed.
- Inspected fresh desktop and phone captures against UI-01/UI-02. Screenshots
  are retained as ignored PR assets and published through native attachments.

Local managed runs removed inherited `KANDEV_INTERNAL_*` launcher configuration
and supplied existing Nix Chromium library paths. These were runner environment
adjustments, not application or test-harness changes. The final desktop test
corrects a selector that previously omitted a shortcut-count suffix.

PR CI and review remain an external delivery gate until their exact-head checks
are terminal and findings are addressed. No delegation was used.


## Review remediation

Review found two layout cases: an inferred Integrations parent was
absent from explicit edit state, and wider listing menus received the phone-only
collapsed disclosure. The parent now materializes for both rendering and editing
without writing on read. The shared resource component passes the phone
presentation choice through instead of applying it to every consumer.
Regression tests reproduce both defects before correction. The desktop browser
flow additionally edits a layout that originally lacks the parent, and a new
coarse-pointer tablet flow opens the listing menu with expanded plugin links.
Fresh published-plugin screenshots passed on desktop, phone, and tablet. The touch/pen pointer handler now preserves propagation to the context-menu trigger. The Office singleton check passes without a code change because builtin eligibility removes its Integrations parent. The shared draft materialization also supplies the legacy parent to Settings, so its controls remain editable. Focused validation passed: 111 tests in 16 suites, typecheck, changed-file ESLint, i18n, docs, and specification checks. Final browser regression passed: 12 desktop tests (including tablet navigation) and 11 phone tests against the same fresh production build.

### UI-04: Tablet listing menu, expanded Integrations

| Level | Content | Behavior |
| --- | --- | --- |
| Heading | Integrations | Expanded, without a phone disclosure toggle |
| Link | YouTrack | Visible on opening the menu, opens its registered route |
| Link | Integration settings | Retains the existing setup destination |

Reuse the wider menu surface and its scroller. Touch controls keep their existing
44px sizing. This view preserves the wider-consumer contract in the frontend
guide. See [combined plan](plan.md#ui-04-tablet-listing-menu-expanded-integrations).

Final review-validation logs, 2026-10-26:

- Desktop/tablet: `/tmp/youtrack-e2e-logs/kandev-run.e2e.r9Mj2pnK.log`, 12 passed, retries disabled.
- Phone: `/tmp/youtrack-e2e-logs/kandev-run.e2e.xj4A8ZlJ.log`, 11 passed, retries disabled.
- Focused reproductions of four initial CI failures: `/tmp/youtrack-e2e-logs/kandev-run.e2e.a5BE0TWh.log`, 4 passed, retries disabled. These unchanged tests cover Jira auth health, cancellation across task switches, jump-to-latest, and the Quick Chat goal disclosure. Fresh PR CI remains required.

The final source also excludes portaled menu focus from sidebar expansion, with a behavioral regression and a positive control for clipped-row focus. The final browser runs above include the legacy-parent hide, save, reload, and restore flow. Fresh published YouTrack captures cover desktop, phone, and tablet.

## Final Claude observations

The review at head `8c21847710c` confirmed the earlier fixes. The possible Settings dirty-draft issue was not reproduced: the actual draft and drag/action hooks keep both directions of a rejected cross-group move clean. A valid integration sibling move marks the draft dirty. Three new hook regressions cover those outcomes. The full focused command now passes 111 tests in 16 suites.

A missing legacy parent intentionally appends to the flat list, preserving the prior fallback position and existing node order. The design and projection assertion now state that position. Settings move controls disable boundary actions and clamp their positions; sibling lists always include the rendered node, so those controls cannot request an out-of-range position.

Removed the redundant inner editor key and used `cn()` for the existing phone-row classes. The rendered class values and keyed row identity are unchanged. Typecheck and focused tests passed. Fresh desktop/tablet and phone browser checks passed before this follow-up is pushed.

The final follow-up incorporates base `e5c38184cf9c191075dc88c1ffad28c636858bf5` by rebasing the existing branch. Base and PR paths do not overlap, and the update applied without conflicts. The combined source passed 111 focused tests and typecheck. The fresh combined-source browser build passed 12 desktop/tablet cases and 11 phone cases with retries disabled. Logs: `/tmp/youtrack-e2e-logs/kandev-run.e2e.2rP49zaU.log` and `/tmp/youtrack-e2e-logs/kandev-run.e2e.zsiyyYKR.log`.

The previous-head CI locale setup failure (`pseudo-coverage.spec.ts`, Office workspace org) passed three consecutive focused repetitions on the combined source, with retries disabled. Log: `/tmp/youtrack-e2e-logs/kandev-run.e2e.gLW8excu.log`. The failed CI artifact's retry trace also showed a successful pseudo-cookie request; the first attempt had no retained trace. No locale source change was made. Fresh exact-head CI is still required; this focused pass does not by itself classify the original failure as resolved.

## Collision-safe legacy parent

Claude's review at `a07ef4e6edb` raised a possible ID collision. Inspection confirmed that valid custom sections may use `integrations`. The materializer now follows the existing Coordinator suffix convention to choose the first unused node ID, preserving custom IDs and destination ownership. A behavioral regression failed with `duplicate_id` before the fix and passed afterward. It covers multiple occupied suffixes, stable repeated materialization, editable parent visibility, and unchanged custom sections.

The desktop legacy-layout flow now seeds a custom section with that colliding ID and an explicit integration shortcut. It hides and restores the distinct Integrations parent through real settings writes and reload, checks unique persisted IDs, and keeps the custom shortcut visible. The fixture explicitly expands navigation so this case tests identity and persistence. The normal desktop, phone, and geometry cases remain covered separately. Final collision validation passed: 112 focused tests in 16 suites, typecheck, zero-warning changed-file ESLint, 12 desktop/tablet browser cases, and 11 phone cases. The browser cases used retries disabled and the same fresh frontend build. Logs: `/tmp/youtrack-e2e-logs/kandev-run.e2e.0VfWh9CF.log` and `/tmp/youtrack-e2e-logs/kandev-run.e2e.LLasneTS.log`.


## CI fixture follow-up

The user authorized CI remediation and merge after the implementation. The full
run on the grouping fix passed all non-browser checks. Four browser shards failed;
each case passed locally without a retry, but the retained CI artifacts expose
specific setup or readiness races:

- `layout/mobile-sidebar-read-recovery.spec.ts`: start a valid idle session before
  simulating failed workspace reads. Settle drawer animation and hold recovery
  responses until the native Retry tap ends, so recovery cannot move another
  control under the same gesture. Retain the row and 44px assertions.
- `pages/mobile-github-page.ts` and `github/mobile-github-sidebar.spec.ts`: target
  the saved-query button, settle drawer animation, and verify selection dismisses
  the drawer before checking the repository filter and reload persistence.
- `chat/question-image-previews.spec.ts`: arm the image-response wait before
  reload and await its body before checking the rendered preview. The CI snapshot
  showed the correct question with its image still loading.
- `task/workspace-file-transfer.spec.ts`: assert the uploaded copy in Changes,
  then return to Files before checking both filenames. The CI snapshot showed
  the upload succeeded and the default layout activated Changes.

These changes preserve the existing behavioral assertions and native input. No
production behavior, retry setting, or timeout budget changed. They extend this
work order only for the user's explicit PR cleanup request.

Verification commands, from repository root with the isolated runtime environment
described above:

```bash
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/layout/mobile-sidebar-read-recovery.spec.ts tests/github/mobile-github-sidebar.spec.ts --retries 0 --trace on
pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/chat/question-image-previews.spec.ts tests/task/workspace-file-transfer.spec.ts --retries 0 --trace on
```

The two mobile failure cases passed three consecutive repetitions (six tests).
The final full mobile files passed all nine tests with retries disabled. Logs:
`/tmp/youtrack-e2e-logs/kandev-run.e2e.mSkDwWyz.log` (repetitions) and
`/tmp/youtrack-e2e-logs/kandev-run.e2e.PuqLl6RH.log` (full files).
The final full desktop files passed all nine tests with retries disabled. Log:
`/tmp/youtrack-e2e-logs/kandev-run.e2e.sQMBV1VB.log`. Changed-file ESLint,
specification validation, and whitespace checks passed. Fresh exact-head CI and
review remain pending.


## Migrated MCP fixture follow-up

The failed-jobs-only rerun passed the unchanged runner-settings and cancellation
shards. The MCP explorer delivery assertion failed again, so its fixture required
a correction. Startup migration marks legacy profile configuration complete; the
resolver then intentionally ignores later legacy JSON writes. The explorer test
still configured its server through that legacy endpoint, making delivery depend
on whether an earlier test had restarted the shared backend.

Reproduced the identical assertion locally by migrating an empty legacy profile
before applying the old fixture. RED log:
`/tmp/youtrack-e2e-logs/kandev-run.e2e.AnPqtJyT.log`. The corrected fixture retains
that startup migration and asserts its completion, then creates and selects a
workspace MCP catalog definition through the current API. Cleanup restores the
prior profile selection and deletes only the test-owned definition. All existing
navigation, geometry, focus, tool-catalog, and delivery assertions remain. This
changes only `apps/web/e2e/tests/chat/mcp-status.spec.ts`, not MCP runtime behavior.

The corrected migrated-profile case passed three consecutive repetitions with
retries disabled. Log: `/tmp/youtrack-e2e-logs/kandev-run.e2e.fdDgnVPr.log`.
Changed-file ESLint and whitespace checks passed. The full MCP file passed all
three tests with retries disabled, including mixed connection/customization and
tablet geometry checks. Log: `/tmp/youtrack-e2e-logs/kandev-run.e2e.KkyxkfgH.log`.
Fresh exact-head CI/review remain pending.
