# Kandev Web — E2E Test Suite

Playwright-based end-to-end tests. Each Playwright worker spawns its own real Go backend (no mocks of internal services) on isolated ports; that backend serves the Vite SPA assets and boot data while Playwright drives a real Chromium against it.

## Project layout

| Folder                 | What's in it                                                                                                                                 |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `tests/`               | Spec files, grouped by feature (`chat/`, `docker/`, `git/`, `integrations/`, `kanban/`, `pr/`, `search/`, `session/`, `ssh/`, `layout/`, …). |
| `fixtures/`            | Worker-scoped fixtures that own the backend lifecycle (`backend.ts`, `docker-test-base.ts`, `ssh-test-base.ts`, `test-base.ts`).             |
| `helpers/`             | Reusable building blocks for specs (`api-client.ts`, `docker.ts`, `ssh.ts`, `git-helper.ts`, `ws-capture.ts`, …).                            |
| `pages/`               | Page Objects (one class per top-level UI surface — `SessionPage`, `KanbanPage`, `JiraSettingsPage`, `SSHSettingsPage`, …).                   |
| `playwright.config.ts` | Project definitions, timeouts, sharding config.                                                                                              |
| `global-setup.ts`      | Pre-flight checks for required binaries (kandev, mock-agent) and the Vite web build.                                                         |

## Playwright projects

The suite is split into five projects. Pick one with `--project=<name>`.

### `routing`

Runs `office-routing-*.spec.ts` in an isolated desktop worker. Those specs restart
their backend with provider overrides that apply only to the restart/spec that
supplies them; the next restart rebuilds the environment from its clean baseline.
Keeping these specs separate also keeps their routing-specific provider and agent
fixtures away from tests that count agents or assert the active-agent label. Run
it directly with:

```sh
pnpm e2e:raw --project=routing
```

### `auth`

Runs `tests/auth/**` in an isolated desktop worker. These specs restart their
backend with authentication enabled, so `chromium` intentionally excludes them.
Select the project explicitly; otherwise Playwright reports `No tests found`:

```sh
pnpm e2e:raw --project=auth tests/auth/auth-lifecycle.spec.ts
```

### `chromium` (default)

The everyday surface — runs in every CI shard. Excludes the heavyweight `containers` specs and the mobile specs.

```sh
pnpm e2e:raw
```

### `mobile-chrome`

Same as `chromium` but on Playwright's Pixel-5 viewport, gated on `tests/**/mobile-*.spec.ts`. Runs in the same CI shard matrix as `chromium`.

### `containers` — **Docker required**

**This is the "real-infra heavyweight" project.** Despite the name, it covers **more than just the Docker executor** — it's where any test that needs Docker on the host as a runtime lives:

- **Docker executor tests** (`tests/docker/*.spec.ts`) — verify kandev launches real `kandev-agent:e2e` containers, recovers them across backend restarts, cleans them up on archive/delete, etc.
- **SSH executor tests** (`tests/ssh/*.spec.ts`) — verify kandev SSHes into a real `kandev-sshd:e2e` container, uploads agentctl, runs an agent end-to-end, recovers across backend restarts, etc. The SSH executor's _remote target_ is a Docker container in tests, even though the SSH connection itself is a real SSH connection.

This project:

- **Skips entirely** when no Docker daemon is reachable. Contributors without Docker can still run `chromium` + `mobile-chrome`.
- **Builds container images on demand.** First run builds `kandev-agent:e2e` (slim Node base + git) and `kandev-sshd:e2e` (Alpine + openssh-server + git + pre-baked mock-agent). Subsequent runs hit Docker's layer cache.
- **Has a longer per-test timeout** (180s vs 60s) because container starts + agent setup are slow.

How to run it locally (requires Docker running):

```sh
KANDEV_E2E_CONTAINERS=1 pnpm e2e:raw --project=containers
```

Or a single spec:

```sh
KANDEV_E2E_CONTAINERS=1 pnpm e2e:raw --project=containers tests/ssh/launch-task.spec.ts
```

### Remote-executor fixture contracts

Host-only `file://` fixtures are not reachable from an SSH or Docker target. Use a disposable provider-shaped HTTP Git fixture with a target-side URL rewrite; if the spec also uses host-local `GitHelper` or LSP paths, materialize a local clone at the expected temporary path. Verify both the remote checkout and every host-local fixture consumer.

