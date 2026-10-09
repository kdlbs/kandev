---
id: "04-seeded-comparison"
title: "Launch an isolated seeded navigation comparison"
status: done
wave: 4
depends_on:
  - "03-phone-navigation"
plan: "plan.md"
requirements:
  - REQ-UI-NAV-HIERARCHY-001
  - REQ-UI-NAV-HIERARCHY-002
  - REQ-UI-NAV-HIERARCHY-003
acceptance_criteria:
  - AC-UI-NAV-HIERARCHY-001.1
  - AC-UI-NAV-HIERARCHY-001.2
  - AC-UI-NAV-HIERARCHY-001.3
  - AC-UI-NAV-HIERARCHY-001.5
  - AC-UI-NAV-HIERARCHY-002.2
  - AC-UI-NAV-HIERARCHY-002.3
  - AC-UI-NAV-HIERARCHY-002.4
  - AC-UI-NAV-HIERARCHY-003.1
  - AC-UI-NAV-HIERARCHY-003.3
  - AC-UI-NAV-HIERARCHY-003.5
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
---

# Task 04: Launch the seeded comparison

## Current comparison

The initial `/tmp` instance recorded below expired. The current preview is
`http://127.0.0.1:37497/tasks?workspaceId=31bd354e-3f7a-4b9f-816f-a5360eede356`.
It reflects the latest removal of decorative vertical rules. Its fresh Compass
Studio dataset contains 11 tasks (including six subtasks), two prepared sessions
with real file changes, two mock issues, two linked mock PRs, and one paused
automation. Its data is independent of the original preview and other instances.

Current artifacts and provenance:
`/home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa`.
The exact stop command is
`python3 /home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa/stop.py`.
The initial results below remain historical evidence, not the current runtime.

## Summary

Leave the implemented branch running with disposable, coherent task and provider
data. Give the user a verified URL and desktop/phone screenshots to compare with
the supplied references. This is an implementation preview, not marketing capture.

## In scope

- Apply `/product-demo-seeding` and `/playwright-cli`; use the user's requested
  changed branch instead of the skill's current-main marketing source gate.
  Record baseline `75a37f34eb6bd0e0a46e9a35a46491bb9f52f7ef`, current HEAD, and a
  hash of the implementation diff. No commit is needed to make the preview truthful.
- Create a fresh artifact root, home/data/database, fictional Git repository,
  worktree root, browser profile, logs/PID files, and collision-free ports.
- Reuse `ApiClient` seed methods and fixture launch contracts. A persistent
  temporary launcher must use an allowlisted environment with mock providers,
  controlled mock agents, and `KANDEV_E2E_MOCK=true` when session seed APIs are
  needed. Do not modify production launch scripts for this comparison.
- Seed, inspect, rehearse the actual creation/filter/task/Issues controls, then
  reset/reseed the baseline so the user receives the intended starting state.
- Keep the preview running for the user; retain its stop command and ownership
  record. Do not tear it down immediately after verification.

`scripts/dev-isolated --web` is the reference for persistent process ownership
and sanitized launch environment. Its current `env -i` does not forward
`KANDEV_E2E_MOCK`, so merely prefixing that variable is insufficient for session
seed routes. The E2E worker fixture supports the seed flag but tears itself down.
Use a small external harness combining those established contracts; never pause
a Playwright test indefinitely just to keep its worker alive.

## Seed contract

Story: a developer finds a task, checks review progress, and reaches project
issues using a clear navigation hierarchy on desktop and phone.

| Object | Baseline |
| --- | --- |
| Workspace | Compass Studio, regular mode |
| Repository | Fictional `compass/app`, disposable local Git checkout |
| Workflow | To do, In progress, Review, Completed; preserve actual state mapping |
| Tasks | Five total: one todo, one in progress, two review, one completed |
| Titles | Fix issue search; Improve empty states; Review keyboard navigation; Check mobile filters; Restore draft after reload |
| Views | State-grouped All tasks, plus one filtered review view with a saved active-filter cue |
| GitHub | Mock connection, two coherent issues and two linked PRs with deterministic fixture content |
| Automation | One paused automation with a readable name; navigation opens history and never runs it |
| Theme/locale | Dark English baseline; also inspect light and Portuguese |
| Account | Show actual test auth only if seeded through supported auth setup; otherwise no invented account identity |

