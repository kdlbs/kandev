import { defineConfig, devices } from "@playwright/test";

const CI = !!process.env.CI;

export default defineConfig({
  testDir: "./tests",
  // Single worker per process — CI uses --shard to split tests across matrix
  // runners (each gets its own 4 vCPUs). Tests run serially within the worker
  // because the testPage fixture does e2eReset on a shared worker-scoped
  // backend before each test.
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
      name: "chromium",
      testIgnore: [
        /mobile-.*\.spec\.ts/,
        // Container-backed tests (Docker executor, SSH executor) live in the
        // `containers` project and skip when Docker is not available locally.
        // See apps/web/e2e/README.md for what runs there.
        /docker\/.*\.spec\.ts/,
        /ssh\/.*\.spec\.ts/,
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
    },
  ],

  // No webServer — each Playwright worker spawns its own frontend
  // via the backend fixture (see fixtures/backend.ts)

  globalSetup: "./global-setup.ts",
});