### Why "containers" instead of "docker"?

This project used to be named `docker`. It was renamed to `containers` once SSH e2e tests joined it — calling it `docker` was misleading because SSH tests have nothing to do with the Docker _executor_; they just happen to use Docker as the runtime that hosts the sshd target.

`KANDEV_E2E_DOCKER=1` is still honored as a deprecated alias for `KANDEV_E2E_CONTAINERS=1` for one release. Local scripts and stale CI configs won't break, but new code should use the new name.

## Commands

`e2e:raw`, `e2e:run`, and `e2e:ui` are defined only in `apps/web/package.json`. Run them from `apps/web` (or `pnpm --filter @kandev/web e2e:run` from elsewhere). From the repo root you get `ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND`.

| Command                                | What it does                                     |
| -------------------------------------- | ------------------------------------------------ |
| `pnpm e2e:raw`                         | Run the default (chromium) project headless.     |
| `pnpm e2e:ui`                          | Open Playwright's UI mode for interactive runs.  |
| `pnpm e2e:headed`                      | Run headless project but with a visible browser. |
| `pnpm e2e:raw --project=containers`    | Run container-backed tests (needs Docker).       |
| `pnpm e2e:raw --project=mobile-chrome` | Run mobile responsive tests.                     |
| `pnpm e2e:raw --project=routing`       | Run provider-mutating Office routing tests.      |
| `pnpm e2e:raw --project=auth`          | Run auth-isolated tests.                         |
| `E2E_DEBUG=1 pnpm e2e:raw`             | Surface Docker build output + extra logging.     |

Common flags: `--shard=1/4`, `-g "fragment of test name"`, `--repeat-each=3` (flake hunting).

### Duration-aware CI sharding

CI creates an ephemeral manifest for each cohort from the current test catalog.
The normal cohort has 14 shards and runs `chromium` plus `mobile-chrome`; the
container cohort has 6 shards and runs `containers`. The manifest assigns
project/file units with a deterministic longest-processing-time planner. Matrix
jobs pass the assigned files to `run-planned-shard.sh`; they do not use ordinal
Playwright `--shard` selection.

Successful `main` runs publish `e2e-timing-profile`, which stores bounded
first-attempt passing samples keyed by project, file, and full test title. The
next planning job downloads that artifact when available. New files use the
count fallback, and changed files are marked warm and receive the conservative
multiplier. If the artifact is missing, the planner records
`profile.mode=count-fallback` and still validates complete catalog coverage.

The runner validates the manifest against the checkout before launching
Playwright. A missing, stale, overlapping, or incomplete manifest is a hard
failure; it cannot silently fall back to ordinal sharding.

To dry-run the planner locally:

```bash
cd apps
pnpm exec tsx web/e2e/scripts/plan-shards.ts \
  --web-root "$PWD/web" \
  --output-dir "$PWD/web/e2e/manifests"
```

To execute one generated shard locally, set `E2E_SHARD` and pass its manifest:

```bash
cd apps/web
E2E_SHARD=1 bash e2e/scripts/run-planned-shard.sh e2e/manifests/normal/1.json
```

The report job publishes `e2e-retry-summary` and `e2e-timing-diagnostics`.
The summary includes first-attempt passes, passed-after-retry tests, final
failures, timeouts, skipped tests, predicted/actual shard duration deltas, and
unknown/warm/stale/fallback planning counters. The profile candidate is uploaded on
every report, but only a successful `main` workflow run is eligible to seed a
future plan. Manifests are retained for 3 days; timing profiles for 30 days;
retry diagnostics for 14 days.

Dispatch the workflow with `fail_on_flaky=true` to set
`failOnFlakyTests: true` for a diagnostic run. Normal PR runs retain the
existing two-retry policy while the summary makes retry groups visible.

### Flake rate and trend

CI retries hide flakes: with `retries: 2` and `failOnFlakyTests: false`, a test
that fails and then passes never fails the build. The **E2E flake rate** section
of the `e2e-report` job summary makes that number visible without downloading
anything. It reports, for the run:

- the flake count, the number of executed tests, and the rate per 1000;
- the baseline (median of the last 10 recorded runs) and the change against it;
- every flaky test by name, with its attempt statuses and how many of the last
  10 recorded runs it flaked in;
- the trend table and the repeat offenders across that window.

A "flaky" test is Playwright's own verdict, not "passed after a retry":
`computeOutcome` in `e2e/scripts/retry-summary.ts` mirrors Playwright's
`computeTestCaseOutcome`, so an interrupted-then-passed test and a `test.fail()`
test are classified the way Playwright classifies them. The report cross-checks
its count against `stats.flaky` in the merged Playwright JSON report for the
same run and prints **MISMATCH** plus a workflow warning if the two disagree.

The cross-check reports its verdict in the workflow log on every run, not only
on a mismatch, and annotates a skipped check as a warning. If only a mismatch
spoke, a cross-check that had silently stopped running would be indistinguishable
in the log from one that passed, which is the same shape of bug as a flake that
never fails the build.

The trend is carried between runs as the `e2e-flake-history` artifact (90 days,
capped at 50 entries, newest first). `e2e-report` looks for the newest completed
`main` run that published one, uses it as the baseline, appends this run's entry,
and re-uploads it. There is no external service; a missing or unreadable baseline
just makes the run seed a fresh trend.

To render the report locally against a blob report directory:

```bash
cd apps/web
pnpm exec tsx e2e/scripts/retry-summary.ts --input <blob-dir> --output /tmp/retry-summary.json
pnpm exec tsx e2e/scripts/flake-report.ts --summary /tmp/retry-summary.json
```

### `pnpm e2e:run` — the managed runner (build + run + teardown)

`e2e/scripts/run-e2e.sh` (aliased as `pnpm e2e:run`) handles the build, the run, and cleanup so you don't have to assemble the steps by hand. It **auto-selects docker vs host**, runs **N shards concurrently**, enforces strict WS accounting by default (`KANDEV_E2E_WS_ASSERT=1`, matching CI), and never leaves root-owned artifacts behind.

```bash
pnpm e2e:run                                  # auto: docker if the daemon + CI image are available, else host
pnpm e2e:run --shards 3                        # 3 shards concurrently (isolated containers, or host procs with distinct ports)
pnpm e2e:run tests/chat/foo.spec.ts            # extra args pass straight through to Playwright
pnpm e2e:run --project auth tests/auth/auth-lifecycle.spec.ts
pnpm e2e:run --host --no-build -- --grep "x"   # force host, skip rebuild, forward flags after --
pnpm e2e:docker                                # force the docker CI image (full isolation from a host dev instance)
pnpm e2e:clean                                 # remove build/test artifacts, incl. root-owned ones from prior docker runs
```

Why a script instead of raw `docker run`: in docker mode it builds the CGO/`fts5` backend on the **host** and runs it in the runtime image — a host glibc that's the same or older than the image's (the usual case; the image tracks recent Ubuntu) is forward-compatible, so no build image is needed. The runner smoke-tests the binary in the image first and only falls back to the build image (`KANDEV_CI_BUILD_IMAGE`, default `…/kandev-ci:build-latest`) if your host glibc is _newer_. It also builds the Vite web assets on the host, points Playwright output at a container-local dir, and cleans up. Run `clean` if a previous bare `docker run` left root-owned files you can't delete.

> **Apple Silicon:** the docker path runs the amd64 CI image. Under Docker Desktop's default QEMU emulation the amd64 Go toolchain segfaults (`SIGSEGV` in `modindex.dirHash`) during backend build. Use Colima with Rosetta instead: `colima start --vm-type=vz --vz-rosetta`. QEMU is not viable for the Go build; Rosetta is required for local amd64 E2E repro on arm64.

> **Office is always enabled in e2e (and dev); only prod gates it off.** `profiles.yaml` sets `KANDEV_FEATURES_OFFICE` to `"true"` in the `e2e` and `dev` profiles and `"false"` in `prod`, and the fixture selects the e2e profile via `KANDEV_E2E_MOCK=true`. So `tests/office/*` always have office routes registered — no manual env setup.
>
> This used to break when e2e was launched from a shell that had inherited `KANDEV_FEATURES_OFFICE=false` (e.g. from a host kandev backend running the prod profile): `profiles.ApplyProfile` only sets vars that are **unset** (so launchers/shells win — see `docs/decisions/0007-runtime-feature-flags.md`), and the fixture spreads `process.env` into the spawned backend, so the stale prod value won and 404'd every office spec. Fixed at the source: `sanitizeInheritedEnv` in `e2e/fixtures/backend.ts` strips all inherited `KANDEV_FEATURES_*` before spawn, so the e2e profile — not whatever the host exported — decides feature flags. No `unset` needed.