Use a stored anchor timestamp and canonical seed hash. API-generated IDs may
differ across resets; record their mapping and hash the canonical narrative.
Use actual repository changes or supported mock seed APIs for diff statistics;
never patch displayed text or paint fake statuses in the DOM. Missing optional
stats remain missing if a truthful source cannot be seeded. Add a second empty
workspace during rehearsal to prove setup/workspace isolation, then remove that
rehearsal-only state before handing over the five-task baseline.

## Out of scope

Real accounts/tokens, production data, copied databases, changing the user's main
instance, public exposure, marketing video, commits, or installing plugins.

## Acceptance

1. Running backend/frontend paths, builds, environment, ports, database, and seed
   prove isolation and the implemented branch. Mock routing is verified before
   any provider operation. The changed UI matches UI-01/UI-02 structurally.
2. Real desktop and phone controls successfully create/open a task, filter/save,
   collapse groups, and reach the seeded Issues list; refreshed baseline counts
   are exact. Capture 1280px desktop, 393px phone, and 767px phone with dark/light
   examples and no overflow or blocked actions.
3. Return the verified usable URL, seed name, screenshots, logs/PID record, and
   exact stop command. Leave only the intended preview alive. Record any failed
   readiness or reachability check rather than claiming the instance works.

## ASCII UI preview

UI-01/UI-02 comparison states, detailed in the [plan](plan.md#ascii-ui-preview):

```text
Desktop                        Phone drawer
Workspace / primary New Task   Workspace / primary New Task
Home                           Home / quick actions
Integrations v                 Integrations v
  GitHub                         GitHub -> Issues -> 2 rows
TASKS / All tasks               TASKS / All tasks
  To do          1               compact touch task rows
  In progress    1             Utilities / Settings
  Review         2
  Completed      1
Settings / actual account
```

The seed's four groups are representative data, not hardcoded product states.
The app remains the source of all rendered controls. The task-title picker and
existing provider view picker remain separate native phone surfaces.

## Verification and launch commands

First author an external `compare.ts` harness and `seed.json` under the new
`NAV_COMPARISON_ROOT`. These are proposed temporary artifacts, not existing repo
commands. The harness accepts `start`, `seed`, `check`, and `stop`; `check` verifies
health, source/build identity, mock routing, exact API counts, and the rendered
routes, writing evidence to its root. It derives unique runtime paths/ports,
launches detached owned processes, and records `runtime.json` for later commands.
Keep imports tied to this checkout's `ApiClient` and installed Playwright.

From repository root, after setting `NAV_COMPARISON_ROOT` to that actual directory:

```bash
comparison_source="$PWD"
comparison_root="${NAV_COMPARISON_ROOT:?Set the fresh comparison artifact directory}"
make -C apps/backend build
make build-web
test -f "$comparison_root/compare.ts"
test -f "$comparison_root/seed.json"
pnpm --dir apps/web exec tsx "$comparison_root/compare.ts" start --source "$comparison_source"
pnpm --dir apps/web exec tsx "$comparison_root/compare.ts" seed
pnpm --dir apps/web exec tsx "$comparison_root/compare.ts" check
```

Use the printed URLs with a fresh Playwright browser profile. Rehearsal mutations
must be removed by reseeding before the final `check`. Record the concrete command
and output, not just this planned invocation. The temporary harness may serve
the freshly built Vite bundle through the backend, avoiding an unnecessary second
web process. If it uses Vite, configure its explicit backend port and record both.

Do not run the following stop command until the comparison is finished; return
it to the user with the concrete artifact path:

```bash
pnpm --dir apps/web exec tsx "$NAV_COMPARISON_ROOT/compare.ts" stop
```

The stop action checks recorded process ownership, stops only those groups,
verifies their ports are released, and removes owned runtime data. Keep final
screenshots/provenance. No production instance, tunnel, or credential is touched.

## Files likely touched

- Temporary `compare.ts`, `seed.json`, `runtime.json`, browser profile, logs,
  database, disposable repository, and screenshots under `NAV_COMPARISON_ROOT`.
- This work order and `plan.md` for actual URLs, artifact paths, hashes, counts,
  checks, and intentional retained processes.
- Paired draft spec statuses only after all applicable work orders pass and
  implementation matches the design.

## Dependencies

Task 03 and all preceding task-defined checks. Load backend guidance before
launch and demo isolation references before seeding.

## Risks

Development-profile mock mode and E2E seed-route availability are separate facts.
Do not proceed on live provider fallback, occupied ports, unknown process
ownership, or an unreachable URL. A dev server starting does not prove the user
can reach it; use the environment's available port exposure and report its actual
address without exposing it publicly.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/navigation-hierarchy.md) and
  [design](../../specs/ui/system-design/navigation-hierarchy.md).
