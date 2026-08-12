import { defineConfig } from "vitest/config";

/**
 * Pin the mode instead of inheriting it. Vitest only *defaults* this
 * (`process.env.NODE_ENV ??= "test"`), so the `NODE_ENV=production` that the
 * Kandev runtime image exports (`Dockerfile`) reaches the workers untouched.
 *
 * `buildWebEnv` spreads `...process.env`, so `src/shared.test.ts`'s dev-mode case
 * asserts on that inherited value and failed inside the container while passing
 * in CI, whose image sets none. Same root cause as `web/vitest.config.ts`, where
 * it also decided which React build loaded.
 *
 * Set here rather than in the `test` script so a bare `pnpm vitest run <file>`
 * and IDE runners are covered too. `src/vitest-environment.test.ts` fails by name
 * if this is ever removed.
 */
process.env.NODE_ENV = "test";

export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.test.ts", "bin/**/*.test.mjs"],
    // Already the default, pinned because it is load-bearing: a run that
    // collects nothing must exit non-zero rather than read as a green suite.
    passWithNoTests: false,
  },
});