> **Profile-managed environment variables:** when adding an environment variable with an `e2e:` or `dev:` profile default, add it to `sanitizeInheritedEnv` in `e2e/fixtures/backend.ts` so inherited shell/task values cannot override the selected profile. Keep explicit `backend.restart({ ... })` overrides applied after baseline sanitization, so a spec can still opt into a deliberate per-test value.

> **Host oversubscription:** running >=5 heavy shards concurrently on one machine (each = Go backend + Vite-served SPA assets + Chromium + mock agent) starves CPU/IO and induces timing flakes that CI's isolated runners never see. Use 2-3 concurrent shards locally for a clean signal; see "flake triage" in the `/e2e` skill.

## Backend isolation per worker

Every Playwright worker gets:

- A unique backend port in `BACKEND_BASE_PORT + workerIndex` (default `18080+`).
- A unique frontend port in `FRONTEND_BASE_PORT + workerIndex` (default `13000+`).
- A fresh tmpdir (`HOME`, `KANDEV_HOME_DIR`, worktree base, repo clone base — all under that tmpdir).
- A unique agentctl instance port range (`30001 + E2E_PORT_OFFSET * 1000 + workerIndex * 200`).
- Its own SQLite DB.
- A process-scoped Docker ownership label for backend-created E2E containers
  and test-created SSH/storage fixtures. Cleanup and storage reporting filter
  by that label and never sweep another shard's `kandev.managed=true`
  containers.

Workers run in parallel across CI manifest shards; within a worker, tests run
serially because the `testPage` fixture calls `e2eReset` on the shared backend
before each test.

Docker image usage remains daemon-wide because images and their layers are
shared across shards; only container records, cleanup, and container-based
storage reporting are filtered by the process-scoped ownership label.

## Mocked vs real

- **Mocked**: Azure DevOps (`KANDEV_MOCK_AZURE_DEVOPS=true`), Jira (`KANDEV_MOCK_JIRA=true`), Linear (`KANDEV_MOCK_LINEAR=true`), GitHub (`KANDEV_MOCK_GITHUB=true`), and the agent process itself (`KANDEV_MOCK_AGENT=only`). These are third-party services or external processes we don't want CI to depend on.
- **Real**: Everything inside the kandev backend — orchestrator, lifecycle manager, agentctl, SSH/SFTP, Docker SDK, git, worktree manager. The point of e2e is to exercise the real code paths.

The SSH executor specifically has no mock controller. Tests use a real Docker-hosted sshd as the remote target, and fault-injection (host-key rotation, dropped traffic, killed pids) is done by operating on the container itself.

## Waiting: name the cause, don't budget for the effect

The suite's dominant flake shape is an assertion on a rendered consequence with
a hand-picked budget:

```ts
await expect(button).toBeEnabled({ timeout: 30_000 }); // why 30s? nobody knows
```

The element renders fine. It never leaves its pending state inside the budget
because the backend round trip that would flip it was late. Raising the number
buys time, it does not remove the race, and when it does fail it tells you
nothing about what was missing. Three of the four flakes measured in one recent
green CI run were exactly this, with budgets of 5s, 10s and 30s.

`helpers/causal-waits.ts` has one primitive per transport the app actually uses.
Arm the wait **before** the action, await it after, then assert the UI with its
default timeout:

```ts
import { waitForHttp, watchWs } from "../../helpers/causal-waits";

// HTTP: the branch chip cannot enable until this read returns.
const branchesLoaded = waitForHttp(page, "GET", /^\/api\/v1\/workspaces\/[^/]+\/branches$/);
await createRepositoryButton.click();
await branchesLoaded;
await expect(branchSelector).toBeEnabled(); // no budget: only a render is left

// WS notification (server push).
const ws = watchWs(page); // MUST be called before page.goto()
const processed = ws.waitForEvent("office.run.processed", {
  where: (payload) => payload.task_id === task.id,
});
await apiClient.updateRunStatus(runId, { status: "cancelled" });
await processed;

// WS request/response round trip, correlated by frame id.
const stamped = ws.waitForResponse("task.plan.implementation_started");
await implementButton.tap();
expect((await stamped).payload.implementation_started_session_id).toBe(sessionId);
```

