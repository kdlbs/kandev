import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./debug",
  testMatch: "debug-expansion.spec.ts",
  timeout: 90_000,
  reporter: "list",
  use: {
    screenshot: "on",
    baseURL: "http://localhost:3001",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