- `scripts/dev-isolated`, `apps/web/e2e/fixtures/backend.ts`,
  `apps/web/e2e/helpers/api-client.ts`, and `apps/web/e2e/pages/mobile-github-page.ts`.
- `.agents/skills/product-demo-seeding/SKILL.md` and its isolation reference.

## Results

Implemented and left running on 2026-09-28.

- URL: `http://127.0.0.1:37497/tasks?workspaceId=6f4eb8c7-75f5-4ece-9187-2f8065a86836`.
- Artifact root: `/tmp/kandev-navigation-compare-z54_iuek`.
- Owned backend PID: `1652633`; PID and allowlisted environment recorded in `backend.pid` and `runtime.json`.
- Executable: artifact `build/bin/kandev`; static Vite bundle: artifact `build/web`. Both were copied from this branch's fresh builds. Backend serves frontend on the same loopback port; no public tunnel was created.
- Isolated HOME: artifact `home`; Kandev home and database: `data/compare.db`; Git checkout: `home/compass/app`; worktrees: `worktrees`; managed clones: `managed`; agentctl range: `61200-61215`.
- Logs: `backend.log` (process output) and `data/logs/backend-logs.log` (structured log). Startup proves mock agent, GitHub, GitLab, Jira, Linear, and Sentry providers. No developer credentials or database were copied.
- `provenance.json` records baseline/current HEAD `75a37f34eb6bd0e0a46e9a35a46491bb9f52f7ef`, dirty source-diff and added-file hashes, binary/web-index hashes, and canonical seed hash. The comparison intentionally uses the uncommitted implementation branch.

Exact API checks passed after reset/reseed: one Compass Studio workspace, one Delivery workflow, five unique tasks (TODO 1, IN_PROGRESS 1, REVIEW 2, COMPLETED 1), four seeded sessions, two issues, two PRs, and one disabled automation. Delivery uses the native simple workflow's Backlog/In Progress/Review/Done steps; the sidebar explicitly groups by task state. The second saved view filters the Review workflow step and displays the active-filter cue. The fictional local remote is a file URL under the artifact root.

Browser rehearsal used the real controls to create a sixth task, save a filter, collapse/reopen groups, open a task, and reach the two-issue list from the phone menu. A second empty workspace proved isolation and was removed. The harness then cleared task/session/automation state and reseeded the exact five-task baseline. Final browser checks passed at 1280px, 393px, and 767px, in dark/light and Portuguese, with no page errors or horizontal overflow. Browser profile: artifact `browser`. `check.json`, `browser-check.json`, and provider verification JSON files contain the read-back evidence.

Screenshots under the artifact `screenshots` directory:

- `desktop-1280-dark.png` and `desktop-1280-light.png`.
- `desktop-saved-review-filter.png`.
- `phone-393-dark.png`, `phone-393-light.png`, and `phone-393-portuguese.png`.
- `phone-767-dark.png` and `phone-767-light.png`.
- `phone-393-issues.png` and `phone-767-issues.png`.

