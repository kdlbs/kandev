# Mobile List visual acceptance record

**Status:** browser visual acceptance recorded; actual iPhone validation remains open.

The pre-change phone List screenshot and measured control heights are recorded
below. The rendered browser matrix and behavior checks passed. The real-device
blur check remains a required completion gate. [Task 05](task-05-preview-conformance.md)
owns this record.

## Build and fixtures

The worktree is uncommitted. `pnpm run build:vite` passed on 2026-10-27.
Screenshots use the isolated Playwright E2E fixture and seeded task/view states.
Phone captures use 393x852 CSS px unless named otherwise, at light and dark
themes; the fine-pointer phone is separately measured at the same viewport.
Responsive captures include 360x800, 767x900, 768x900, and 1024x900 CSS px.
All viewport and theme evidence is stored in `evidence/` as PNG plus geometry JSON.

## Pre-change baseline

Captured before UI edits with:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome e2e/tests/task/mobile-list-baseline.spec.ts
```

- Image: `evidence/phone-list-before-393x852.png` (944x2048 screenshot pixels).
- Viewport: 393x852 CSS px, mobile-chrome Pixel 5 profile, light theme.
- `mobile-topbar-page-context`: x=12, y=5.5, w=95.48, h=44 CSS px.
- `tasks-list-archive-modes`: x=16, y=72, w=361, h=44 CSS px.
- `tasks-list-search`: x=16, y=128, w=260.89, h=44 CSS px.
- `tasks-list-filters-trigger`: x=284.89, y=128, w=92.11, h=44 CSS px.
- The screenshot shows the fixed Active/Archived/All row and permanent search/Filters row. These rows are absent from ordinary List entry in UI-01; the search/Filters row returns only in UI-02.
- Existing investigation measurements for the layout choices (40px) and Select options (28px) remain separate control-family baselines. Capture every changed family at the same root font and viewport.

The baseline test passed: 1 discovered, 1 executed. Its output geometry was
recorded in the managed E2E log. This is a baseline only, not post-change
acceptance.

## Preview comparison

| Preview | State                            | Screenshot paths                                                                                                                                                             | Geometry/density                                                                                                                                                                    | Visual comparison                                                                                                                                                                                                                                              | Status                            |
| ------- | -------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- |
| UI-01   | Ordinary List, search hidden     | `ui-01-phone-list-{light,dark}-393x852.png`; `ui-01-phone-fine-pointer-393x852.png`; `ui-01-phone-fine-breakpoint-767x900.png`; `ui-01-phone-long-names-360x800.png`         | At 393x852: header x=12/y=5.5/h=44; tabs x=16/y=56/h=40; task body y=96/h=756. At 360px and 767px, names and controls stay inside the viewport.                                     | Matches: saved-view tabs and task body only; no search/filter row. Tabs own horizontal overflow; body owns vertical scroll.                                                                                                                                    | Matched in browser                |
| UI-02   | Search/results open              | `ui-02-phone-search-{light,dark}-393x852.png`                                                                                                                                | Search row x=0/y=96/w=393/h=52; task body y=148/h=704. The 44px header controls and 40px tabs retain their measured heights.                                                        | Matches: icon reveals one focused input/Filters row under tabs. Applied summary is tested separately and appears only when refinements are active.                                                                                                             | Matched in browser                |
| UI-03   | Temporary filters                | `ui-03-phone-result-filters-{light,dark}-393x852.png`                                                                                                                        | At 393x852, drawer y=449.5/h=402.5; three options exceed the 403px visible body and scroll internally. The shared drawer surface is visually inset by 8px.                          | Matches structure and content-sized sheet. The rendered title is the shorter localized “Filters” label instead of the schematic “Filter results”; accepted copy variation, no behavior change.                                                                 | Matched with copy note            |
| UI-04   | Left options, collapsed sections | `ui-04-phone-view-options-{light,dark}-393x852.png`                                                                                                                          | Outer visible surface x=8/y=253.5/w=377/h=590.5. Editor scroller x=16/y=474/w=361/h=362. Search is closed underneath; document overflow is false.                                   | Matches: layout selector and selected saved view lead into the shared editor; one scroll owner and no empty action gap.                                                                                                                                        | Matched in browser                |
| UI-05   | Four-rule sort expanded          | `ui-05-phone-four-rule-sort-{light,dark}-393x852.png` and `ui-05-phone-four-rule-sort-{light,dark}-lower-393x852.png`                                                        | Editor scroller x=16/y=382.9/w=361/h=453.1 with scrollHeight=1118. The lower capture shows rule four, Add sort, grouping, row settings, and automatic colors.                       | Matches: Running first, Red matching first, Orange matching first, Last activity newest first. Rules remain stacked and reachable in one internal scroller.                                                                                                    | Matched in browser                |
| UI-06   | Navigation, Tasks collapsed      | `ui-06-phone-navigation-{light,dark}-393x852.png` and `ui-06-phone-navigation-{light,dark}-lower-393x852.png`                                                                | At 393x852, menu container x=0/y=16/w=393/h=836; its navigation content scrolls internally. The fixed header and workspace picker are visible above it; document overflow is false. | Matches hierarchy, 16px top breathing room, and collapsed Tasks with no editor action. The existing global Search tasks quick action remains between workspace and New Task; the schematic omitted this pre-existing action. Accepted preservation difference. | Matched with existing-action note |
| UI-07   | Wider consumer compatibility     | `ui-07-coarse-tablet-768x900.png`; `ui-07-desktop-list-fine-1024x900.png`; `ui-07-desktop-threads-fine-1024x900.png`                                                         | 768px is the coarse-pointer tablet composition; at 1024px desktop List controls remain 28px and Threads controls 32px. No horizontal overflow.                                      | Matches: desktop sidebar/List controls remain independent; Threads controls and empty state remain intact.                                                                                                                                                     | Matched in browser                |
| UI-08   | Loading, empty, error            | `ui-08-phone-loading-light-393x852.png`; `ui-08-phone-empty-results-dark-393x852.png`; `ui-08-phone-error-light-393x852.png`                                                 | Each is 393x852 with header/tabs retained and feedback inside the task body. Document width equals viewport width.                                                                  | Matches: loading skeleton, empty-result clear action, and retry error state remain in the task body.                                                                                                                                                           | Matched in browser                |
| UI-09   | Explicit link/plugin override    | `ui-09-phone-link-scope-{light,dark}-393x852.png`; `ui-09-phone-plugin-group-override-{light,dark}-393x852.png`; `ui-09-phone-plugin-link-override-{light,dark}-393x852.png` | Link state adds a 106px search/refinement region; plugin sort/group controls stay inside the left editor scroller. All 393px captures have no document overflow.                    | Matches: link scope has a removable temporary chip; plugin overrides are identified as result options and do not replace saved view fields.                                                                                                                    | Matched in browser                |

Use the [required matrix](plan.md#required-visual-acceptance). Each row needs
light/dark and edge-case paths as applicable. Record actual dimensions and
visual observations, not only CSS class names or visibility.

## Real iPhone

| Environment                                                           | Cold load | Scroll  | Options/menu dismissal | Orientation | Label sharpness | Evidence                                  |
| --------------------------------------------------------------------- | --------- | ------- | ---------------------- | ----------- | --------------- | ----------------------------------------- |
| Safari, actual OS/device unavailable in this Linux environment        | Not run   | Not run | Not run                | Not run     | Unverified      | No actual iPhone capture available        |
| Installed PWA, actual OS/device unavailable in this Linux environment | Not run   | Not run | Not run                | Not run     | Unverified      | No installed iPhone PWA capture available |

Missing device evidence leaves the blur item unverified and acceptance incomplete.

## Behavioral and compatibility results

Results on 2026-10-27:

- `pnpm e2e:run --host --no-build --project mobile-chrome` with the 16 affected
  mobile specs: 62 passed, 0 failed.
- `pnpm e2e:run --host --no-build --project chromium` with the responsive,
  desktop sidebar sorting/filtering, and List preference specs: 14 passed,
  0 failed. The responsive test captured the fine-pointer phone, 767px phone,
  768px coarse tablet, desktop List, and desktop Threads views.
- The mobile suite verified saved-view reload, the complete ordered sort chain,
  page navigation, no-write temporary filters, search debounce/cancel/reset,
  archive/link scope, plugin facets, and workspace isolation.
- Focused frontend unit coverage (123 tests), `pnpm run typecheck`,
  `pnpm run i18n:check`, the focused changed backend tests, `pnpm run build:vite`,
  and `make -C apps/backend build` passed. The 3-test visual conformance run was
  repeated after the fresh Vite build and passed.
- The broad Go handler package passed. The broad SQLite package suite was
  stopped after 475.9 seconds while it was still running unrelated retention/
  office tests; focused workspace-search and paging tests passed. No failing
  assertion was reported by the stopped package run.
- SQLite tests ran. PostgreSQL-specific coverage remains skipped because
  `KANDEV_TEST_POSTGRES_DSN` is not configured in this environment.

## Code-review remediation results

Focused regressions ran on 2026-10-27 after the review fixes:

- Hook and adapter coverage passed. It proves request filters drive both row
  projection and inventory completeness; changed workflow scope and an
  archived link over active-only inventory use the bounded server query.
- `mobile-task-list-archive-navigation.spec.ts` and
  `mobile-task-list-link-overrides.spec.ts` passed. The archive test replaces a
  differently scoped saved view with `archive=archived`, `archive=all`, and
  `archive=active`. The link test checks effective no-group repository labels,
  title sort order, workflow grouping over a saved executor group, and no saved
  view mutation after temporary group collapse.
- `mobile-task-list-orientation-dialog.spec.ts` passed with the Rename dialog
  and draft retained from 393x852 to 852x393 and back. A hook test confirms the
  inactive phone host does not request a result page.
- The settled-search E2E verifies the 44px clear button stays within the input
  bounds and clears on tap.
- The four focused E2E specs passed 12/12 tests. The focused Vitest run passed
  28/28 hook tests. The broader focused Vitest run previously passed 57 tests
  across five files, and web typecheck passed. The E2E command rebuilt Vite
  assets successfully.
- CI-failing mobile scenarios passed after their selectors were aligned with
  the current phone UI: 35 affected cases across the broad run and targeted
  retries. The three mobile startup cases and eight narrow-navigation cases
  also passed. The GitLab row, workflow outcome, and damaged-task deletion
  cases each passed in their targeted reruns.
- Both preview capture specs now resolve evidence paths relative to their
  source files. They keep routine captures in Playwright output and update
  committed files only during an explicit `CAPTURE_PR_ASSETS=true` run.
- The PR run at `adde3d19407` reported five E2E shards failing on retry-only
  flakes. The selected-filter assertion now waits for the current option set,
  the phone retry target allows subpixel rounding at 44px, the GitHub page
  result gets a longer async deadline, and the plugin navigation test only
  toggles its disclosure when that control is present. The Quick Terminal
  drawer-close assertion allows for the mobile drawer transition. All five
  affected tests passed in focused one-worker reruns.
- Follow-up PR run `40458512805` reported two retry-only E2E flakes in the
  completed-session-resume and session-tab-restart specs. Both tests passed
  after the CI retry. Exact zero-retry replays in the CI runtime image passed
  shard 10 (142 passed, 1 skipped) and shard 11 (145 passed). Each failed test
  also passed four constrained repetitions at 2 CPUs and 4 GiB of memory. The
  restart case's initial two-idle-session poll had Playwright's default 5
  second timeout; it now uses the same bounded 30 second readiness wait as the
  adjacent restart cases. With retries disabled, the changed test passed four
  repetitions and the complete session-tab-restart spec passed all 5 tests.
  These local replays do not clear the failed CI result; a fresh PR run must
  verify the pushed fix.
- PR head `064b4593af59fa4b002d8b545990b954cafdd847` run
  `40464156388` ended with 85 checks passed, 11 skipped, and six failed checks
  (five E2E shard leaves plus the aggregate). The five test cases failed only
  on their first attempt and passed on retry; the workflow marks retry-only
  flakes as failures. Exact zero-retry replays of shards 2, 15, 17, 18, and 21
  on the same merge tree passed each of those five original cases. Shards 17,
  18, and 21 passed; shards 2 and 15 each exposed a different single test
  failure during replay.
- The two replay-only failures were the archive-link retry case in
  `mobile-task-list-filters.spec.ts` and the session-name assertion in
  `mobile-session-menu-gesture.spec.ts`. In fresh 2-CPU/4-GiB containers, the
  archive-link retry case passed five focused repetitions and the complete
  mobile List filters spec passed twice; the session-menu spec passed three
  repetitions. The original five CI-failing cases each passed two focused
  repetitions in fresh 2-CPU/4-GiB containers. These replays did not change the
  GitHub check result. A fresh PR run is still required to clear the failed
  E2E check.
- PR head `66c0d4c01c36d202be225f68366a3fe7ea820c70` run `40472095546` ended
  with 87 checks passed, 11 skipped, and four failed checks (three E2E shard
  leaves plus the aggregate). The retry summary had three retry-only flakes:
  the Kotlin LSP hover request, the mobile pause/resume queue, and a 44px mobile
  workflow target measuring 43.99997px. The pause/resume failure showed the
  8-second mock turn finishing while the test prepared its queue. The test now
  waits for the running-response signal and uses a 30-second turn. The LSP test
  now re-enters the current symbol position when the hover event is not
  observed. Related touch-target checks allow 0.5px for browser subpixel
  rounding. No retries were enabled in local verification:
  - Managed E2E build and Kotlin hover test: 1 passed; the test then passed
    twice more in a separate fresh managed run.
  - Pause/resume queue test: 1 passed, then 2/2 in a separate run.
  - Mobile workflow drawer test: 1 passed, then 2/2 in a separate run.
  - ESLint passed for all three changed E2E files.
  - `pnpm --dir apps/web run typecheck` and `pnpm --dir apps/web run
    build:vite` passed. The managed E2E run also rebuilt the backend and
    `build:e2e` assets. Documentation validation and `git diff --check` passed.
  A fresh PR run is required to validate these fixes on GitHub.
- PR head `a43fd70cd4c16767d3b6765a518c32babbeee13d` run `40476223800` ended
  with 88 checks passed, 11 skipped, and three failed checks (two E2E shard
  leaves plus the aggregate). The workflow-import test's five-second card poll
  could expire before the route refresh returned; it now waits for that refresh
  response before checking the card. The turn-changes test observed finalized
  backend history while the page still held its earlier history snapshot; it
  now reloads after finalization and waits for the page history response before
  measuring the card. Both tests passed one targeted no-retry run and then two
  more repetitions each in fresh managed runs, all with retries disabled. A
  new PR run is required to verify these fixes on GitHub.

- PR head `4ba96f070ec2ae2d5a297798752ae7f28ebb182b` run `40480462861` reached
  the 45-minute `pr-await` deadline with 86 checks passed, 11 skipped, three
  E2E shard leaves failed, and the backend plus E2E aggregate still pending.
  The failures were the mobile Stats recovery route accepting an uncontrolled
  retry, the terminal shell fixture using an unavailable default agent type,
  and the active-task storage test timing out with that same default profile.
  The Stats route now stays failed until Retry is tapped, and both task fixtures
  create an explicit `mock-agent` / `mock-fast` profile. Each affected scenario
  passed twice in focused managed E2E runs with retries disabled; both terminal
  shell scroll cases passed in those same repetitions. The PR branch also
  merged current `main` locally to resolve the two Traditional Chinese locale
  conflicts; the merge commit and these fixes require a fresh PR run after push.
  This run does not clear the CI result. The real-iPhone Safari/PWA Default-label
  check remains pending.

These browser tests do not replace the UI-01 through UI-09 screenshot review or
real-device proof. The Default workspace-label blur remains pending on actual
iPhone Safari and the installed PWA, as recorded below.

## Differences and resolution

The app menu retains the existing global Search tasks action between the
workspace picker and New Task. The schematic did not show it; it stays because
it is an existing navigation action and does not alter the new group order or
view editing ownership. The filter drawer says “Filters” rather than the
schematic “Filter results”; this is the existing shorter localized label. Both
differences are documented above and require no product correction.

The Default workspace label appears sharp in headless Chromium screenshots.
This environment does not reproduce native Safari/PWA top-edge compositing, so
that observation neither confirms nor disproves the user's reported blur.

## Final decision

The rendered browser UI matches the reviewed hierarchy, density, and scroll
ownership in UI-01 through UI-09, with the two accepted preview differences
above. Full acceptance remains incomplete until actual iPhone Safari and
installed-PWA evidence confirms the Default label before and after the required
scroll, drawer dismissal, orientation, and cold-load states.
