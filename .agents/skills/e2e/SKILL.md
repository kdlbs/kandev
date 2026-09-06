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
pnpm e2e:run --no-build -- --grep "task creation"  # skip rebuild; forward flags after --
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

`--no-build` reuses every production E2E artifact, not only Vite assets and the
backend executable. On a fresh worktree or after rebasing, confirm packaged
fixtures also exist; global setup currently requires
`apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz`. If it is absent, run
without `--no-build` or first run `make -C apps/backend e2e-plugin-package`.
Prefer a normal managed build after source or base-branch changes and reserve
`--no-build` for repeated runs against unchanged artifacts.

For a raw Docker/SSH/container run, `make build-backend` alone does not build
the Linux mock-agent fixture. Prefer the managed runner; otherwise run
`make build-backend build-backend-remote-helpers build-web`. If the fixture
reports a missing `KANDEV_MOCK_AGENT_LINUX_BINARY`, run
`make -C apps/backend build-mock-agent-linux` before diagnosing product code.

### Raw commands (when you need fine control)

```bash
make test-e2e                                                      # all tests, headless (host)
cd apps && pnpm --filter @kandev/web e2e:raw -- tests/task/my-test.spec.ts  # single file
cd apps && pnpm --filter @kandev/web e2e:raw -- --grep "task creation" # by name
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
     bash -lc 'git config --global --add safe.directory /work 2>/dev/null; bash e2e/scripts/run-raw-e2e.sh --project=chromium --project=mobile-chrome --shard=<failed-shard>/14 -- --reporter=list --retries=0'
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
     bash -lc 'git config --global --add safe.directory /work 2>/dev/null; bash e2e/scripts/run-raw-e2e.sh --project=mobile-chrome e2e/tests/terminal/mobile-terminal-keybar.spec.ts --grep "user presses an OS-keyboard letter while no modifier is active" --repeat-each=30 --reporter=list --retries=0'
   ```
3. Preserve nearby test ordering when a single-test repeat stays green; run the full spec or shard with the same resource limits before declaring a flake non-reproducible.

The config uses `failOnFlakyTests: !CI`: local runs fail on a flaky retry, while
CI temporarily tolerates one. Either result is a failure signal for agents:
reproduce it with `--retries=0` only after disabling any `test.describe.configure({ retries: 1 })` override; never
rerun until it happens to pass. If isolated repeats stay green but the shard
fails, binary-search preceding specs in one worker. The fix is complete only
when the smallest reproducing sequence passes without retries.

Record the exact command, resource limits, repeat number, and failure artifact path. For a failed shard, inspect every `error-context.md` in its downloaded
`test-results-<shard>` artifact and compare shared page-object waits with `main`
before changing product code; the context can expose duplicate active terminals
or a terminal stuck on "Starting terminal...".

When a PR E2E shard fails, investigate every spec, including those outside the
diff; never dismiss, rerun, or merely record a failure as unrelated. Reproduce
the exact test, then preserve shard ordering and CI pressure. Fix every valid
defect with retries disabled, or evidence a concrete external blocker.

**CRITICAL: E2E tests run against the production Vite build served by the Go backend**, not dev mode. After any frontend code change, you **must** rebuild before running tests (`pnpm e2e:run` does this for you):

```bash
make build-web   # ~30s, required after every frontend change
```

Without this, tests can exercise stale code: after backend changes run `make -C apps/backend build` before reproducing Playwright failures, and compare the binary timestamp/hash if a fixed test still fails. `make test-e2e` and `pnpm e2e:run` handle both builds.

## Writing a test

1. Read `helpers/api-client.ts` and `pages/` to discover available seed methods and page objects; use `data-testid` attributes for selectors — add them to components as needed
2. Import fixtures from `../../fixtures/test-base` — provides `testPage`, `apiClient`, and `seedData` (pre-created workspace with default workflow). Pull `backend` from the fixture too when you need the backend URL — it's worker-scoped, dynamic, and `process.env.KANDEV_API_BASE_URL` is **not** set in the Playwright runner. Use `backend.baseUrl`.
3. Use page objects for common interactions; create new ones for new pages. For GitHub features, use `apiClient.mockGitHub*()` methods to seed mock data

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

Run the assertion in the relevant desktop and mobile projects when responsive
layout can change the result. Do not rely on fixed pixels when the product
contract is equality or alignment.

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

Before writing an E2E test, validate the feature works interactively using
`pnpm --dir apps exec playwright-cli` against a dev server. This gives a fast
feedback loop — code changes are picked up by hot reload in ~1-2 seconds, no
production rebuild needed. Once confirmed working, translate the interactions
into a proper E2E test.

### Start the dev environment

