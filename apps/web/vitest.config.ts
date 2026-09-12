import path from "node:path";
import { defineConfig, mergeConfig } from "vitest/config";

import viteConfig from "./vite.config";
import { BASE_TEST_EXCLUDES, projectDefinitions } from "./scripts/vitest-project-selection";
import { resolveMaxWorkers } from "./scripts/vitest-worker-budget";

if (process.env.DEBUG === "1") process.env.DEBUG = "";

// Pins NODE_ENV=test — see apps/web/AGENTS.md "Testing notes" for why this is load-bearing.
process.env.NODE_ENV = "test";

const configuredMaxWorkers = process.env.VITEST_MAX_WORKERS?.trim();
const isCI = Boolean(process.env.CI);
const allowUnsafeParallelism = process.env.KANDEV_ALLOW_UNSAFE_TEST_PARALLELISM === "1";
const maxWorkers = resolveMaxWorkers(configuredMaxWorkers, isCI, allowUnsafeParallelism);
if (configuredMaxWorkers && !isCI && !allowUnsafeParallelism) {
  delete process.env.VITEST_MAX_WORKERS;
}

export default mergeConfig(
  viteConfig,
  defineConfig({
    resolve: {
      alias: [
        {
          find: /^monaco-editor$/,
          replacement: path.resolve(__dirname, "vitest.monaco-editor.ts"),
        },
      ],
    },
    test: {
      // Each project below inherits these shared worker and empty-selection
      // safeguards. Environment and setup are project-specific so Node-only
      // helpers do not pay for happy-dom, React, or locale catalogs.
      exclude: [...BASE_TEST_EXCLUDES],
      pool: "threads",
      maxWorkers,
      // Already the default, pinned because it is load-bearing: a run that
      // collects nothing must exit non-zero rather than read as a green suite.
      passWithNoTests: false,
      projects: [
        {
          extends: true,
          test: {
            name: "node",
            include: [...projectDefinitions.node.include],
            exclude: [...projectDefinitions.node.exclude],
            setupFiles: ["./vitest.setup.node.ts"],
            environment: "node",
            testTimeout: 15_000,
          },
        },
        {
          extends: true,
          test: {
            name: "browser",
            include: [...projectDefinitions.browser.include],
            exclude: [...projectDefinitions.browser.exclude],
            setupFiles: ["./vitest.setup.ts"],
            environment: "happy-dom",
            environmentOptions: {
              happyDOM: {
                settings: {
                  navigation: {
                    disableMainFrameNavigation: true,
                    disableChildFrameNavigation: true,
                  },
                },
              },
            },
          },
        },
        {
          extends: true,
          test: {
            name: "browser-locales",
            include: [...projectDefinitions["browser-locales"].include],
            exclude: [...projectDefinitions["browser-locales"].exclude],
            setupFiles: ["./vitest.setup.ts", "./vitest.setup.locales.ts"],
            environment: "happy-dom",
            environmentOptions: {
              happyDOM: {
                settings: {
                  navigation: {
                    disableMainFrameNavigation: true,
                    disableChildFrameNavigation: true,
                  },
                },
              },
            },
          },
        },
      ],
    },
  }),
);
