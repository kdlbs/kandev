import { defineConfig, mergeConfig } from "vitest/config";

import viteConfig from "./vite.config";

const configuredMaxWorkers = process.env.VITEST_MAX_WORKERS?.trim();
const maxWorkers = resolveMaxWorkers(configuredMaxWorkers, Boolean(process.env.CI));

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: "happy-dom",
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
