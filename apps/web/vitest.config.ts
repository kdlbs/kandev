import { defineConfig, mergeConfig } from "vitest/config";

import viteConfig from "./vite.config";

if (process.env.DEBUG === "1") process.env.DEBUG = "";

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
      exclude: ["e2e/**", "node_modules/**"],
      pool: "threads",
      maxWorkers,
    },
  }),
);

function resolveMaxWorkers(value: string | undefined, isCI: boolean) {
  if (/^[1-9]\d*%$/.test(value ?? "")) return value;

  const workers = Number(value);
  if (Number.isInteger(workers) && workers > 0) return workers;

  return isCI ? undefined : "20%";
}
