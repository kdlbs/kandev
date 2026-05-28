import { defineConfig, devices } from "@playwright/test";

const CI = !!process.env.CI;

export default defineConfig({
  testDir: "./tests",
  // `fullyParallel: true` is required so `--shard=N/M` splits work at the test
  // level (not the file level). With file-level sharding the suite was wildly
  // unbalanced: largest shard ran 12 min, smallest 1.5 min, because spec files
  // vary from 1 to 30+ tests. Test-level sharding flattens that distribution.
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
      name: "chromium",
      testIgnore: [
        /mobile-.*\.spec\.ts/,
        // Container-backed tests (Docker executor, SSH executor) live in the
        // `containers` project and skip when Docker is not available locally.
        // See apps/web/e2e/README.md for what runs there.
        /docker\/.*\.spec\.ts/,
        /ssh\/.*\.spec\.ts/,
        /office-routing-.*\.spec\.ts/,
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
      // hosts without a Docker daemon — Docker is used as the runtime for
      // both the Docker executor's own containers AND the sshd target the
      // SSH executor connects to. Container-bound tests are slow (~10-30s
      // each) so they live in their own project to keep the default CI fast.
      //
      // See apps/web/e2e/README.md for context and how to run locally.
      name: "containers",
      testMatch: [/docker\/.*\.spec\.ts/, /ssh\/.*\.spec\.ts/],
      use: { ...devices["Desktop Chrome"] },
      timeout: 180_000,
      // Per-test sharding: CI runs `--shard=N/6` to split this project's
      // tests across runners. Playwright only shards at the test level
      // when fullyParallel is true (otherwise sharding is by file, and
      // since this project has a single spec file every test would land
      // in shard 1). Each shard is its own process with its own backend,
      // and workers:1 still serializes tests within a shard, so this is
      // safe.
      fullyParallel: true,
    },
  ],

  // No webServer — each Playwright worker spawns its own frontend
  // via the backend fixture (see fixtures/backend.ts)

  globalSetup: "./global-setup.ts",
});