Optional task diff counts are absent because these seeded sessions do not own live Git snapshots. PR metadata comes from the supported mock API; no display text or stats were fabricated. Actual diff-stat and action containment behavior passed the existing mock-agent Git-change E2E scenario.

Executed from this checkout with the pinned mise toolchain:

```bash
/home/jcfs/.local/bin/mise exec -- make -C apps/backend build
/home/jcfs/.local/bin/mise exec -- pnpm --dir apps/web e2e:run --host --project chromium tests/layout/navigation-hierarchy.spec.ts
/home/jcfs/.local/bin/mise exec -- pnpm --dir apps/web exec tsx /tmp/kandev-navigation-compare-z54_iuek/compare.ts start
/home/jcfs/.local/bin/mise exec -- pnpm --dir apps/web exec tsx /tmp/kandev-navigation-compare-z54_iuek/compare.ts seed
/home/jcfs/.local/bin/mise exec -- pnpm --dir apps/web exec tsx /tmp/kandev-navigation-compare-z54_iuek/compare.ts check --rehearse
/home/jcfs/.local/bin/mise exec -- pnpm --dir apps/web exec tsx /tmp/kandev-navigation-compare-z54_iuek/compare.ts seed
/home/jcfs/.local/bin/mise exec -- pnpm --dir apps/web exec tsx /tmp/kandev-navigation-compare-z54_iuek/compare.ts check
```

The managed E2E command above builds the production web bundle. Re-running `start` refreshes that bundle only after verifying the recorded process ownership; it does not create another server. Initial rehearsal corrections were confined to the temporary harness (structured-log path, workspace-scoped GitHub status, terminal timestamps, filter dimensions, and explicit creation preferences). All final checks passed.

The verified address is local to this development host. No port-forwarding capability was exposed by available tools; the URL is not advertised as a public or remote phone endpoint. The preview remains alive for comparison.

Stop only this owned preview when finished:

```bash
/tmp/kandev-navigation-compare-z54_iuek/stop.sh
```

The wrapper runs `compare.ts stop`, checks PID/executable/home ownership, stops the owned process group, verifies backend/agentctl port release, preserves structured logs and evidence, then removes only disposable runtime data. No commit or push was made.


## Refinement refresh (2026-09-28)

The same owned process and URL now serve the revised action area and single-row
footer. No reseed or database reset was performed. The user's ungrouped All tasks
view remains intact; the capture harness permits that presentation while still
checking the seeded task counts, saved review filter, and all launchers.

The updated browser harness verifies the 44px New Task action, visible Quick Chat
and Terminal labels, a single-row footer, and the labelled utilities menu.
Captures disable transient CSS animations. Dark/light desktop, 393px and 767px
phones, issue lists, and Portuguese checks passed with no page errors. The earlier
captures are retained in `screenshots-iteration-1`; updated images remain in
`screenshots`, including `desktop-footer-menu.png`. API checks retain the original
five tasks/four sessions/two issues/two PRs/paused automation. Served assets match
the current branch build. The isolated runtime remains running.

The final refresh observed one seeded task transition from In progress to Review.
That live state was retained. `compare.ts check --api-only --preserve-edits`
records current state counts while continuing to verify task identity/counts,
provider fixtures, isolation, health, and served assets. The exact baseline check
remains available without `--preserve-edits`; no reset is needed for subsequent
interactive comparison.


## Quiet hierarchy comparison (2026-09-28)

`quiet-study.mts` compared neutral filled and outlined action treatments in an
isolated browser page before selecting the implementation. `quiet-capture.mts`
captures the real served build without style overrides. Its `quiet-final` images
include desktop dark/light, phone dark/light, desktop Portuguese, and keyboard
focus. It checks text contrast, live target geometry, Portuguese/Japanese label
containment, and page errors. `quiet-check.json` records the measurements.
The regular `compare.ts` ownership/build/fixture checks continue to run with
`--api-only --preserve-edits`; task data and saved grouping are not reset.
