# E2E flake eradication

**Status:** draft · 2026-05-12 · cfl

## Goal

Drive the office Playwright suite to 100% pass rate over 3 consecutive
local runs on a single machine, with no `retries: 2` cushion required
in CI. Today's local rate is ~95% (272–278 / 285) and the failing set
rotates between runs — the hallmark of multiple distinct root causes,
not a single missing wait.

Why this matters:

- CI flakes burn maintainer attention and erode trust in green builds.
  Every retried-pass hides a class of race that will eventually fail
  for real (a slow customer machine, a network blip, a Linux kernel
  scheduling jitter under load).
- Per-spec timeout bumps and polling deadlines have hit diminishing
  returns. The remaining flakes have structural causes that need
  structural fixes.

## Why this is not "one fix"

Six investigation passes plus three delegated agent runs have now
sorted the flake population into **eight distinct root-cause classes**.
Each class needs its own targeted change. They are NOT the same race
with different symptoms; the per-class evidence below is what proves
that.

This spec is a plan: one item per class, sequenced by impact ÷ effort.
Land them in order and the suite should converge on 100%.

---

## Class A — Routing-spec `KANDEV_MOCK_PROVIDERS` env-handling bug

**Failure shape:**
- `office-routing-fallback` / `disabled` / `agent-override`: PUT
  `/workspaces/:ws/routing` returns `400: order has 2 entries, max 0
  allowed`.
- `office-routing-fallback` / `recovery`: PATCH
  `/office/tasks/:id` returns `500: FOREIGN KEY constraint failed`.

**Root cause:**

`registry.RoutableProviderIDs` is empty when these tests fire even
though the backend spawned with `KANDEV_MOCK_PROVIDERS="claude-acp,
codex-acp,opencode-acp"`. The validator at
`internal/office/routing/types.go:344` reads the catalogue from
`registry.RoutableProviderIDs`; if that's empty, every non-empty
`provider_order` is rejected.

The FK violation is the same bug rotated: the test fixture's
`officeSeed.agentId` references an agent profile that didn't survive
the backend restart-with-mock-providers cycle (or was never written
during the post-restart re-seed).

**Evidence this is a real bug, not a race:**

- The failure mode is deterministic per-run when it happens (no
  amount of polling recovers — the catalogue is empty, period).
- It rotates between specs in the routing project because the FIRST
  test of that project to call `backend.restart({...PROVIDERS})` hits
  the bug; siblings inherit the broken state.
- Manual repro: spawn backend with `KANDEV_MOCK_PROVIDERS=...`, GET
  `/api/v1/agents` should list mock-agent under each canonical
  provider ID. Today it sometimes doesn't.

**Fix:**

1. Trace `KANDEV_MOCK_PROVIDERS` from env-var → `discovery.Detect` →
   `agentRegistry.Register` (or equivalent). Find the path that's
   conditional and breaks.
2. Most likely candidate: the registry-mutation code path runs
   *before* the env var is read, or vice versa. The fix is a
   re-ordering or a synchronous wait.
3. Add a startup-time invariant: if `KANDEV_MOCK_PROVIDERS` is set
   and `len(RoutableProviderIDs) < N+1` (where N is the comma count),
   log a hard error and `return false` (matching the
   `runInitialAgentSetup` fatal pattern landed in `fd7593bf`).
4. Add a Go unit test in `internal/agent/registry/` that exercises
   `RegisterCanonical(claude-acp)` and asserts visibility through
   `RoutableProviderIDs`.

**Effort:** 1–2 hours (the trace is bounded; the fix is small).

**Impact:** Eliminates 1–3 flakes per run. Routing project goes
from 30–60% pass to 100%.

---

## Class B — Frontend SSR/store hydration race after create

**Failure shape:**
- `agent-roles > UISecurityBot span not visible` (20s)
- `runtime-agent-creation > agent-topbar-name not visible` (30s)
- `org-chart > CEO node not visible`
- `sidebar-navigation > CEO link not visible` (10s)

**Root cause:**

Test creates an entity via API → `testPage.goto(...)` → Next.js SSR
fetches the list → hydrates the Zustand store from `initialAgents` →
page renders.

If the agent landed in the DB AFTER the SSR fetch fired, the page
hydrates with the stale list and there is **no client-side refetch**
to recover. The store is the source of truth from that point on. The
new agent never appears even though the DB has it.

