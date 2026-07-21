---
name: e2e
description: Write and run web E2E tests (Playwright) using TDD — locations, patterns, commands, and debugging.
---

# E2E Tests

## Execution Context

Load `/planner-orchestration`. The user-started primary session delegates this
procedure to an `implementer` or `test-engineer` worker and does not write or run
E2E tests directly. An explicitly assigned worker continues below and does not
spawn other workers.

Write E2E tests using TDD (Red-Green-Refactor). Always run the tests you create and watch them fail before implementing.

## Available skills and subagents

- **`/tdd`** — Follow the Red-Green-Refactor cycle when writing tests.
- **`/verify`** — The planner launches this separately after targeted E2E tests pass.
- **`/playwright-cli`** — Interactive browser automation. Use to validate features against the dev server before writing tests, and to debug failing tests with `--debug=cli`.

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

### Preferred: `pnpm e2e:run` (managed runner — builds, runs, tears down)

`e2e/scripts/run-e2e.sh` handles the build, the run, and cleanup in one command. Use it instead of stitching the steps together. It auto-selects docker vs host, runs N shards concurrently, enforces strict WS accounting by default (matching CI), and never leaves root-owned artifacts behind.

```bash
cd apps/web
pnpm e2e:run                                   # auto: docker if daemon + CI image available, else host; builds first
pnpm e2e:run tests/task/my-test.spec.ts        # single file (extra args pass through to Playwright)
pnpm e2e:run tests/path/spec.ts -- --grep "exact test name"  # exact CI failure with a fresh build
pnpm e2e:run --shards 3                          # 3 shards concurrently on this machine (isolated)
pnpm e2e:run --no-build -- --grep "task creation"  # skip rebuild; forward flags after --
pnpm e2e:docker                                # force the docker CI image (full isolation from a host dev instance)
pnpm e2e:clean                                 # remove build/test artifacts, incl. root-owned ones from prior docker runs
```

