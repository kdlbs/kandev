import { defineConfig, devices } from "@playwright/test";

const CI = !!process.env.CI;

export default defineConfig({
  testDir: "./tests",
  // Parallelize across workers (each worker = own backend/frontend), but run
  // tests sequentially within each worker. The testPage fixture does e2eReset
  // on a shared worker-scoped backend before each test — concurrent tests
  // within a worker would interfere via cleanup races.
  fullyParallel: false,
  forbidOnly: CI,
  retries: CI ? 2 : 0,
  workers: CI ? 4 : 1,
  timeout: 60_000,
  reporter: CI ? [["html", { outputFolder: "./playwright-report" }]] : "list",
  outputDir: "./test-results",

  use: {
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "on-first-retry",
  },

  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],

  // No webServer — each Playwright worker spawns its own frontend
  // via the backend fixture (see fixtures/backend.ts)

  globalSetup: "./global-setup.ts",
});
