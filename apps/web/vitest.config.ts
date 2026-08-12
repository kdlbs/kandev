import { defineConfig, mergeConfig } from "vitest/config";

import viteConfig from "./vite.config";

if (process.env.DEBUG === "1") process.env.DEBUG = "";

/**
 * Pin the mode instead of inheriting it. Vitest only *defaults* this
 * (`process.env.NODE_ENV ??= "test"`) and then hands the value straight to the
 * worker pool, so an ambient `NODE_ENV` wins — and the Kandev runtime image
 * exports `NODE_ENV=production` (`Dockerfile`), which every agent and container
 * dev shell inherits.
 *
 * That is not a cosmetic difference. `react/index.js` picks its build from this
 * variable, and `cjs/react.production.js` does not export `act`; testing-library's
 * act-compat shim then falls back to the deprecated `react-dom/test-utils`, whose
 * production build calls `React.act` and throws `TypeError: React.act is not a
 * function` on the first `render()`. Every component and hook suite died there —
 * 178 of 268 tests — while CI stayed green, because CI's image sets no NODE_ENV.
 *
 * Set here rather than in the `test` script so a bare `pnpm vitest run <file>`
 * and IDE runners are covered too; that single-file path is how the breakage was
 * found. `vitest-environment.test.tsx` fails by name if this is ever removed.
 */
process.env.NODE_ENV = "test";

const configuredMaxWorkers = process.env.VITEST_MAX_WORKERS?.trim();
const maxWorkers = resolveMaxWorkers(configuredMaxWorkers, Boolean(process.env.CI));

export default mergeConfig(
  viteConfig,
  defineConfig({
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