Specifically — `apps/web/app/office/agents/page.tsx` SSRs
`listAgentProfiles(workspaceId)`. The matching client
(`agents-page-client.tsx`) only calls `setOfficeAgentProfiles` from
the SSR `initialAgents` prop in a `useEffect`; it does NOT refetch
on mount or visibility change.

**Evidence this is a hydration race, not a backend race:**

- Adding `expect.poll(listAgents → includes new agent)` before
  navigation (which I landed for `agent-roles`) reduced this flake
  for that one spec but didn't eliminate it. Even when the list
  endpoint definitely has the agent, SSR can still fetch a stale
  snapshot under load. The agent's apiClient hit a different SQLite
  connection / page cache layer than SSR's listAgentProfiles.
- The component is structurally vulnerable: there's no recovery path
  for "store was hydrated with stale data".

**Fix:**

A. **Defensive client refetch on mount** — each list page that owns
   a `useEffect(() => setX(initialX), [])` gains a follow-up
   `useEffect(refetchOnce, [])` that re-hits the same endpoint and
   replaces the store. Cheap. Adds one HTTP call per page load.
B. **OR: WS-driven invalidation** — `useOfficeRefetch("agents")`
   already exists for this. Wire it into every list page that
   currently SSRs. The WS layer publishes `office.agent.created` /
   `.updated` / `.deleted`; the hook subscribes and refetches.
   Preferred long-term.

**Effort:** 4–6 hours (need to audit every list page in
`apps/web/app/office/` and add the hook).

**Impact:** Eliminates 2–4 flakes per run. Largest single chunk.

---

## Class C — Sporadic frontend `fetch failed` errors

**Failure shape:**
- `task-chat > sidebar shows CEO agent` / `sidebar shows tasks link`:
  `TypeError: fetch failed [cause]: AggregateError`
- `advanced-mode > dockview layout renders`: same shape.
- `dashboard-ui > dashboard shows agent count metric`:
  `ERR_CONNECTION_REFUSED`.

**Root cause:**

Frontend (Next.js standalone) makes server-side fetches to the
backend during SSR. Under saturating load:

- Sometimes the TCP connection is reset mid-handshake (ENOENT /
  ECONNRESET).
- Sometimes the backend is mid-restart (routing project specs) and
  the SSR fetch hits the listener-disposed window between the old
  process exiting and the new process binding.
- Sometimes the Next.js standalone server itself has crashed (we
  see `connection refused` on the FRONTEND port, not backend).

**Evidence:**

- Failures cluster in time — multiple specs in a row hit `fetch
  failed`, then it clears.
- Always under heavy load (full-suite runs); never reproduces in
  isolated single-spec runs.

**Fix:**

A. **Backend graceful-shutdown wait** in `backend.restart()`: today
   it `killProcessGroup` + 2s sleep. Bump to wait for the old port
   to close (`waitForPortFree`) before spawning the new process.
B. **Frontend retry-on-SSR-error**: wrap `listAgentProfiles` /
   `listWorkspaces` / etc in a small retry layer (3 attempts, 100ms
   backoff). All these calls are idempotent reads.
C. **Per-route Next.js error boundary** that recovers from SSR
   transient failures by falling back to a client-side fetch instead
   of a 500 page. Already exists for some routes; needs auditing.

**Effort:** 3–4 hours.

**Impact:** Eliminates 1–2 flakes per run.

---

## Class D — Single-machine resource saturation

**Failure shape:**
- Multiple specs go slow simultaneously: `property-pickers`
  hits its 60s test timeout (despite the assertion timeouts being
  10s individually).
- `agent-dashboard > latest-run-card`: 5s testid timeout, fast on
  isolated rerun, slow only under full-suite load.
- Generic "test timeout of N exceeded" with no specific assertion
  failure.

**Root cause:**

Single worker runs 285 specs in series for ~6 minutes. Each test
chains: API call → page navigation → SSR + hydration → assertion +
network idle. On a machine doing other work (compilation, indexing,
Spotlight), individual assertions get jittered into >5s territory
even though the median is <500ms.

**Evidence:**

- Pass rate on a freshly-restarted machine with no other workload
  is higher than on an in-use dev machine.
- CI passes more often than local because CI shards across runners
  (multi-machine parallelism).
