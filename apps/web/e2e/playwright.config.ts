import { defineConfig, devices } from "@playwright/test";

const CI = !!process.env.CI;

export default defineConfig({
  testDir: "./tests",
  // Single worker per process — CI uses --shard to split tests across matrix
  // runners (each gets its own 4 vCPUs). Tests run serially within the worker
  // because the testPage fixture does e2eReset on a shared worker-scoped
  // backend before each test.
  //
  // Isolation strategy: office-routing-* specs are gathered into their own
  // Playwright project (see below) so the worker-scoped backend env
  // (KANDEV_MOCK_PROVIDERS, KANDEV_PROVIDER_FAILURES) that those specs
  // restart with cannot leak into specs that count agents or read the
  // topbar agent name. Each routing spec restarts the backend back to
  // baseline in `afterAll` (see backend.restart() — no args = revert to
  // the fixture's baseline env snapshot).
  fullyParallel: false,
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
      testIgnore: [/mobile-.*\.spec\.ts/, /docker\/.*\.spec\.ts/, /office-routing-.*\.spec\.ts/],
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "mobile-chrome",
      testMatch: /mobile-.*\.spec\.ts/,
      use: { ...devices["Pixel 5"] },
    },
    {
      // Real-Docker E2E. Opt-in: run with `playwright test --project=docker`.
      // Spawns the backend with KANDEV_E2E_DOCKER=1, builds the
      // kandev-agent:e2e image, and skips entirely on hosts without a Docker
      // daemon. Container-bound tests are slow (~10-30s each) so they live
      // in their own project to keep the default CI fast.
      name: "docker",
      testMatch: /docker\/.*\.spec\.ts/,
      use: { ...devices["Desktop Chrome"] },
      timeout: 180_000,
    },
  ],

  // No webServer — each Playwright worker spawns its own frontend
  // via the backend fixture (see fixtures/backend.ts)

  globalSetup: "./global-setup.ts",
});
