import { defineConfig, globalIgnores } from "eslint/config";
import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import sonarjs from "eslint-plugin-sonarjs";
import unusedImports from "eslint-plugin-unused-imports";
import i18next from "eslint-plugin-i18next";
import tseslint from "typescript-eslint";
import { i18nGuardFiles, noLiteralStringOptions } from "./eslint.i18n.options.mjs";

const eslintConfig = defineConfig([
  {
    linterOptions: {
      reportUnusedDisableDirectives: "off",
    },
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "dist/**",
    // Test artifacts (Playwright):
    "**/test-results/**",
    "**/playwright-report/**",
  ]),
  {
    plugins: {
      "react-hooks": reactHooks,
      sonarjs,
      "unused-imports": unusedImports,
    },
    rules: {
      "no-control-regex": "off",
      "no-empty": "off",
      "no-undef": "off",
      "no-unsafe-finally": "off",
      "max-lines": ["warn", { max: 600, skipBlankLines: true, skipComments: true }],
      "max-lines-per-function": ["warn", { max: 100, skipBlankLines: true, skipComments: true }],
      complexity: ["warn", 15],
      "max-depth": ["warn", 4],
      "max-params": ["warn", 5],
      "no-nested-ternary": "warn",
      "sonarjs/cognitive-complexity": ["warn", 20],
      "sonarjs/no-duplicate-string": ["warn", { threshold: 4 }],
      "sonarjs/no-identical-functions": "warn",
      "unused-imports/no-unused-imports": "warn",
      "@typescript-eslint/no-unused-vars": [
        "warn",
        { varsIgnorePattern: "^_", argsIgnorePattern: "^_", caughtErrorsIgnorePattern: "^_" },
      ],
    },
  },
  // Hardcoded user-facing strings. An ERROR, but only on the allowlist in
  // eslint.i18n.options.mjs — paths already migrated, which is where a
  // regression is real. A repo-wide error would break every unrelated PR that
  // lands a literal; a warning would let migrated paths drift back. Each
  // migration PR externalizes one path and appends it to `i18nGuardFiles`.
  {
    files: i18nGuardFiles,
    // Test files build fixtures out of literal strings on purpose; guarding them
    // would force every `label="Tasks"` in a test through the catalog.
    ignores: ["**/*.test.ts", "**/*.test.tsx", "e2e/**"],
    plugins: { i18next },
    rules: { "i18next/no-literal-string": ["error", noLiteralStringOptions] },
  },
  // E2E tests (Playwright): disable React hooks rules since Playwright's `use()` and
  // `test.extend()` patterns are falsely flagged, and relax test-specific limits.
  {
    files: ["e2e/**/*.ts"],
    rules: {
      "react-hooks/rules-of-hooks": "off",
      "react-hooks/exhaustive-deps": "off",
      "max-lines-per-function": "off",
      "max-lines": "off",
      "sonarjs/no-duplicate-string": "off",
    },
  },
]);

export default eslintConfig;