The runner solves the sharp edges hand-rolling would hit: in docker it builds the CGO backend on the **host** and runs it in the runtime image (forward-compatible when the host glibc ≤ the image's — the usual case; it smoke-tests this and only falls back to the build image if the host is newer), builds the Vite web assets on the host, runs them through the Go-served SPA, and keeps Playwright output container-local. See `apps/web/e2e/README.md` → "the managed runner".

`--no-build` reuses every production E2E artifact, not only Vite assets and the
backend executable. On a fresh worktree or after rebasing, confirm packaged
fixtures also exist; global setup currently requires
`apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz`. If it is absent, run
without `--no-build` or first run `make -C apps/backend e2e-plugin-package`.
Prefer a normal managed build after source or base-branch changes and reserve
`--no-build` for repeated runs against unchanged artifacts.

### Raw commands (when you need fine control)

```bash
make test-e2e                                                      # all tests, headless (host)
cd apps && pnpm --filter @kandev/web e2e -- tests/task/my-test.spec.ts  # single file
cd apps && pnpm --filter @kandev/web e2e -- --grep "task creation" # by name
```

### Flake reproduction

Start by matching CI as closely as possible, then add pressure deliberately:

1. Run the exact failed shard in the CI runtime image with CI env enabled:
   ```bash
   docker run --rm --ipc=host -v "$PWD":/work -w /work/apps/web \
     -e CI=true -e GITHUB_ACTIONS=true -e GITHUB_WORKSPACE=/work \
     -e NODE_OPTIONS=--dns-result-order=ipv4first \
     -e PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
     ghcr.io/kdlbs/kandev-ci:runtime-latest \
     bash -lc 'git config --global --add safe.directory /work 2>/dev/null; npx playwright test --config e2e/playwright.config.ts --project=chromium --project=mobile-chrome --shard=10/10 --reporter=list'
   ```
2. If the exact shard passes, constrain container resources and repeat the
   failing spec/test. GitHub-hosted runners can expose timing bugs that a roomy
   local machine hides:
   ```bash
   docker run --rm --ipc=host --cpus=2 --memory=4g --memory-swap=4g \
     -v "$PWD":/work -w /work/apps/web \
     -e CI=true -e GITHUB_ACTIONS=true -e GITHUB_WORKSPACE=/work \
     -e NODE_OPTIONS=--dns-result-order=ipv4first \
     -e PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
     ghcr.io/kdlbs/kandev-ci:runtime-latest \
     bash -lc 'git config --global --add safe.directory /work 2>/dev/null; npx playwright test --config e2e/playwright.config.ts --project=mobile-chrome e2e/tests/terminal/mobile-terminal-keybar.spec.ts --grep "user presses an OS-keyboard letter while no modifier is active" --repeat-each=30 --reporter=list'
   ```
3. Preserve nearby test ordering when a single-test repeat stays green. Run the
   full spec file or full shard with the same resource limits before declaring
   a flake non-reproducible.

Record the exact command, resource limits, repeat number, and failure artifact
path. Always inspect `error-context.md`; mobile/terminal flakes often show
state that the stack trace alone hides, such as duplicate active terminals or a
terminal stuck on "Starting terminal...".

When a PR-specific E2E shard fails, first identify the failed spec(s). If failures are in unrelated existing specs and no changed code plausibly affects that surface, record the failure as unrelated in the PR fixup summary instead of changing unrelated tests.

**CRITICAL: E2E tests run against the production Vite build served by the Go backend**, not dev mode. After any frontend code change, you **must** rebuild before running tests (`pnpm e2e:run` does this for you):

```bash
make build-web   # ~30s, required after every frontend change
```

Without this, tests run against stale code and failures are misleading. `make build-backend` is also required after Go changes. `make test-e2e` and `pnpm e2e:run` handle both automatically.

## Writing a test

1. Read `helpers/api-client.ts` and `pages/` to discover available seed methods and page objects
2. Import fixtures from `../../fixtures/test-base` — provides `testPage`, `apiClient`, and `seedData` (pre-created workspace with default workflow). Pull `backend` from the fixture too when you need the backend URL — it's worker-scoped, dynamic, and `process.env.KANDEV_API_BASE_URL` is **not** set in the Playwright runner. Use `backend.baseUrl`.
3. Use `data-testid` attributes for selectors — add them to components as needed
4. Use page objects for common interactions; create new ones for new pages
5. For GitHub features, use `apiClient.mockGitHub*()` methods to seed mock data

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

Run the assertion in the relevant desktop and mobile projects when responsive
layout can change the result. Do not rely on fixed pixels when the product
contract is equality or alignment.

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

Before writing an E2E test, validate the feature works interactively using `playwright-cli` against a dev server. This gives a fast feedback loop — code changes are picked up by hot reload in ~1-2 seconds, no production rebuild needed. Once confirmed working, translate the interactions into a proper E2E test.

### Start the dev environment

Multiple agents may run in parallel, so use random ports to avoid collisions. Fixture ports auto-offset from 18080 (backend) and 13000 (frontend) using `E2E_PORT_OFFSET` (derived from `PID % 30` by default) — stay outside those ranges. Parallel E2E test runs are safe by default.

```bash
OFFSET=$((RANDOM % 100))
BACKEND_PORT=$((19000 + OFFSET))
FRONTEND_PORT=$((14000 + OFFSET))
```

Start the backend:
```bash
E2E_TMP=$(mktemp -d) && mkdir -p "$E2E_TMP/.kandev" && \
printf '[user]\n  name = E2E Test\n  email = e2e@test.local\n[commit]\n  gpgsign = false\n' > "$E2E_TMP/.gitconfig" && \
HOME="$E2E_TMP" KANDEV_HOME_DIR="$E2E_TMP/.kandev" KANDEV_SERVER_PORT=$BACKEND_PORT \
KANDEV_DATABASE_PATH="$E2E_TMP/kandev.db" KANDEV_MOCK_AGENT=only \
KANDEV_MOCK_GITHUB=true KANDEV_DOCKER_ENABLED=false KANDEV_WORKTREE_ENABLED=false \
KANDEV_LOG_LEVEL=warn apps/backend/bin/kandev &
```

Start the dev frontend:
```bash
KANDEV_API_BASE_URL=http://localhost:$BACKEND_PORT NEXT_PUBLIC_KANDEV_API_PORT=$BACKEND_PORT \
pnpm --filter @kandev/web dev --port $FRONTEND_PORT &
```

### Validate with playwright-cli

```bash
playwright-cli open http://localhost:$FRONTEND_PORT
playwright-cli snapshot                    # see page structure and element refs
playwright-cli click e5                    # interact using refs from snapshot
playwright-cli fill e3 "test input"
playwright-cli snapshot                    # verify result
```

### Fast iteration cycle

1. Make a code change in `apps/web/`
2. HMR picks it up in ~1-2 seconds
3. `playwright-cli snapshot` or `playwright-cli screenshot` to verify
4. Repeat until the flow works correctly

### Translate to E2E test

Once validated, write the Playwright test using project fixtures and page objects. The `playwright-cli` interactions map directly to Playwright API calls:

| playwright-cli | Playwright API |
|---|---|
| `playwright-cli click e5` | `page.getByTestId('...').click()` |
| `playwright-cli fill e3 "text"` | `page.getByTestId('...').fill('text')` |
| `playwright-cli snapshot` (verify element visible) | `expect(page.getByTestId('...')).toBeVisible()` |

Use `data-testid` selectors in the test (not snapshot refs), and wrap common flows in page objects.

### Capture PR evidence

After confirming the feature works, capture screenshots or a video as proof for the PR:

```bash
# Screenshots of key states
playwright-cli screenshot --filename=apps/web/.pr-assets/feature-before.png
# ... interact to show the feature ...
playwright-cli screenshot --filename=apps/web/.pr-assets/feature-after.png

# Or record a video walkthrough
playwright-cli video-start apps/web/.pr-assets/feature-demo.webm
# ... perform the user flow ...
playwright-cli video-stop
```

Create `apps/web/.pr-assets/manifest.json` so the `/pr` skill picks them up:
```json
{
  "assets": [
    {"name": "feature-demo", "file": "feature-demo.webm", "format": "gif", "caption": "Feature demo"},
    {"name": "feature-after", "file": "feature-after.png", "format": "png", "caption": "Result"}
  ]
}
```

### Final verification

Always verify against the production build before finishing — dev mode can hide boot-payload, asset-serving, or hydration issues:

```bash
playwright-cli close
# Kill dev server and backend
make build-web
cd apps && pnpm --filter @kandev/web e2e -- tests/path/to/test.spec.ts
```

## Test organization

Tests are grouped by feature area in subdirectories under `tests/`. When creating a new test:

- **Place it in the matching feature directory.** A test for PR detection goes in `pr/`, a test for session resume goes in `session/`, etc.
- **Merge related tests into the same file.** Tests covering the same feature (e.g., git commit body and pre-hooks) belong in one file with separate `test.describe` blocks. Don't create a new file for each narrow scenario.
- **Import paths from subdirectories** use `../../` (e.g., `from "../../fixtures/test-base"`).
- **Standalone root files** are allowed for truly cross-cutting tests that don't fit any group.
- **Extract large shared helpers.** For large specs with shared setup or polling helpers, extract helpers into a sibling `*-helpers.ts` file once the spec approaches the repo file-size limit. Keep spec files focused on test scenarios; put reusable page polling, seeding, and Dockview cleanup helpers in the helper module.

## Test quality guidelines

- **Test through the UI, not the API.** E2E tests verify user-facing behavior. Don't write tests that only call the API and assert the response -- those are integration tests. Instead, navigate to the page, interact with UI elements, and assert what the user sees.
- **Verify persistence with page reload.** After changing a setting or creating data, reload the page (`testPage.reload()`) and assert the state is still correct. This catches hydration bugs and Go boot-payload/client-store mismatches.
- **Restore patched persisted settings.** When a test PATCHes user settings, capture the baseline and restore it in `test.afterEach`. The backend is worker-scoped, and `e2eReset` does not reset every persisted setting, including `system_metrics_display`; leaking one can affect later tests in the same worker.
- **Nested Escape controls.** If an inner panel inside a Radix Dialog handles Escape, intercept the key in capture phase and call both `preventDefault()` and `stopPropagation()` before dismissing the inner panel. A bubble-phase window handler runs after Radix can dismiss the outer dialog. Add a regression that asserts the inner panel collapses while the outer dialog remains open.
- **Seed via API, assert via UI.** Use `apiClient` to set up preconditions quickly, but always verify the result by opening the page and checking the DOM.
- **Workflow/session invariants.** For session-primary/profile behavior, prefer polling backend state with `apiClient.listTaskSessions(taskId)` for invariants such as `agent_profile_id`, `is_primary`, `state`, and session count, then add UI assertions as secondary evidence. UI tab markers can lag or be absent when the backend invariant is the behavior under test.
- **Scope terminal helpers to the active panel.** Terminal/mobile helpers must avoid document-wide `.xterm` or `terminal-xterm-host` selectors because multiple terminal panels can be mounted at once. Scope locators through `data-testid="terminal-panel"` and prefer the visible or latest panel for `page.evaluate` helpers.
- **Scope Dockview preview polling to visible panels.** Hidden or stale Dockview panels can remain mounted and produce false positives if helpers scan all matching custom elements globally. For `diffs-container`, filter candidate elements by visible layout box and computed visibility before reading shadow DOM text.
- **Poll before Dockview cleanup.** If an E2E helper uses `window.__dockviewApi__`, wait or poll for the API to be attached before acting. A one-shot `if (!api) return` cleanup can silently skip cleanup during page initialization and leak prior preview panels into later assertions.

## Debugging failures

### Triage

When a test fails:

1. **Read the error output** — the Playwright error message, expected vs. actual, and which locator timed out
2. **Read `error-context.md`** from `test-results/<test-name>/` — contains a YAML DOM snapshot showing exactly what was rendered. Search for expected elements, check if the page is in the right state (e.g., simple mode vs advanced mode). **These files persist across runs** — always confirm timestamps (portable: `ls -la e2e/test-results/.../error-context.md`; or `stat -c %y` on Linux / `stat -f %Sm` on macOS) or rebuild + rerun the spec fresh before trusting the snapshot. A stale context from a previous failure mode will send you debugging the wrong bug.
3. **Read the failure screenshot** from `e2e/test-results/` — see what the page actually rendered
4. **Attach to the failure** for deeper debugging using `playwright-cli`:
   ```bash
   cd apps && PLAYWRIGHT_HTML_OPEN=never pnpm --filter @kandev/web e2e -- tests/path.spec.ts --debug=cli &
   # Wait for "Debugging Instructions" with session name
   playwright-cli attach tw-<session>
   playwright-cli snapshot    # inspect page state at failure point
   playwright-cli console     # check for JS errors
   playwright-cli network     # check API responses
   ```

### Classify and fix

| Category | Signals | Fast loop |
|---|---|---|
| **Test logic** | Wrong selector, wrong expected text, missing page object method | Fix test files, re-run immediately (no rebuild -- Playwright transpiles TS at runtime) |
| **Frontend-only** | Screenshot shows wrong UI, missing element, client error. API calls succeed. | Start dev server, fix with hot reload, verify with `playwright-cli`, then `make build-web` + re-run test |
| **Backend** | 500 errors, wrong API response, "Backend did not become healthy" | Fix Go code, `make build-backend`, re-run test |

### Common issues

- **"Backend did not become healthy"** — run `make build-backend build-web`, check with `E2E_DEBUG=1`
- **"Cannot find module"** — run `cd apps && pnpm install`
- **Port conflicts** — backends use 18080+ and frontends use 13000+ (per worker), auto-offset by `E2E_PORT_OFFSET` (derived from PID). Set `E2E_PORT_OFFSET=0` for deterministic ports
- **Auto-started session never goes idle** — for sessions started by the same call that creates them, the mock agent can finish before the client WS subscription registers, so a raw `idleInput()` visibility wait hangs. Use `SessionPage.waitForChatIdle()` instead; it reloads once and re-derives state from the Go boot payload.
- **Flaky timeouts** — **never increase locator timeouts to fix flaky tests.** If a locator times out, the root cause is almost always something else: a setup failure, missing navigation, race condition, or the element genuinely not rendering. Investigate why the element never appears instead of giving it more time. Note: infrastructure health timeouts (30s in `fixtures/backend.ts`) and overall test timeouts (60s in `playwright.config.ts`) are separate and should not be modified either.
- Screenshots on failure, video on first retry (CI)

### Debugging CI shard failures

CI splits tests across 10 shards. To reproduce a specific shard locally:

```bash
# List which tests are in a shard
npx playwright test --config e2e/playwright.config.ts --shard=2/10 --list

# Run that shard locally (requires production build)
make build-backend build-web
cd apps/web && npx playwright test --config e2e/playwright.config.ts --shard=2/10
```

E2E tests run against the **production Vite build served by the Go backend**, not dev mode. Always rebuild with `make build-web` (or `pnpm --filter @kandev/web build`) after code changes before running E2E tests locally.

```bash
# Unzip a shard's blob report from CI artifacts
unzip report-*.zip -d report-shard && cat report-shard/*.jsonl
```

When a CI shard fails, download its report-*.zip artifact and unzip it; the report is a *.jsonl event stream. Build a testId map by walking the events: test titles and locations come from the testBegin events, and final status plus duration come from the testEnd events. Match them by test id. This surfaces the slow but passing specs (the timing markers in Playwright output) that never show up as outright failures but are latent flake risks. Specs whose duration approaches the 60s per-test timeout (defined in playwright.config.ts) are the flake candidates to harden. Typically by converting raw chat-flow assertions to the waitForChatIdle() / expectChatResponseVisible() recovery helpers documented earlier in this file.

### Flake triage: intrinsic race vs. contention

A test that flakes under parallel/sharded load is one of two things — decide which **before** touching it:

1. **Re-run it in a fresh, isolated container** (or at minimum a single fresh worker), `--retries=0`, a few reps:
   ```bash
   pnpm e2e:docker --no-build -- --repeat-each=4 --workers=1 --retries=0 tests/path.spec.ts:LINE
   # or raw: pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium --repeat-each=4 --workers=1 --retries=0 tests/path.spec.ts:LINE
   ```
   (On Apple Silicon, `pnpm e2e:docker` needs Colima + Rosetta — `colima start --vz-rosetta`; default QEMU segfaults the amd64 Go build. See `apps/web/e2e/README.md`.)
   - **Flakes alone (fails some reps, fast):** intrinsic race — fix it (condition-correct wait, fix the actual race; not a timeout bump). E.g. a `waitForRequest` that times out the full window means the request *never fired* (a click swallowed during hydration) — retry the action with `await expect(async () => { ... }).toPass()`, don't extend the timeout.
   - **Passes clean AND fast alone (well under timeout):** contention, not a defect. The wait is correct; the test just starved for CPU/IO under load. No code/test fix applies.
2. **Signature of contention, not a code path:** two identical-config full runs giving *different* hard-fail counts (e.g. 0 vs 3). Same code + same config + different outcome ⇒ host oversubscription, not a bug. CI's isolated runners don't reproduce it; reduce local concurrency (2–3 shards, not 5+) for a clean signal.
3. **Caveat — don't flake-hunt with `--repeat-each` across many heavy specs in one long-lived worker.** It exhausts per-worker resources (agentctl port range, memory) over a long run and manufactures *false* failures unrelated to the test. Use **one fresh container per spec** instead.

## Selector guidelines

- **Prefer `data-testid` selectors** over text-based locators. Text content can change when UI is updated (e.g., hiding a badge), breaking tests that match by text. Use `getByTestId()` or `locator("[data-testid='...']")` for stable targeting.
- **Scope Radix tooltip locators to the visible portal.** Radix can render an
  accessibility copy as well as the visible tooltip, so global test-id, text,
  or role locators may match multiple elements in strict mode. Start from a
  open portal `[data-slot="tooltip-content"][data-state="open"]`, then locate its visible
  descendant. Do not assume the trigger's `aria-describedby` target is the
  visual portal. Use bounding-box assertions when relative placement matters.
- **Use page object methods** like `clickSessionChatTab()` (stable `data-testid`) instead of `sessionTabByText("1")` (fragile text match) for session tabs.
- **Dropdown menus can detach** from the DOM when React re-renders the parent (e.g., WS events updating the sidebar). The `openSidebarMenuAndClick()` helper in `session-page.ts` retries the full open-click sequence on detachment — use this pattern for similar interactions.

## TDD workflow

Follow `/tdd` when writing E2E tests:

1. **RED** — Write the spec, run it, watch it fail (missing `data-testid`, feature not implemented, etc.)
2. **GREEN** — Implement the feature/fix, add `data-testid` attributes, run the test until green
3. **REFACTOR** — Extract page objects, clean up selectors, keep tests green
4. Run the targeted E2E spec when done and report that full verification is
   required as a separate planner assignment