A fourth case, "the backend has reached state X", needs no primitive:
`expect.poll(() => apiClient.getX(id))` already reads the backend directly
instead of through the DOM.

Three things that are easy to get wrong:

- **`watchWs(page)` must be called before the first `page.goto()`.** Playwright
  only reports sockets opened after the listener is attached, so a watcher
  created once the app is running observes nothing and every wait times out.
  It survives `page.reload()`.
- **Confirm the causal chain, don't infer it from the code.** Attach a throwaway
  `page.on("response")` / `page.on("websocket")` logger and run the spec once.
  Two of the three chains behind this section's example specs were not what a
  careful reading of the components predicted: the branch list comes from the
  _workspace_-scoped route, not the repository-scoped one, and a plan panel that
  already received `task.plan.created` over WS never fetches on mount at all.
- **When two transports can deliver the same fact, there is no single frame to
  wait for.** Wait on the _data_ precondition instead (assert the plan content
  is rendered before asserting the button that depends on it) rather than
  picking one transport and hoping it wins the race.

Use `predicate` when several requests share a route and you need the one
carrying a specific payload. That is strictly better than route matching,
because an unrelated refetch landing first cannot satisfy it:

```ts
const cancelledDelivered = waitForHttp(page, "GET", COMMENTS_PATH, {
  predicate: async (response) =>
    ((await response.json()) as CommentsBody).comments.some((c) => c.runStatus === "cancelled"),
});
```

Converted examples to copy from: `tests/task/create-task-new-local-repository.spec.ts`
(HTTP), `tests/task/mobile-plan-toolbar-implement.spec.ts` (both WS waits), and
`tests/office/comment-run-status.spec.ts` (HTTP with a body predicate).

### `dwell` — the only sanctioned wall-clock wait

Some delays genuinely cannot be replaced by an event. For those, and **only**
for those, use `dwell(page, ms, category, reason)`:

```ts
await dwell(page, 300, "negative-assertion", "asserting the tooltip never opens");
await dwell(page, 300, "library-timer", "Radix open delay publishes no event");
```

Backend fixtures, the API client's retry loops, docker probing and the office
routing helpers have no `Page` in scope at all. They use the **page-less form**,
same name, one greppable token either way:

```ts
await dwell(500, "poll-interval", "backend health poll; no page exists yet");
```

The two forms are told apart by the type of the first argument, since a `Page`
is never a number. **Pass the page whenever one is in scope** — the wait then
delegates to `page.waitForTimeout` and dies with the page instead of hanging
past it. The page-less form is a plain timer with nothing to cancel it.

Raw `page.waitForTimeout()` and hand-rolled promise sleeps are not sanctioned.
They are indistinguishable, at a glance and to a grep, from someone who could
not find the right event and reached for a number instead. `dwell` is greppable
by name, its category is a closed `DwellCategory` union, its reason is mandatory
rather than optional, and the whole population is countable.

> **The category is validated at runtime, not only by the type.**
> `apps/web/tsconfig.json` excludes `e2e`, and Playwright and vitest both strip
> types without checking them, so **nothing in CI typechecks a spec file**. The
> union gives you editor-time safety and self-documentation; the runtime check
> is what actually stops a typo'd category from silently escaping the closed
> set. Keep that in mind more broadly: a type error in `e2e/**` will not fail
> any gate, so lean on tests for anything load-bearing here.

