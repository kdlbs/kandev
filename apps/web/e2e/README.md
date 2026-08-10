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

## Adding a new spec

1. Pick a directory under `tests/` (or create one for a new feature).
2. Decide which project it belongs to. Anything that needs Docker → `tests/docker/` or `tests/ssh/` (lands in `containers`). Auth-isolated specs belong in `tests/auth/` and need `--project=auth`; provider-mutating Office routing specs use `routing`. Anything mobile-specific → name it `mobile-*.spec.ts`. Otherwise it joins `chromium` automatically.
3. Import the right test base:
   - `import { test, expect } from "../../fixtures/test-base";` for normal tests.
   - `import { test, expect } from "../../fixtures/docker-test-base";` for Docker executor tests.
   - `import { test, expect } from "../../fixtures/ssh-test-base";` for SSH executor tests.
4. Use `getByTestId` for selectors. If the surface you're testing lacks stable testids, add them — drift-prone CSS / text selectors are not worth the maintenance cost.

## CI

`.github/workflows/e2e-tests.yml` defines two test cohorts and a report job:

- `e2e` — a 14-entry matrix executing the generated normal manifests.
- `e2e-containers` — a 6-entry matrix executing the generated container
  manifests and requiring Docker.
- `e2e-report` — merges blob reports and publishes timing/retry artifacts.

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