- `property-pickers` consistently hits the test timeout
  across multiple runs — not at the same individual assertion, just
  somewhere in the test.

**Fix:**

A. **Enable workers ≥ 2 locally** — backend.ts already allocates
   per-worker ports; the missing piece is per-worker frontend
   build/copy. Each worker runs ~140 specs instead of 285. Per-test
   resource contention drops proportionally.
B. **OR: relax timeouts globally** — bump the playwright config
   `timeout: 60_000` to 120s, and per-test default `expect.timeout`
   from 5s to 15s. Brute-force but reliable.
C. **OR: shard locally** — `playwright test --shard=1/2` then
   `--shard=2/2` in two terminals. Manual but works today.

**Effort:** workers ≥ 2 is the right answer; ~6–8 hours to
audit/fix per-worker isolation (separate frontend `.next/standalone`
copies, separate worktree basepaths). Already partially done.

**Impact:** Eliminates 1–2 flakes per run.

---

## Class E — `runInitialAgentSetup` discovery walk under contention

**Failure shape:**
- `seedData fixture timeout after 30s of polling, listAgents
  returned 0 agents`.

**Root cause:**

`EnsureInitialAgentProfiles` runs `c.discovery.Detect(ctx)` which
walks the filesystem for installed agent CLIs. The walk is
synchronous and unbounded. Under heavy load (parallel CI workers,
SSD contention, antivirus), the walk can take >30s. Backend
processes startup but never finishes seeding agents within the e2e
fixture's deadline.

The fatal promotion (`fd7593bf`) catches this loudly now, but the
underlying defect — unbounded filesystem scan during startup — is
unchanged.

**Evidence:**

- Failure ONLY happens on full-suite runs, never isolated.
- When it happens, backend log shows `Failed to run initial agent
  setup` after multi-second discovery latency.

**Fix:**

A. **Bound the discovery walk** — apply a context with `WithTimeout(5s)`
   to `discovery.Detect`. Partial discovery is better than no
   discovery; mock-agent is force-marked available anyway.
B. **Skip filesystem discovery in mock mode** — when
   `KANDEV_MOCK_AGENT=true` and `KANDEV_E2E_MOCK=true`, skip
   `discovery.Detect` entirely; mock-agent is the only agent we
   need anyway. This is the surgical e2e-mode fix.

**Effort:** 1 hour for option B; 2–3 for option A.

**Impact:** Eliminates the rare 30s-timeout class. Also speeds up
e2e suite startup.

---

## Class F — Cross-project worker transition cold-boot

**Failure shape:**
- First test in chromium project right after routing project ends:
  `E2E mock harness not mounted: /api/v1/_test/health returned 404
  after 10s of polling`.

**Root cause:**

Playwright disposes the routing worker → spawns chromium worker
fresh → new backend boots → routes mount → `/health` flips green →
fixture proceeds. The readiness gate from `ead2f643` ensures
`/health=200` means listener accepts, but `apiClient`'s probe of
`/api/v1/_test/health` still 404s sometimes.

Most likely cause: when the routing worker dies, the OS holds the
port in `TIME_WAIT` for 30–120s. The new worker fixture allocates
the SAME port (per parallelIndex). The new backend can't bind →
exits → fixture sees `connection refused` until retry.

**Evidence:**

- Failure ONLY at the routing → chromium transition.
- Adding `await new Promise(r => setTimeout(r, 5000))` between
  projects empirically reduces this flake.

**Fix:**

A. **`SO_REUSEADDR` on the backend listener** — Go's net.Listen
   already sets this on Linux; macOS may not. Verify.
B. **Per-project port allocation** — assign different port bases to
   different projects. Routing uses `Backend + 100`, chromium uses
   `Backend + 200`. No port collision possible.
C. **Wait for port free in `restart()`** — already does a 2s sleep;
   replace with explicit `waitForPortFree` loop.

**Effort:** 1–2 hours.

**Impact:** Eliminates the first-chromium-spec flake class.

---

## Class G — Test-fixture cross-pollination (user_settings drift)

**Failure shape:**
- `onboarding-task-launch > task created during onboarding does not
  fail to launch`: expected workspace_id mismatches actual.

**Root cause:**

