import { defineConfig } from "eslint/config";
import i18next from "eslint-plugin-i18next";
import reactHooks from "eslint-plugin-react-hooks";
import sonarjs from "eslint-plugin-sonarjs";
import tseslint from "typescript-eslint";

import { noLiteralStringOptions } from "./eslint.i18n.options.mjs";

/**
 * Preview config for `pnpm run lint:i18n <path>`.
 *
 * The main config (eslint.config.mjs) now applies the rule repo-wide.
 *
 * This config applies the same rule with the same options to whatever paths you
 * pass, regardless of the allowlist:
 *
 *   pnpm run lint:i18n components/task
 *
 * It is not part of CI and never gates a merge; use it as a focused work-list
 * generator when auditing a path.
 *
 * Options are imported, not duplicated, so the preview and the gate cannot
 * disagree about what counts as a string.
 */
export default defineConfig([
  {
    linterOptions: { reportUnusedDisableDirectives: "off" },
  },
  {
    files: ["**/*.{ts,tsx}"],
    ignores: ["**/*.test.ts", "**/*.test.tsx", "e2e/**", "node_modules/**", "dist/**"],
    languageOptions: { parser: tseslint.parser, parserOptions: { ecmaFeatures: { jsx: true } } },
    // Inline disables may reference rules outside this focused config. Register
    // their plugins so ESLint can still evaluate the i18n rule in isolation.
    plugins: {
      "@typescript-eslint": tseslint.plugin,
      i18next,
      "react-hooks": reactHooks,
      sonarjs,
    },
    rules: { "i18next/no-literal-string": ["error", noLiteralStringOptions] },
  },
]);
