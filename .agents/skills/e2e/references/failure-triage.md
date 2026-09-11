# E2E failure triage

Load this reference for failing tests, CI shard reproduction, or suspected contention.
Read [resource-safety.md](resource-safety.md) before full or repeated runs.

### Flake reproduction

Start by matching CI as closely as possible. CI uses duration-aware manifests: replay the
matching manifest with `E2E_SHARD=<n> bash e2e/scripts/run-planned-shard.sh <manifest.json>`;
`--shard=N/14` is only approximate. Regenerate the manifest after source changes and never
overlap another managed/raw E2E run. Then add pressure deliberately:

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

Record the exact command, resource limits, repeat number, and failure artifact path. For an explicit no-flakes requirement, download `blob-report-*` with `gh run download <run-id> --pattern 'blob-report-*' --dir <tmp>` and run `python3 scripts/playwright-blob-audit <tmp>`; aggregate green is not flake-free. For a failed shard, inspect every `error-context.md` in its downloaded
`test-results-<shard>` artifact and compare shared page-object waits with `main`
before changing product code; the context can expose duplicate active terminals
or a terminal stuck on "Starting terminal...".

When a PR E2E shard fails, investigate every spec, including those outside the
diff; never dismiss, rerun, or merely record a failure as unrelated. Reproduce
the exact test, then preserve shard ordering and CI pressure. Fix every valid
defect with retries disabled, or evidence a concrete external blocker.

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
- **Initial hydration/readiness regressions:** do not use a page-object readiness helper that can reload or re-navigate the page (for example `SessionPage.waitForLoad`) to prove first-render behavior. Wait directly for the invariant locator on the current navigation, using a bounded non-reloading wait when needed. Keep reload-capable helpers for persistence and recovery scenarios.
- **Auto-started session never goes idle** — for sessions started by the same call that creates them, the mock agent can finish before the client WS subscription registers, so a raw `idleInput()` visibility wait hangs. Use `SessionPage.waitForChatIdle()` before opening transient dialogs/drawers/popovers; it may reload and re-derive state from the Go boot payload. If it must run later, reopen the transient UI first. For WS/session hydration races, retain a bounded reload-and-retry fallback in the page object: keep the fast path immediate and the final check failing when genuinely stuck; remove it only with an equivalent deterministic readiness guarantee and focused regression.
- **Editor readiness is not submit readiness** — a `contenteditable` may be present and writable while a session is still `STARTING`. Before submitting, wait for the scoped submit control to be enabled or for the exact session to reach `WAITING_FOR_INPUT`; use retries disabled when reproducing this boundary.
- **Flaky timeouts** — **never increase locator timeouts to fix flaky tests.** If a locator times out, the root cause is almost always something else: a setup failure, missing navigation, race condition, or the element genuinely not rendering. Investigate why the element never appears instead of giving it more time. Note: infrastructure health timeouts (30s in `fixtures/backend.ts`) and overall test timeouts (60s in `playwright.config.ts`) are separate and should not be modified either.
- Screenshots on failure, video on first retry (CI). In workflow cache steps, `actions/cache/restore` sets `cache-hit` to `true` for an exact key, `false` for a restore-key/prefix hit, and empty on a miss; gate verification/fallback on empty versus non-empty, smoke-test Chromium before skipping image extraction, and use bounded backoff for transient registry metadata probes such as `docker buildx imagetools inspect`.
- **Page-closed errors after timeouts** — `Target page, context or browser has been closed` can be teardown masking an earlier overlay interception. Inspect the preceding action and screenshot, for example with `rtk proxy unzip -p <trace.zip> 0-trace.trace | rtk proxy jq -c 'select(.type == "action" or .type == "error")'`, then close the overlay in the page object, assert it is closed, and rerun with retries disabled before changing timeouts or routes.

### Debugging CI shard failures

CI splits host tests across 14 shards (plus 6 container shards); reproduce a specific shard locally. Download `e2e-shard-manifests` when available, verify its selected file list matches the failed job, and prefer the CI runtime image:

```bash
# Run the exact duration-aware manifest-selected shard
E2E_SHARD=2 KANDEV_E2E_SKIP_FRESHNESS=1 bash e2e/scripts/run-planned-shard.sh <workspace>/e2e-manifests/normal/2.json
# If no manifest exists, inspect the ordinal fallback with `npx playwright test --config e2e/playwright.config.ts --shard=2/14 --list`, then run it with a production build
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

A failure under load can indicate a race, leaked fixture state, or resource contention.
Gather evidence before assigning a cause:

1. **Re-run it in a fresh, isolated container** (or at minimum a single fresh worker), `--retries=0`, a few reps:
   ```bash
   pnpm e2e:docker --no-build -- --repeat-each=4 --workers=1 --retries=0 tests/path.spec.ts:LINE
   # or raw: pnpm e2e:raw -- --project=chromium --repeat-each=4 --retries=0 tests/path.spec.ts:LINE
   ```
   (On Apple Silicon, `pnpm e2e:docker` needs Colima + Rosetta — `colima start --vz-rosetta`; default QEMU segfaults the amd64 Go build. See `apps/web/e2e/README.md`.)
   - **Flakes alone (fails some reps, fast):** intrinsic race — fix it (condition-correct wait, fix the actual race; not a timeout bump). E.g. a `waitForRequest` that times out the full window means the request *never fired* (a click swallowed during hydration) — retry the action with `await expect(async () => { ... }).toPass()`, don't extend the timeout.
   - **Passes clean and fast alone:** the cause remains unknown. Preserve shard order and resource limits to distinguish fixture leakage from contention.
2. **Different failure counts across identical runs are inconclusive.** Races can also produce different results. Record CPU, memory, and process pressure. Compare resource-bounded runs before attributing the failure to oversubscription.
3. **Caveat — don't flake-hunt with `--repeat-each` across many heavy specs in one long-lived worker.** It exhausts per-worker resources (agentctl port range, memory) over a long run and manufactures *false* failures unrelated to the test. Use **one fresh container per spec** instead.