| Category             | What it means                                                                                                                                                                                               |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `negative-assertion` | The assertion is that something **never** happens. There is no event for a non-event, so the only way to give the regression room to occur is to outlast the window in which it would. Permanent, not debt. |
| `product-timer`      | A `setTimeout` or debounce in our own code that publishes nothing observable. Fixable in principle: make it publish.                                                                                        |
| `library-timer`      | A third-party timer we do not control — a Radix open delay, dnd-kit sensor arming that needs React to commit a pointerdown first.                                                                           |
| `clock-separation`   | Forcing two writes apart so their timestamps or ordering stay distinguishable. Not waiting for a timer at all.                                                                                              |
| `poll-interval`      | Waiting out a polling loop. Usually the best candidate for conversion to `waitForHttp` on the request the poll makes.                                                                                       |
| `browser-chrome`     | Browser-level chrome the page cannot observe: native dialogs, focus transitions, print.                                                                                                                     |
| `unverified`         | Pre-existing spacing that could not be tied to any timer. Debt, flagged honestly rather than dressed up as intent.                                                                                          |

**Choosing between them.** Several can look applicable at once, so categorize by
_what makes the delay unavoidable_, first match wins:

1. Is the assertion that something **never** happens? → `negative-assertion`.
   You always know this from the test itself, and it outranks whatever timer
   happens to sit nearby.
2. Otherwise you are waiting out a timer. Can you **name** it? → the matching
   one of `product-timer`, `library-timer`, `clock-separation`, `poll-interval`,
   `browser-chrome`.
3. Cannot name it? → `unverified`. Do not guess a plausible-looking timer; an
   honest debt marker is worth more than a confident wrong label.

`unverified` is a **debt marker to be driven down, not a resting place** — it is
the one category that should trend toward zero. `negative-assertion` is the
opposite: it is permanent and legitimate, and a PR that "fixes" one by deleting
the wait has removed the regression's room to happen.

The `reason` must say **why no event exists**, not what the code is doing.
"Radix opens after 300ms and publishes nothing" is a reason; "wait for the
tooltip" is not — that is a `waitForHttp` or `watchWs` wait you have not found
yet. Reaching for `dwell` to avoid looking is the misuse this helper exists to
make visible, and an empty reason throws rather than passing silently.

`dwell` takes no options and has no defaults, on purpose. All of its value is in
the name, the closed category set, and the required reason; keep it that way.

### `injectLatency` — slowing a mocked response on purpose

A sleep inside a `page.route()` handler is not a wait. It slows the _system
under test_ so a pending or loading state becomes observable, and shortening or
removing it destroys the scenario:

```ts
await page.route("**/runs?*", async (route) => {
  await injectLatency(800, "make the in-flight spinner observable");
  await route.continue();
});
```

**This is a sibling of `dwell`, not a `dwell` category**, for two reasons:

- `dwell`'s `reason` is contractually an answer to "why can I not wait for an
  event here?". A latency-injection site has no such answer, so folding it in
  would mean weakening that contract for all seven categories to accommodate the
  one case that does not fit.
- Every `dwell` category is a compromise, from "permanent" to "fixable in
  principle". This one is correct by construction and must **never** be
  converted to an event wait.

Keeping them apart also keeps the numbers honest: counting fixture configuration
as sleep debt would inflate the population a ratchet is trying to drive down.

A ratchet banning raw sleeps needs to allow **both** tokens, `dwell(` and
`injectLatency(`. Do not try to exempt route handlers by scope detection
instead — a proximity heuristic misfires, and a wrong exemption silently
unguards real sleeps.

### How this is enforced

Two checks, deliberately scoped differently, because ~126 raw sleeps predate
them and `pnpm lint` is `eslint --max-warnings 0` — a repo-wide rule at _any_
severity would break every unrelated PR until the conversion finishes.

**1. The new-code ratchet** (`scripts/check-new-e2e-sleeps.mjs`) runs in CI and
pre-commit and judges the **change**, not the file:

- a file the change **added** must be clean outright;
- a file the change **modified** is judged only on the lines it touched.

So editing a line next to somebody else's `waitForTimeout` never becomes your
problem, and there is no migration treadmill. Same model as
`golangci-lint --new-from-rev` and the i18n ratchet.

**2. The eslint rule** (`eslint-rules/no-unsanctioned-sleep.mjs`) is an
**error**, but only on `e2eSleepGuardFiles` — directories the conversion has
already driven to zero. Append a directory there in the same PR that clears it.
**Never remove an entry to make a build pass**: a removal means a sleep was added
to a clean directory.

When the list would cover all of `e2e/`, replace it with `e2e/**/*.ts`. That is
the graduation, and from then on the ratchet is a second line of defence rather
than the only one.

