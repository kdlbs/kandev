# Kandev Web — E2E Test Suite

Playwright-based end-to-end tests. Each Playwright worker spawns its own real Go backend (no mocks of internal services) on isolated ports and a real Next.js standalone frontend, then drives a real Chromium against them.

## Project layout

| Folder                 | What's in it                                                                                                                                 |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `tests/`               | Spec files, grouped by feature (`chat/`, `docker/`, `git/`, `integrations/`, `kanban/`, `pr/`, `search/`, `session/`, `ssh/`, `layout/`, …). |
| `fixtures/`            | Worker-scoped fixtures that own the backend lifecycle (`backend.ts`, `docker-test-base.ts`, `ssh-test-base.ts`, `test-base.ts`).             |
| `helpers/`             | Reusable building blocks for specs (`api-client.ts`, `docker.ts`, `ssh.ts`, `git-helper.ts`, `ws-capture.ts`, …).                            |
| `pages/`               | Page Objects (one class per top-level UI surface — `SessionPage`, `KanbanPage`, `JiraSettingsPage`, `SSHSettingsPage`, …).                   |
| `playwright.config.ts` | Project definitions, timeouts, sharding config.                                                                                              |
| `global-setup.ts`      | Pre-flight checks for required binaries (kandev, mock-agent, standalone next build).                                                         |

## Playwright projects

The suite is split into three projects. Pick one with `--project=<name>`.

### `chromium` (default)

The everyday surface — runs in every CI shard. Excludes the heavyweight `containers` specs and the mobile specs.

```sh
pnpm e2e
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
KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers
```

Or a single spec:

```sh
KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers tests/ssh/launch-task.spec.ts
```

### Why "containers" instead of "docker"?

This project used to be named `docker`. It was renamed to `containers` once SSH e2e tests joined it — calling it `docker` was misleading because SSH tests have nothing to do with the Docker _executor_; they just happen to use Docker as the runtime that hosts the sshd target.

`KANDEV_E2E_DOCKER=1` is still honored as a deprecated alias for `KANDEV_E2E_CONTAINERS=1` for one release. Local scripts and stale CI configs won't break, but new code should use the new name.

## Commands

| Command                            | What it does                                     |
| ---------------------------------- | ------------------------------------------------ |
| `pnpm e2e`                         | Run the default (chromium) project headless.     |
| `pnpm e2e:ui`                      | Open Playwright's UI mode for interactive runs.  |
| `pnpm e2e:headed`                  | Run headless project but with a visible browser. |
| `pnpm e2e --project=containers`    | Run container-backed tests (needs Docker).       |
| `pnpm e2e --project=mobile-chrome` | Run mobile responsive tests.                     |
| `E2E_DEBUG=1 pnpm e2e`             | Surface Docker build output + extra logging.     |

Common flags: `--shard=1/4`, `-g "fragment of test name"`, `--repeat-each=3` (flake hunting).

## Backend isolation per worker

Every Playwright worker gets:

- A unique backend port in `BACKEND_BASE_PORT + workerIndex` (default `18080+`).
- A unique frontend port in `FRONTEND_BASE_PORT + workerIndex` (default `13000+`).
- A fresh tmpdir (`HOME`, `KANDEV_HOME_DIR`, worktree base, repo clone base — all under that tmpdir).
- A unique agentctl instance port range (`30001 + E2E_PORT_OFFSET * 1000 + workerIndex * 200`).
- Its own SQLite DB.

Workers run in parallel across CI shards (`--shard=N/M`); within a worker, tests run serially because the `testPage` fixture calls `e2eReset` on the shared backend before each test.

## Mocked vs real

- **Mocked**: Jira (`KANDEV_MOCK_JIRA=true`), Linear (`KANDEV_MOCK_LINEAR=true`), GitHub (`KANDEV_MOCK_GITHUB=true`), the agent process itself (`KANDEV_MOCK_AGENT=only`). These are third-party services or external processes we don't want CI to depend on.
- **Real**: Everything inside the kandev backend — orchestrator, lifecycle manager, agentctl, SSH/SFTP, Docker SDK, git, worktree manager. The point of e2e is to exercise the real code paths.

The SSH executor specifically has no mock controller. Tests use a real Docker-hosted sshd as the remote target, and fault-injection (host-key rotation, dropped traffic, killed pids) is done by operating on the container itself.

## Adding a new spec

1. Pick a directory under `tests/` (or create one for a new feature).
2. Decide which project it belongs to. Anything that needs Docker → `tests/docker/` or `tests/ssh/` (lands in `containers`). Anything mobile-specific → name it `mobile-*.spec.ts`. Otherwise it joins `chromium` automatically.
3. Import the right test base:
   - `import { test, expect } from "../../fixtures/test-base";` for normal tests.
   - `import { test, expect } from "../../fixtures/docker-test-base";` for Docker executor tests.
   - `import { test, expect } from "../../fixtures/ssh-test-base";` for SSH executor tests.
4. Use `getByTestId` for selectors. If the surface you're testing lacks stable testids, add them — drift-prone CSS / text selectors are not worth the maintenance cost.

## CI

`.github/workflows/e2e-tests.yml` defines two jobs:

- `e2e` — matrixed `chromium` + `mobile-chrome` shards.
- `e2e-containers` — single job, runs `--project=containers`, needs Docker.

Both upload blob reports that `e2e-report` merges into a single HTML artifact.
