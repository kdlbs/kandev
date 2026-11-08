/**
 * Focused config for the sleep ratchet and its preview command.
 *
 * The main `oxlint.config.mjs` covers all of `e2e/`. This isolated config stays
 * separate because `scripts/check-new-e2e-sleeps.mjs` lints paths the main
 * config's exclusions may carve out and needs findings that are unambiguously
 * sleeps. The rule and the exemption list are imported, not restated.
 */
import { e2eSleepGuardFiles, SLEEP_EXEMPT_FILES } from "./eslint-rules/no-unsanctioned-sleep.mjs";

export default {
  categories: {
    correctness: "off",
    suspicious: "off",
    pedantic: "off",
    perf: "off",
    style: "off",
    restriction: "off",
    nursery: "off",
  },
  plugins: ["eslint", "typescript"],
  jsPlugins: [{ name: "e2e-sleeps", specifier: "./eslint-rules/no-unsanctioned-sleep.mjs" }],
  overrides: [
    {
      files: e2eSleepGuardFiles,
      excludeFiles: [...SLEEP_EXEMPT_FILES, "dist/**"],
      rules: { "e2e-sleeps/no-unsanctioned-sleep": "error" },
    },
  ],
};
