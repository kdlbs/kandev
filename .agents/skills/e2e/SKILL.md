---
name: e2e
description: Write and run web E2E tests (Playwright) using TDD — locations, patterns, commands, and debugging.
---

# E2E Tests

## Execution Context

Write and run E2E coverage directly in the primary conversation. For a
cost-controlled feature workflow, the user switches that conversation to the
lower-cost implementation/test model before this phase.

Write E2E tests using TDD (Red-Green-Refactor). Always run the tests you create and watch them fail before implementing.

## Related skills

- **`/tdd`** — Follow the Red-Green-Refactor cycle when writing tests.
- **`/pr-fixup`** — Use after the PR opens only for CI or reviewer findings.
- **`/playwright-cli`** — Interactive browser automation. Use to validate features against the dev server before writing tests, and to debug failing tests with `--debug=cli`.

## References

Load `references/fixture-state.md` for capability-readiness and remembered
workflow fixture rules.
Load `references/ui-state-and-cleanup.md` for lifecycle, WebSocket, terminal,
Dockview, and sidebar/context-menu rules.

## Location

`apps/web/e2e/`

```
apps/web/e2e/
├── fixtures/
│   ├── backend.ts           # Worker-scoped backend + frontend process
│   ├── test-base.ts         # Extended fixture (apiClient, seedData, testPage)
│   └── office-fixture.ts    # Office fixtures (officeApi, officeSeed with workspace+agent)
├── helpers/
│   ├── api-client.ts        # HTTP client for seeding data (read for available methods)
│   └── office-api-client.ts # Office-specific API client (onboarding, issues, agents)
├── pages/                   # Page objects (read for available pages and methods)
└── tests/                   # Spec files (*.spec.ts), grouped by feature
    ├── task/                # Task creation, deletion, archiving, environment, subtasks
    ├── kanban/              # Kanban board, mobile kanban, preview panel
    ├── session/             # Session lifecycle, resume, recovery, multi-session, layout
    ├── workflow/            # Workflow steps, settings, automation, import/export
    ├── git/                 # Git changes panel, commits, diffs, symlinks
    ├── pr/                  # PR detection, watchers, changes panel
    ├── terminal/            # Terminal agent, keyboard, settings
    ├── chat/                # Quick chat, message queue, clarification, markdown, toolbar
    ├── settings/            # Config management, agent profiles, editor integration
    └── review/              # Code review diffs
```

Each worker gets an isolated backend, frontend, database, and mock agent — no Docker, no API keys needed.

## Run commands

**Always run headless** (`make test-e2e`). Never use `--headed`, `e2e:headed`, or `test-e2e-headed` — headed mode requires a display and will fail in agent environments.

**Fresh worktree bootstrap:** Before the first pnpm or E2E command in a new
worktree, install the workspace dependencies:

```bash
cd apps && pnpm install --frozen-lockfile
```

Do this once before changing into `apps/web` or running a filtered package
command. Shared `.git` metadata does not include `apps/node_modules`.

### Preferred: `pnpm e2e:run` (managed runner — builds, runs, tears down)

`e2e/scripts/run-e2e.sh` handles the build, the run, and cleanup in one command. Use it instead of stitching the steps together. It auto-selects docker vs host, runs a resource-bounded number of shards concurrently, enforces one Playwright worker per shard and strict WS accounting by default (matching CI), and never leaves root-owned artifacts behind.

```bash
cd apps/web
pnpm e2e:run                                   # auto: docker if daemon + CI image available, else host; builds first
pnpm e2e:run tests/task/my-test.spec.ts        # single file (extra args pass through to Playwright)
pnpm e2e:run tests/path/spec.ts -- --grep "exact test name"  # exact CI failure with a fresh build
pnpm e2e:run --shards 3                          # 3 shards concurrently on this machine (isolated)
pnpm e2e:run --no-build -- --grep "task creation"  # runner options before --; Playwright options after
pnpm e2e:run --no-build --project mobile-chrome tests/layout/mobile-spa-resilience.spec.ts
pnpm e2e:docker                                # force the docker CI image (full isolation from a host dev instance)
pnpm e2e:clean                                 # remove build/test artifacts, incl. root-owned ones from prior docker runs
```

**Select the owning Playwright project.** The default `chromium` project
intentionally excludes routing, auth, mobile, and container suites; a matching
path with the wrong project exits with `No tests found`. Pass the project before
the spec path, for example:

```bash
pnpm e2e:run --project auth tests/auth/auth-lifecycle.spec.ts
pnpm e2e:run --project routing tests/office-routing-<name>.spec.ts
pnpm e2e:run --project containers tests/docker/<name>.spec.ts
```

