import { defineConfig } from "vitest/config";
export default defineConfig({
  resolve: { alias: { "@": new URL("../", import.meta.url).pathname } },
  test: {
    environment: "node",
    include: [
      "e2e/playwright.config.test.ts",
      "e2e/fixtures/upgrade-backend.test.ts",
      "e2e/helpers/upgrade-oracle.test.ts",
    ],
    maxWorkers: 1,
    passWithNoTests: false,
  },
});