Two commands:

```bash
pnpm run e2e:waits              # debt report: raw sleeps, dwell by category
pnpm run lint:e2e-sleeps <path> # preview the rule on a path not yet guarded
```

`e2e:waits` reports three numbers that must not be added together. Raw sleeps are
what the conversion drives to zero. **Dwell debt excludes `negative-assertion`,
which is permanent** — a test asserting something never happens has no event to
wait for, and deleting its wait removes the window the regression needs in order
to appear. `injectLatency` is counted apart from both, so deliberate fixture
configuration cannot inflate a number that is supposed to be shrinking. Inside
the debt, `unverified` is the sub-total to attack first.

**What the rule does not flag, and why.** It is an AST rule rather than a grep
because `e2e/` holds ~700 `test.setTimeout(60_000)` calls — Playwright's per-test
timeout setter, unrelated to sleeping. It also leaves alone two shapes that look
almost identical to a sleep:

- a **race guard**, where the timer is one of several resolution paths
  (`socket.onopen = …; setTimeout(() => resolve(false), 3000)`);
- a **cancellable timer**, where the handle is kept for a later `clearTimeout`.
  A sleep never keeps the handle.

The same reasoning leaves `requestAnimationFrame(() => setTimeout(resolve, 0))`
alone: that promise resolves on the **frame**, and the `setTimeout(…, 0)` only
hops out of the rAF callback so a measurement reflects painted geometry. It is
not a duration wait. This falls out of the rule's shape rather than from a
carve-out — a blanket "any `setTimeout(…, 0)` is fine" exemption would have been
a loophole.

## Adding a new spec

1. Pick a directory under `tests/` (or create one for a new feature).
2. Decide which project it belongs to. Anything that needs Docker → `tests/docker/` or `tests/ssh/` (lands in `containers`). Auth-isolated specs belong in `tests/auth/` and need `--project=auth`; provider-mutating Office routing specs use `routing`. Anything mobile-specific → name it `mobile-*.spec.ts`. Otherwise it joins `chromium` automatically.
3. Import the right test base:
   - `import { test, expect } from "../../fixtures/test-base";` for normal tests.
   - `import { test, expect } from "../../fixtures/docker-test-base";` for Docker executor tests.
   - `import { test, expect } from "../../fixtures/ssh-test-base";` for SSH executor tests.
4. Use `getByTestId` for selectors. If the surface you're testing lacks stable testids, add them — drift-prone CSS / text selectors are not worth the maintenance cost.
5. Wait on causal signals, not timeout budgets — see [Waiting](#waiting-name-the-cause-dont-budget-for-the-effect). Reach for `{ timeout: N }` only when you can say what the number is for.

## CI

`.github/workflows/e2e-tests.yml` defines two test cohorts and a report job:

- `e2e` — a 14-entry matrix executing the generated normal manifests.
- `e2e-containers` — a 6-entry matrix executing the generated container
  manifests and requiring Docker.
- `e2e-report` — merges blob reports, publishes timing/retry artifacts, and
  writes the flake rate and its trend to the job summary.

The build job uploads `e2e-shard-manifests` for the current run. Both cohorts
upload blob reports that `e2e-report` merges into a single HTML artifact.

Keep the default at `workers: 1` until the controlled worker-concurrency
experiment shows a repeatable wall-time improvement without retries. Record
worker count, shard wall time, CPU/memory pressure, setup time, and retry count
for comparisons. Measure package install, runtime image startup, and browser
extraction separately from test-work balance before changing the CI matrix.

For a local worker experiment, run the same selected heavy files twice and
save the command output. The second command is diagnostic only; it does not
change the checked-in default:

```bash
cd apps/web
E2E_SHARD=2 /usr/bin/time -v bash e2e/scripts/run-planned-shard.sh \
  e2e/manifests/normal/2.json -- --workers=1
E2E_SHARD=2 /usr/bin/time -v bash e2e/scripts/run-planned-shard.sh \
  e2e/manifests/normal/2.json -- --workers=2
```

Record results as `{ "workers": 2, "wall_seconds": 0, "max_rss_kb": 0,
"retries": 0, "backend_errors": 0 }`. Compare at least three repetitions
with the same build, duration-aware manifest, and profile before considering a
default change.