Multiple agents may run in parallel, so use random ports for dev servers. Managed runner shards are isolated, but separate raw Playwright processes are not automatically safe: fixture ports are backend `18080 + E2E_PORT_OFFSET + workerIndex` and agentctl `30001 + E2E_PORT_OFFSET*1000 + workerIndex*200`; `--repeat-each` advances `workerIndex`, so nearby fixed offsets can overlap later.

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
KANDEV_API_BASE_URL=http://localhost:$BACKEND_PORT VITE_KANDEV_API_PORT=$BACKEND_PORT \
pnpm --filter @kandev/web dev --port $FRONTEND_PORT &
```

### Validate with playwright-cli

```bash
pnpm --dir apps exec playwright-cli open http://localhost:$FRONTEND_PORT
pnpm --dir apps exec playwright-cli snapshot                    # see page structure and element refs
pnpm --dir apps exec playwright-cli click e5                    # interact using refs from snapshot
pnpm --dir apps exec playwright-cli fill e3 "test input"
pnpm --dir apps exec playwright-cli snapshot                    # verify result
```

### Fast iteration cycle

1. Make a code change in `apps/web/`
2. HMR picks it up in ~1-2 seconds
3. `pnpm --dir apps exec playwright-cli snapshot` or `pnpm --dir apps exec playwright-cli screenshot` to verify
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
pnpm --dir apps exec playwright-cli screenshot --filename=apps/web/.pr-assets/feature-before.png
# ... interact to show the feature ...
pnpm --dir apps exec playwright-cli screenshot --filename=apps/web/.pr-assets/feature-after.png

# Or record a video walkthrough
pnpm --dir apps exec playwright-cli video-start apps/web/.pr-assets/feature-demo.webm
# ... perform the user flow ...
pnpm --dir apps exec playwright-cli video-stop
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
pnpm --dir apps exec playwright-cli close
# Kill dev server and backend
make build-web
cd apps && pnpm --filter @kandev/web e2e:raw -- tests/path/to/test.spec.ts
```

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
  `e2eReset` do not reset every seeded record. A test that PATCHes a canonical
  `seedData` profile, repository, executor, or setting must capture its
  baseline and restore it in `test.afterEach` (so cleanup also runs after
  failure); prefer a disposable record when the UI can select it. Verify by
  running the mutating spec followed by its affected neighbour with
  `--workers=1 --retries=0`.
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

### Triage

When a test fails:

1. **Read the error output** — the Playwright error message, expected vs. actual, and which locator timed out
2. **Read `error-context.md`** from `test-results/<test-name>/` — contains a YAML DOM snapshot showing exactly what was rendered. Search for expected elements, check if the page is in the right state (e.g., simple mode vs advanced mode). **These files persist across runs** — always confirm timestamps (portable: `ls -la e2e/test-results/.../error-context.md`; or `stat -c %y` on Linux / `stat -f %Sm` on macOS) or rebuild + rerun the spec fresh before trusting the snapshot. A stale context from a previous failure mode will send you debugging the wrong bug.
3. **Read the failure screenshot** from `e2e/test-results/` — see what the page actually rendered
4. **Attach to the failure** for deeper debugging using `playwright-cli`:
   ```bash
   cd apps && PLAYWRIGHT_HTML_OPEN=never pnpm --filter @kandev/web e2e:raw -- tests/path.spec.ts --debug=cli &
   # Wait for "Debugging Instructions" with session name
   pnpm --dir apps exec playwright-cli attach tw-<session>
   pnpm --dir apps exec playwright-cli snapshot    # inspect page state at failure point
   pnpm --dir apps exec playwright-cli console     # check for JS errors
   pnpm --dir apps exec playwright-cli network     # check API responses
   ```

### Classify and fix

| Category | Signals | Fast loop |
|---|---|---|
| **Test logic** | Wrong selector, wrong expected text, missing page object method | Fix test files, re-run immediately (no rebuild -- Playwright transpiles TS at runtime) |
| **Frontend-only** | Screenshot shows wrong UI, missing element, client error. API calls succeed. | Start dev server, fix with hot reload, verify with `playwright-cli`, then `make build-web` + re-run test |
| **Backend** | 500 errors, wrong API response, "Backend did not become healthy" | Fix Go code, `make build-backend`, re-run test |

### Common issues

- **"Backend did not become healthy"** — run `make build-backend build-web`, check with `E2E_DEBUG=1`
- **"Cannot find module"** — run `cd apps && pnpm install --frozen-lockfile`
- **Port conflicts** — run raw repeat/stress invocations sequentially, or prove both computed port ranges are disjoint and check listeners. A totally white screenshot plus `ERR_CONNECTION_REFUSED` is a port/lifecycle signature to rule out before changing waits; never fix it with longer locator timeouts.
- **Responsive layout stays stale after `page.setViewportSize()`** — record
  `window.innerWidth`, the affected element and parent `clientWidth`, and any
  layout-library width before changing waits. Headless Chromium reliably
  resizes the DOM/container and fires `ResizeObserver`, while an
  application-only `window.resize` listener may not be observed in the test.
  Prefer synchronizing with the layout container/observer and assert the
  intended result after both viewport and container-only changes.
