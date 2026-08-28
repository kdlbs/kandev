import path from "node:path";
import { defineConfig, mergeConfig } from "vitest/config";

import viteConfig from "./vite.config";

if (process.env.DEBUG === "1") process.env.DEBUG = "";

// Pins NODE_ENV=test — see apps/web/AGENTS.md "Testing notes" for why this is load-bearing.
process.env.NODE_ENV = "test";

const configuredMaxWorkers = process.env.VITEST_MAX_WORKERS?.trim();
const maxWorkers = resolveMaxWorkers(configuredMaxWorkers, Boolean(process.env.CI));

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
      environment: "happy-dom",
      environmentOptions: {
        happyDOM: {
          settings: {
            navigation: {
              disableChildFrameNavigation: true,
            },
          },
        },
      },
      setupFiles: ["./vitest.setup.ts"],
      // `e2e/**/*.spec.ts` belongs to Playwright, but plain unit tests for the
      // e2e helpers themselves (`*.test.ts`) still run here — excluding the
      // whole tree would let them sit in the repo without ever executing.
      exclude: ["e2e/**/*.spec.ts", "e2e/fixtures/**", "e2e/pages/**", "node_modules/**"],
      pool: "threads",
      maxWorkers,
      // Already the default, pinned because it is load-bearing: a run that
      // collects nothing must exit non-zero rather than read as a green suite.
      passWithNoTests: false,
    },
  }),
);

function resolveMaxWorkers(value: string | undefined, isCI: boolean) {
  if (/^[1-9]\d*%$/.test(value ?? "")) return value;

  const workers = Number(value);
  if (Number.isInteger(workers) && workers > 0) return workers;

  return isCI ? undefined : "20%";
}