Use `mobile-chrome` only for `mobile-*.spec.ts` files. Confirm Playwright discovers the intended test count before treating a focused command as evidence.

`e2e:run` accepts one `--project`; repeating it selects only the last value, so run desktop and mobile separately when both are required and confirm discovery for each.

See [resource-safety.md](references/resource-safety.md) before any full local test run.

The runner solves the sharp edges hand-rolling would hit: in docker it builds the CGO backend on the **host** and runs it in the runtime image (forward-compatible when the host glibc ≤ the image's — the usual case; it smoke-tests this and only falls back to the build image if the host is newer), builds the Vite web assets on the host, runs them through the Go-served SPA, and keeps Playwright output container-local. See `apps/web/e2e/README.md` → "the managed runner".

`--no-build` reuses Vite, backend, and packaged fixtures; prefer normal managed builds
after source/base changes. If intentional, rebuild both with `make -C apps/backend build`
and `make -C apps/backend e2e-plugin-package`; global setup requires the plugin tarball.

For raw Docker/SSH/container runs, `make build-backend` does not build the Linux
mock-agent fixture. Prefer managed; otherwise run `make build-backend
build-backend-remote-helpers build-web`. If `KANDEV_MOCK_AGENT_LINUX_BINARY` is
missing, run `make -C apps/backend build-mock-agent-linux` before diagnosing code.

### Raw commands (when you need fine control)

```bash
make test-e2e                                                      # all tests, headless (host)
cd apps && pnpm --filter @kandev/web e2e:raw -- tests/task/my-test.spec.ts  # single file
cd apps && pnpm --filter @kandev/web e2e:raw -- --grep "task creation" # workspace-filter syntax; from apps/web use `pnpm e2e:raw --project=mobile-chrome e2e/tests/<area>/<spec>.spec.ts` without a standalone `--`; run `--list` first if forwarding syntax is uncertain
```

For flake reproduction, load [failure-triage.md](references/failure-triage.md).

**CRITICAL: E2E tests run against the production Vite build served by the Go backend**, not dev mode. After any frontend code change, you **must** rebuild before running tests (`pnpm e2e:run` does this for you):

```bash
make build-web   # ~30s, required after every frontend change
```

Without this, tests can exercise stale code: after backend changes run `make -C apps/backend build` before reproducing Playwright failures, and compare the binary timestamp/hash if a fixed test still fails. `make test-e2e` and `pnpm e2e:run` handle both builds.

## Writing a test

1. Read `helpers/api-client.ts` and `pages/` to discover available seed methods and page objects; use `data-testid` attributes for selectors — add them to components as needed
2. Import fixtures from `../../fixtures/test-base` — provides `testPage`, `apiClient`, and `seedData` (pre-created workspace with default workflow). Pull `backend` from the fixture too when you need the backend URL — it's worker-scoped, dynamic, and `process.env.KANDEV_API_BASE_URL` is **not** set in the Playwright runner. Use `backend.baseUrl`.
3. Use page objects for common interactions; create new ones for new pages. For GitHub features, use `apiClient.mockGitHub*()` methods to seed mock data
4. Workspace-scoped mock events must carry the real `workspace_id`. `mockGitHubAssociateTaskPR` defaults an omitted ID to the active workspace; pass it explicitly for foreign-workspace cases, and keep production handlers fail-closed for missing or mismatched IDs. Direct-store bridges that inject Git status or inspect Changes must resolve `environmentIdBySessionId[sessionId]` and use environment-keyed state; sessions can share one environment.
5. For new status, hydration, or persistence assertions, inventory existing action/mutation scenarios and add coverage beside them, never replace them; preserve response/error, idempotency/duplicate-suppression, and desktop/mobile pointer/target checks, then run affected specs together.

### Input-modality behavior

For a touch-specific interaction, use Playwright `.tap()` rather than `.click()`
so the app receives a touch `pointerType`. Run focused mobile specs with
`pnpm e2e:run --project mobile-chrome e2e/tests/<area>/mobile-<name>.spec.ts`.
`--project` is a runner option and must precede `--`; the mobile project only
matches `mobile-*.spec.ts` files, so another filename can produce no tests.
After the interaction settles, assert the resulting state and exercise a later
mouse or pen entry when the UI maintains hybrid-device pointer state.

### Visual alignment regressions

For a UI change whose contract is a rendered size or alignment relationship,
assert that relationship from the intended elements' bounding boxes rather than
only asserting visibility. Scope locators to the affected toolbar, dialog, or
panel so unrelated controls cannot make the assertion pass.

```typescript
const metrics = page.getByTestId("task-metrics");
const actions = page.getByTestId("task-actions");
const [metricsBox, actionsBox] = await Promise.all([
  metrics.boundingBox(),
  actions.boundingBox(),
]);

expect(metricsBox).not.toBeNull();
expect(actionsBox).not.toBeNull();
expect(metricsBox!.height).toBeCloseTo(actionsBox!.height, 1);
```

Run the assertion in the relevant desktop and mobile projects when responsive layout can change the result. Do not rely on fixed pixels when the product contract is equality or alignment.
For computed colors, parse alpha/opacity semantically or assert a deliberate class/data contract; do not compare serialized `getComputedStyle` strings because browsers may return `rgba()`, `oklab()`, or `color()`.

**Animation-aware geometry:** Before reading dialog or panel geometry, wait only for currently running Web Animations with finite `effect.getComputedTiming().iterations`; await `animation.finished.catch(() => undefined)` because Radix overlays can cancel animations during close or replacement. Never blanket-await infinite animations or use a fixed sleep; then read bounding boxes and assert the relationship.

For narrow-width clipping or overlap regressions, visibility and containment
are insufficient: assert a real hit target. Check `document.elementFromPoint()`
at the control center resolves to the control (or its descendant), then prove
the action remains clickable at the legal minimum width.

### IDs and response shapes — common pitfalls

- **`apiClient.createTaskWithAgent(...)` returns `CreateTaskResponse`**, which is `Task & { session_id?: string; agent_execution_id?: string }`. Read `created.session_id` directly — don't call `listTaskSessions(taskId)` just to fetch the session that was auto-started by the same call.
- **The URL `/t/:id` contains the TASK ID**, not the session ID. Backend routes like `/port-proxy/:sessionId/:port/*path` expect the session ID. Don't extract IDs from `window.location.pathname` when you need a session ID — pull from the API response.
- **`page.request` shares cookies/storage with the page context**. Fine for the current no-auth local backend; if auth ever lands, this is where you'd plug it in.
- **Go boot-payload data is available before React mounts.** Routes that hydrate from `window.__KANDEV_BOOT_PAYLOAD__` may not issue a browser-visible API request on first paint. Use `apiClient` to seed or re-query backend state, assert the user-visible outcome, and reserve `page.waitForResponse("**/api/v1/...")` for client-side fetches that the browser actually performs.
- **Preview iframe tests:** the seed repo has no `dev_script` configured, so the preview panel renders a placeholder ("Configure a dev script…") and the URL input never appears — tests that try to drive it hang on the locator timeout. To use the preview iframe in a test, set one first: `await apiClient.updateRepository(seedData.repositoryId, { dev_script: "echo dev" })`. Then click the Preview dockview tab (`await session.clickTab("Preview")`) — the toolbar will mount and the URL input becomes targetable.

Example:

```typescript
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("my feature", () => {
  test("does something", async ({ testPage, seedData, apiClient }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Test Task", "Description");
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workspaceId);
    await expect(kanban.taskCardByTitle("Test Task")).toBeVisible();
  });
});
```

## Dev-first workflow

For interactive development and PR captures, load
[dev-workflow.md](references/dev-workflow.md).
Production-build verification remains required after development.

## Test organization

Tests are grouped by feature area in subdirectories under `tests/`. When creating a new test:

- **Place it in the matching feature directory.** A test for PR detection goes in `pr/`, a test for session resume goes in `session/`, etc.
- **Merge related tests into the same file.** Tests covering the same feature (e.g., git commit body and pre-hooks) belong in one file with separate `test.describe` blocks. Don't create a new file for each narrow scenario.
- **Import paths from subdirectories** use `../../` (e.g., `from "../../fixtures/test-base"`).
- **Standalone root files** are allowed for truly cross-cutting tests that don't fit any group.
- **Extract shared helpers.** Extract helpers into a sibling `*-helpers.ts` file whenever they are used by multiple spec files, even when small; keep only scenario-specific setup in specs. Reusable page polling, seeding, and Dockview cleanup belong in the helper module.

## Test quality guidelines

- **Test through the UI, not the API.** E2E tests verify user-facing behavior. Don't write tests that only call the API and assert the response -- those are integration tests. Instead, navigate to the page, interact with UI elements, and assert what the user sees; use `toContainText` or a dedicated locator when labels include metadata such as file sizes.
- **Verify persistence with page reload.** After changing a setting or creating data, reload the page (`testPage.reload()`) and assert the state is still correct. This catches hydration bugs and Go boot-payload/client-store mismatches.
- **Restore patched persisted settings.** When a test PATCHes user settings, capture the baseline and restore it in `test.afterEach`. The backend is worker-scoped, and `e2eReset` does not reset every persisted setting, including `system_metrics_display`; leaking one can affect later tests in the same worker. Fixtures are lazy: acquire `testPage` before setting a non-default persisted value in `beforeEach`, otherwise page initialization can reapply the default and silently undo setup. Verify with the focused test that depends on that setting.
- **Restore patched shared persisted state.** The worker-scoped backend and
  `e2eReset` do not reset every seeded record. A test that creates or PATCHes a
  canonical `seedData` profile, repository, executor, integration/workspace
  preset, or other non-user setting must use unique names for shared/global
  rows, capture every changed baseline, and restore every mutation in
  `test.afterEach` or `finally` (not only delete rows created by the test).
  Prefer a disposable record when the UI can select it. Verify by running the mutating spec followed by its affected neighbour with `--workers=1 --retries=0`. Remove temporary or untracked files written into worker-scoped checkouts in `finally`/`afterEach`, including assertion-failure paths.
- **Pass browser-evaluation values explicitly.** `locator.evaluate` and
  `page.evaluate` callbacks execute in the browser, so they cannot close over
  Node/test variables. Pass expected values as the argument instead, for
  example `locator.evaluate((el, expected) => Math.abs(el.scrollTop - expected), baseline)`.
- **Reset pointer state before hover assertions.** A prior click can leave the
  hover target active; move to a neutral page location before asserting hover UI.
- **Measure asynchronous layout from a settled baseline.** When the initiating
  UI action changes layout before its delayed result arrives, delay the mock
  response and capture the baseline after that synchronous layout settles. Then
  assert the absolute deviation after the asynchronous content appears. This
  attributes movement to the result rather than the initiating action and
  catches movement in either direction.
- **Scroll-positioned markers.** For unread dividers, restore points, or search
  anchors, seed content taller than the viewport and assert the marker's bounds
  are inside the viewport after navigation. A short transcript's DOM-visible
  marker does not prove initial scroll behavior; cover every renderer/viewport
  strategy selected at runtime.
- **Nested Escape controls.** If an inner panel inside a Radix Dialog handles Escape, intercept the key in capture phase and call both `preventDefault()` and `stopPropagation()` before dismissing the inner panel. A bubble-phase window handler runs after Radix can dismiss the outer dialog. Add a regression that asserts the inner panel collapses while the outer dialog remains open.
- **Seed via API, assert via UI.** Use `apiClient` to set up preconditions quickly, but always verify the result by opening the page and checking the DOM.

## Debugging failures

For failed specs, shard artifacts, or suspected contention, load
[failure-triage.md](references/failure-triage.md).

## Selector guidelines

- **Prefer `data-testid` selectors** over text-based locators. Text content can change when UI is updated (e.g., hiding a badge), breaking tests that match by text. Use `getByTestId()` or `locator("[data-testid='...']")` for stable targeting. When translated labels intentionally identify multiple routes, scope by stable `href` or a dedicated test ID rather than role/name alone.
- **Scope Radix and responsive locators to the active instance.** Tooltips may use `instant-open`, `delayed-open`, or `open`; use `[data-slot="tooltip-content"]:not([data-state="closed"])`, then scope to the visible portal/popover/container and active ancestor. For routes with multiple surfaces, scope controls to the active container first; portal overlays must be selected by visible overlay or the trigger's `aria-controls`, not assumed descendants. For portaled pickers, locate the active visible `role=listbox` or picker container first, then scope `getByRole('option')` within it. Hidden mounts can make global locators match the wrong instance; do not use `.first()` to hide duplicates.
- **Use exact dynamic labels and page objects:** `filter({ hasText })` is substring matching; for counts or sibling content, use a stable `data-*` attribute or `getByText(label, { exact: true })` and assert uniqueness. Prefer page object methods like `clickSessionChatTab()` (stable `data-testid`) over fragile text matches such as `sessionTabByText("1")`.
- **Dropdown menus can detach** from the DOM when React re-renders the parent (e.g., WS events updating the sidebar). The `openSidebarMenuAndClick()` helper in `session-page.ts` retries the full open-click sequence on detachment — use this pattern for similar interactions.

## TDD workflow

Follow `/tdd` when writing E2E tests:

1. **RED** — Write the spec and run a defect-specific expected-vs-actual assertion that fails before production changes; a missing selector/testid is scaffold failure, not behavioral RED. Add stable selectors as setup, or label selector-only RED separately. For a pure state/data UI regression, if a component/unit RED test already proves the bug and the E2E fixture cannot deterministically reproduce the pre-fix state without production wiring, add the E2E assertion after the minimal fix, run it against a fresh managed production build, and record why E2E was not the RED gate.
2. **GREEN** — Implement the feature/fix, add `data-testid` attributes, run the test until green
3. **REFACTOR** — Extract page objects, clean up selectors, keep tests green
4. Run the targeted E2E spec when done and report that final change-aware verification is required as a separate planner assignment
