import { describe, expect, it } from "vitest";

/**
 * Guards the test environment itself rather than any product code.
 *
 * `buildWebEnv` spreads `...process.env`, so `shared.test.ts`'s
 * "does not set browser API port env vars in dev mode" case asserts on whatever
 * `NODE_ENV` the runner inherited. Vitest only *defaults* the variable
 * (`process.env.NODE_ENV ??= "test"`), so inside the Kandev runtime image —
 * which exports `NODE_ENV=production` (`Dockerfile`) — that assertion failed
 * locally while CI, whose image sets none, stayed green.
 *
 * `vitest.config.ts` pins the value. This names the reason so the next failure
 * reads as an environment regression rather than a bug in `buildWebEnv`.
 */
describe("vitest environment", () => {
  it("runs with NODE_ENV=test regardless of the inherited value", () => {
    expect(process.env.NODE_ENV).toBe("test");
  });
});
