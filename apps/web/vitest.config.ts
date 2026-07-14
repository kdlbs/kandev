import { defineConfig, mergeConfig } from "vitest/config";

import viteConfig from "./vite.config";

const configuredMaxWorkers = process.env.VITEST_MAX_WORKERS?.trim();

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: "happy-dom",
      setupFiles: ["./vitest.setup.ts"],
      exclude: ["e2e/**", "node_modules/**"],
      pool: "threads",
      maxWorkers: configuredMaxWorkers || (process.env.CI ? undefined : "20%"),
    },
  }),
);