- **Auto-started session never goes idle** — for sessions started by the same call that creates them, the mock agent can finish before the client WS subscription registers, so a raw `idleInput()` visibility wait hangs. Use `SessionPage.waitForChatIdle()` before opening transient dialogs/drawers/popovers; it may reload and re-derive state from the Go boot payload. If it must run later, reopen the transient UI first. For WS/session hydration races, retain a bounded reload-and-retry fallback in the page object: keep the fast path immediate and the final check failing when genuinely stuck; remove it only with an equivalent deterministic readiness guarantee and focused regression.
- **Flaky timeouts** — **never increase locator timeouts to fix flaky tests.** If a locator times out, the root cause is almost always something else: a setup failure, missing navigation, race condition, or the element genuinely not rendering. Investigate why the element never appears instead of giving it more time. Note: infrastructure health timeouts (30s in `fixtures/backend.ts`) and overall test timeouts (60s in `playwright.config.ts`) are separate and should not be modified either.
- Screenshots on failure, video on first retry (CI)

### Debugging CI shard failures

CI splits host tests across 14 shards (plus 6 container shards); reproduce a specific shard locally:

```bash
# List which tests are in a shard
pnpm e2e:raw -- --shard=2/14 --list

# Run that shard locally (requires production build)
make build-backend build-web
cd apps/web && pnpm e2e:raw -- --shard=2/14
```

```bash
# Unzip a shard's blob report from CI artifacts
unzip report-*.zip -d report-shard && cat report-shard/*.jsonl
```

When a CI shard fails, distinguish its useful artifacts: download/unzip
`report-*.zip` to map test IDs and timings; `test-results-<shard>` may be ready
before the workflow completes even when logs refuse, so try it for the exact assertion, `error-context.md`, screenshot, and trace:

```bash
gh run download <run-id> --name test-results-<shard> --dir <temp-dir>
```

The blob report surfaces slow-but-passing specs that are latent flake risks;
the test-results artifact identifies the concrete failure to reproduce. Specs
whose duration approaches the 60s per-test timeout (defined in
`playwright.config.ts`) are candidates to harden, typically by converting raw
chat-flow assertions to the `waitForChatIdle()` / `expectChatResponseVisible()`
recovery helpers documented earlier in this file.

### Flake triage: intrinsic race vs. contention

A test that flakes under parallel/sharded load is one of two things — decide which **before** touching it:

1. **Re-run it in a fresh, isolated container** (or at minimum a single fresh worker), `--retries=0`, a few reps:
   ```bash
   pnpm e2e:docker --no-build -- --repeat-each=4 --workers=1 --retries=0 tests/path.spec.ts:LINE
   # or raw: pnpm e2e:raw -- --project=chromium --repeat-each=4 --retries=0 tests/path.spec.ts:LINE
   ```
   (On Apple Silicon, `pnpm e2e:docker` needs Colima + Rosetta — `colima start --vz-rosetta`; default QEMU segfaults the amd64 Go build. See `apps/web/e2e/README.md`.)
   - **Flakes alone (fails some reps, fast):** intrinsic race — fix it (condition-correct wait, fix the actual race; not a timeout bump). E.g. a `waitForRequest` that times out the full window means the request *never fired* (a click swallowed during hydration) — retry the action with `await expect(async () => { ... }).toPass()`, don't extend the timeout.
   - **Passes clean AND fast alone (well under timeout):** contention, not a defect. The wait is correct; the test just starved for CPU/IO under load. No code/test fix applies.
2. **Signature of contention, not a code path:** two identical-config full runs giving *different* hard-fail counts (e.g. 0 vs 3). Same code + same config + different outcome ⇒ host oversubscription, not a bug. CI's isolated runners don't reproduce it; reduce local concurrency (2–3 shards, not 5+) for a clean signal.
3. **Caveat — don't flake-hunt with `--repeat-each` across many heavy specs in one long-lived worker.** It exhausts per-worker resources (agentctl port range, memory) over a long run and manufactures *false* failures unrelated to the test. Use **one fresh container per spec** instead.

## Selector guidelines

- **Prefer `data-testid` selectors** over text-based locators. Text content can change when UI is updated (e.g., hiding a badge), breaking tests that match by text. Use `getByTestId()` or `locator("[data-testid='...']")` for stable targeting. When translated labels intentionally identify multiple routes, scope by stable `href` or a dedicated test ID rather than role/name alone.
- **Scope Radix and responsive locators to the active instance.** Tooltips may use `instant-open`, `delayed-open`, or `open`; use `[data-slot="tooltip-content"]:not([data-state="closed"])`, then scope to the visible portal/popover/container and active ancestor. Hidden mounts can make global locators match the wrong instance; do not use `.first()` to hide duplicates.
- **Use page object methods** like `clickSessionChatTab()` (stable `data-testid`) instead of `sessionTabByText("1")` (fragile text match) for session tabs.
- **Dropdown menus can detach** from the DOM when React re-renders the parent (e.g., WS events updating the sidebar). The `openSidebarMenuAndClick()` helper in `session-page.ts` retries the full open-click sequence on detachment — use this pattern for similar interactions.

## TDD workflow

Follow `/tdd` when writing E2E tests:

1. **RED** — Write the spec, run it, watch it fail (missing `data-testid`, feature not implemented, etc.)
2. **GREEN** — Implement the feature/fix, add `data-testid` attributes, run the test until green
3. **REFACTOR** — Extract page objects, clean up selectors, keep tests green
4. Run the targeted E2E spec when done and report that final change-aware
   verification is required as a separate planner assignment
