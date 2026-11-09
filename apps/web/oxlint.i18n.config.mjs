/**
 * Focused preview for `pnpm run lint:i18n <path>`.
 *
 * The main `oxlint.config.mjs` applies the rule repo-wide. This config applies
 * the same rule with the same imported options to whatever paths are passed, so
 * it works as a focused work-list generator. It is not part of CI.
 *
 * Options are imported, not duplicated, so the preview and the gate cannot
 * disagree about what counts as a string.
 */
import { noLiteralStringOptions } from "./eslint.i18n.options.mjs";

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
  jsPlugins: [{ name: "i18next", specifier: "eslint-plugin-i18next" }],
  overrides: [
    {
      files: ["**/*.ts", "**/*.tsx"],
      excludeFiles: [
        "**/*.test.ts",
        "**/*.test.tsx",
        "**/*.test-helpers.ts",
        "**/*.test-helpers.tsx",
        "**/*.test-utils.ts",
        "**/*.test-utils.tsx",
        "e2e/**",
        "dist/**",
      ],
      rules: { "i18next/no-literal-string": ["error", noLiteralStringOptions] },
    },
  ],
};
