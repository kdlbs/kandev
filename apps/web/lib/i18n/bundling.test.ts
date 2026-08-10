import { describe, expect, it } from "vitest";

import { PSEUDO_BUNDLE_ENV_VAR, shouldBundlePseudoLocale } from "./bundling";

/**
 * Both directions, deliberately.
 *
 * This switch is only exercised for real by a `vite build`, so nothing in the
 * unit or e2e suites would notice it silently stuck on "include" — which is the
 * state it is replacing. Asserting only the e2e/dev side would leave the whole
 * point of the change untested.
 */
describe("shouldBundlePseudoLocale", () => {
  it("excludes pseudo from a plain production build", () => {
    expect(shouldBundlePseudoLocale({ command: "build", env: {} })).toBe(false);
  });

  it("keeps pseudo when a build opts in", () => {
    for (const value of ["1", "true", "yes", "TRUE", " 1 "]) {
      expect(
        shouldBundlePseudoLocale({ command: "build", env: { [PSEUDO_BUNDLE_ENV_VAR]: value } }),
        `${PSEUDO_BUNDLE_ENV_VAR}=${JSON.stringify(value)} should opt in`,
      ).toBe(true);
    }
  });

  it("does not treat an arbitrary non-empty value as opting in", () => {
    for (const value of ["", "0", "false", "no", "off"]) {
      expect(
        shouldBundlePseudoLocale({ command: "build", env: { [PSEUDO_BUNDLE_ENV_VAR]: value } }),
        `${PSEUDO_BUNDLE_ENV_VAR}=${JSON.stringify(value)} should not opt in`,
      ).toBe(false);
    }
  });

  it("always keeps pseudo in serve mode, which covers `pnpm dev` and vitest", () => {
    expect(shouldBundlePseudoLocale({ command: "serve", env: {} })).toBe(true);
    expect(shouldBundlePseudoLocale({ command: "serve" })).toBe(true);
  });
});