`user_settings.workspace_id` is the active-workspace pointer. The
office fixture resets it to `officeSeed.workspaceId` in its testPage
hook. Specs that DON'T use office-fixture's testPage (using
test-base's directly) inherit whatever value a previous test wrote.

If a previous test ran `completeOnboarding`, that created a NEW
workspace and updated user_settings. The next test using base
test-base sees the wrong active workspace.

**Evidence:**

- The failing specs that hit this are specifically
  `onboarding-task-launch.spec.ts` and similar that use a
  custom-extended test-base, not office-fixture.
- The expected vs actual workspace IDs differ by a fixed prefix
  pattern matching prior-test workspace creations.

**Fix:**

A. **Global beforeEach in test-base** that calls
   `apiClient.saveUserSettings({ workspace_id: seedData.workspaceId })`
   so every test starts with a known active workspace.
B. **OR: audit + migrate** every office spec to use office-fixture.
   More work but eliminates the divergence.

**Effort:** 1 hour for A; 3–4 for B.

**Impact:** Eliminates 1 flake class.

---

## Class H — Per-spec assertion timeouts genuinely too tight

**Failure shape:**
- `agent-dashboard > latest-run-card`: 5s testid timeout.
- `live-presence > dashboard sidebar`: 10s.

**Root cause:**

Some assertions are tight because they were written for the happy
path and never re-evaluated against a fully-loaded suite. After all
the structural fixes above land, a few of these will still trip
because the user's machine is fundamentally slower than the dev's
when the test was written.

**Fix:**

Just bump the timeouts on the long-tail-slow assertions to 15–20s
once the structural fixes have made the average case fast.

**Effort:** 30 minutes per spec, ~5 specs left after structural
work.

**Impact:** Cleans up the long tail.

---

## Sequencing

In order of impact ÷ effort:

1. **Class A** (routing env bug) — 1-2h, kills 3 flakes/run, is a
   real backend defect not a flake.
2. **Class E** (discovery bound) — 1h, kills the worst-case timeout
   class.
3. **Class F** (port reuse) — 1-2h, kills the project transition flake.
4. **Class G** (user_settings drift) — 1h, kills onboarding flake.
5. **Class B** (hydration race) — 4-6h, kills the largest single
   chunk but takes the most careful work.
6. **Class C** (fetch failed retry) — 3-4h, the bulk of remaining
   "network" flakes.
7. **Class D** (workers ≥ 2) — 6-8h, the long-tail performance win.
8. **Class H** (timeout bumps) — 30m × 5 = 2.5h, last-mile cleanup.

Total: ~20–30 hours. Achievable in a focused 2-day push, or 3–4
sessions of background work.

## Verification criteria

After each class lands:
- Run the full office suite 3× back to back on the same machine.
- Track pass rate. Class is "done" when the flake it targets stops
  appearing in any of the 3 runs.

After ALL classes land:
- Run the full office suite 5× back to back. Required: 5/5 clean.
- Repeat on a second machine (CI or dev box) to confirm not
  machine-specific.

Optionally: drop `retries: 2` from `playwright.config.ts` in CI
once 5/5 local-clean is hit. That's the real proof.

## Out of scope

- Backend test flakiness (5464 Go tests, currently 100% — out of
  scope for this spec).
- Frontend unit tests (already deterministic).
- Cross-browser e2e (chromium-only by config; expanding to webkit
  /firefox is a separate effort).
- Replacing Playwright with another runner.

## Files touched (estimate, by class)

- **A**: `apps/backend/internal/agent/registry/`,
  `apps/backend/internal/agent/settings/controller/agent_discovery.go`
- **B**: every `apps/web/app/office/**/page.tsx` + matching
  `*-page-client.tsx` (~10 files)
- **C**: `apps/web/lib/api/*.ts` (retry wrapper),
  `apps/web/e2e/fixtures/backend.ts` (port-free wait)
- **D**: `apps/web/e2e/playwright.config.ts`,
  `apps/web/e2e/fixtures/backend.ts`
- **E**: `apps/backend/internal/agent/settings/controller/agent_discovery.go`,
  `apps/backend/cmd/kandev/initial_agent.go`
- **F**: `apps/web/e2e/fixtures/backend.ts`
- **G**: `apps/web/e2e/fixtures/test-base.ts`
- **H**: scattered spec files in `apps/web/e2e/tests/office/`
