import { defineConfig, devices } from "@playwright/test";

const CI = !!process.env.CI;

export default defineConfig({
  testDir: "./tests",
  // `fullyParallel: true` keeps direct local `--shard=N/M` runs at test level.
  // CI uses explicit duration-aware file manifests from e2e/scripts instead;
  // this setting preserves the existing local debugging behavior.
  //
  // Concurrency is still capped by `workers: 1` below — only one test runs at a
  // time per shard process, preserving the worker-scoped backend invariant that
  // the testPage fixture relies on (e2eReset before each test on a shared
  // backend). `fullyParallel: true` alone does not introduce intra-shard
  // parallelism unless workers > 1.
  //
  // Isolation strategy: office-routing-* specs are gathered into their own
  // Playwright project (see below) so the worker-scoped backend env
  // (KANDEV_MOCK_PROVIDERS, KANDEV_PROVIDER_FAILURES) that those specs
  // restart with cannot leak into specs that count agents or read the
  // topbar agent name. Each routing spec restarts the backend back to
  // baseline in `afterAll` (see backend.restart() — no args = revert to
  // the fixture's baseline env snapshot).
  fullyParallel: true,
  forbidOnly: CI,
  failOnFlakyTests: !CI || process.env.E2E_FAIL_ON_FLAKY === "1",
  retries: CI ? 2 : 0,
  workers: 1,
  timeout: 60_000,
  // CI uses blob reporter for cross-shard merge-reports; local uses list.
  reporter: CI ? [["blob", { outputDir: "./blob-report" }]] : "list",
  outputDir: "./test-results",

  use: {
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "on-first-retry",
  },

  projects: [
    {
      // The office-routing-* specs call `backend.restart()` with
      // KANDEV_MOCK_PROVIDERS, which permanently mutates the backend's
      // env for the lifetime of the worker (and registers extra
      // canonical providers). Run them in their own project so that
      // pollution can never leak into specs that count agents or read
      // the topbar agent name — see CLAUDE.md's note on
      // KANDEV_MOCK_PROVIDERS for the underlying invariant.
      name: "routing",
      testMatch: /office-routing-.*\.spec\.ts/,
      use: { ...devices["Desktop Chrome"] },
    },
    {
      // Opt-in authentication specs restart the worker's backend with
      // KANDEV_AUTH_REQUIRED=true, which locks the whole API behind a login.
      // They live in their own project so that env can never leak into the
      // default suite (which assumes an open backend and a tokenless
      // ApiClient). Serial within the file; afterAll restarts to baseline.
      name: "auth",
      testMatch: /auth\/.*\.spec\.ts/,
      // mobile-users-* specs under tests/auth/ run in the mobile-chrome
      // project (Pixel 5) instead of here.
      testIgnore: /mobile-.*\.spec\.ts/,
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "chromium",
      testIgnore: [
        /mobile-.*\.spec\.ts/,
        // Container-backed tests (Docker, SSH, and Kubernetes executors) live in the
        // `containers` project and skip when Docker is not available locally.
        // See apps/web/e2e/README.md for what runs there.
        /docker\/.*\.spec\.ts/,
        /ssh\/.*\.spec\.ts/,
        /kubernetes\/.*\.spec\.ts/,
        /kubernetes-compat\/.*\.spec\.ts/,
        /office-routing-.*\.spec\.ts/,
        // Auth specs run in the dedicated `auth` project (see above).
        /auth\/.*\.spec\.ts/,
      ],
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "mobile-chrome",
      testMatch: /mobile-.*\.spec\.ts/,
      use: { ...devices["Pixel 5"] },
    },
    {
      // Real-container E2E. Opt-in: run with `playwright test --project=containers`.
      // Spawns the backend with KANDEV_E2E_CONTAINERS=1 (KANDEV_E2E_DOCKER=1
      // is honored as a deprecated alias for one release). Builds the
      // kandev-agent:e2e and kandev-sshd:e2e images and skips entirely on
      // hosts without a Docker daemon. Docker is used as the runtime for the
      // Docker executor's own containers, the sshd target the SSH executor
      // connects to, and the pinned Kind cluster used by Kubernetes specs.
      // Container-bound tests are slow, so they live in their own project to
      // keep the default CI fast.
      //
      // See apps/web/e2e/README.md for context and how to run locally.
      name: "containers",
      testMatch: [/docker\/.*\.spec\.ts/, /ssh\/.*\.spec\.ts/, /kubernetes\/.*\.spec\.ts/],
      use: { ...devices["Desktop Chrome"] },
      timeout: 180_000,
      // Local `--shard=N/6` runs can still split this project at test level.
      // CI uses explicit files from the duration-aware containers manifests.
      // Each CI shard is its own process, and workers:1 still serializes tests
      // within a shard, so the worker-scoped backend remains safe.
      fullyParallel: true,
    },
    {
      // Small API-only Kubernetes compatibility smoke. CI runs this once for
      // each supported server version; it intentionally does not multiply the
      // full browser lifecycle matrix in the `containers` project.
      name: "kubernetes-compat",
      testMatch: /kubernetes-compat\/.*\.spec\.ts/,
      use: { ...devices["Desktop Chrome"] },
      timeout: 300_000,
      fullyParallel: true,
    },
  ],

  // No webServer — each Playwright worker spawns its own frontend
  // via the backend fixture (see fixtures/backend.ts)

  globalSetup: "./global-setup.ts",
});
