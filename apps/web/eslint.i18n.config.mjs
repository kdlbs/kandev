import { defineConfig } from "eslint/config";
import i18next from "eslint-plugin-i18next";
import tseslint from "typescript-eslint";

import { noLiteralStringOptions } from "./eslint.i18n.options.mjs";

/**
 * Preview config for `pnpm run lint:i18n <path>`.
 *
 * The main config (eslint.config.mjs) only errors on `i18nGuardFiles` — paths
 * already migrated. That is the point: unrelated PRs must not be blocked by
 * strings in code nobody has touched. But it means you cannot use `pnpm lint` to
 * find out what is left in a directory you are ABOUT to migrate, because the rule
 * is not on there yet.
 *
 * This config applies the same rule with the same options to whatever paths you
 * pass, regardless of the allowlist:
 *
 *   pnpm run lint:i18n components/task
 *
 * It is not part of CI and never gates a merge — it is the work-list generator.
 * When the directory reports clean, add it to `i18nGuardFiles` in the same PR so
 * the main config keeps it clean from then on.
 *
 * Options are imported, not duplicated, so the preview and the gate cannot
 * disagree about what counts as a string.
 */
export default defineConfig([
  {
    files: ["**/*.{ts,tsx}"],
    ignores: ["**/*.test.ts", "**/*.test.tsx", "e2e/**", "node_modules/**", "dist/**"],
    languageOptions: { parser: tseslint.parser, parserOptions: { ecmaFeatures: { jsx: true } } },
    plugins: { i18next },
    rules: { "i18next/no-literal-string": ["error", noLiteralStringOptions] },
  },
]);
