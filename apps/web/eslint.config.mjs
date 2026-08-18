import { defineConfig, globalIgnores } from "eslint/config";
import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import sonarjs from "eslint-plugin-sonarjs";
import unusedImports from "eslint-plugin-unused-imports";
import i18next from "eslint-plugin-i18next";
import tseslint from "typescript-eslint";
import { noLiteralStringOptions } from "./eslint.i18n.options.mjs";
import {
  e2eSleepGuardFiles,
  e2eSleepPlugin,
  SLEEP_EXEMPT_FILES,
} from "./eslint-rules/no-unsanctioned-sleep.mjs";

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
  // Hardcoded user-facing strings. An ERROR, REPO-WIDE.
  //
  // This was scoped to `i18nGuardFiles` while the migration was in flight: a
  // repo-wide error would have broken every unrelated PR that landed a literal
  // in un-migrated code, which is what made the first attempt unmergeable. That
  // reason has expired. Measured across all 2560 source files the rule reports
  // ZERO violations, so widening it costs nothing today and permanently removes
  // "the file was not on the list" as a way for copy to ship untranslated.
  //
  // `i18nGuardFiles` is kept: it is the migration's record, the scope
  // `pnpm run lint:i18n` previews, and a ratchet `check-guard-allowlist.mjs`
  // still enforces. It is no longer what bounds this rule.
  {
    files: ["**/*.ts", "**/*.tsx"],
    // Test files build fixtures out of literal strings on purpose; guarding them
    // would force every `label="Tasks"` in a test through the catalog. The
    // `*.test-helpers` / `*.test-utils` pair are imported only by tests and are
    // the same class.
    ignores: [
      "**/*.test.ts",
      "**/*.test.tsx",
      "**/*.test-helpers.ts",
      "**/*.test-helpers.tsx",
      "**/*.test-utils.ts",
      "**/*.test-utils.tsx",
      "e2e/**",
    ],
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
  // Unconditional wall-clock sleeps in E2E specs. An ERROR across all of `e2e/`.
  // This was a narrow allowlist while the causal-waits conversion was running —
  // `pnpm lint` is `eslint --max-warnings 0`, so a repo-wide registration at ANY
  // severity (warning included) would have failed the build on the sleeps that
  // predated the rule and blocked every unrelated PR. The conversion reached
  // zero, so `e2eSleepGuardFiles` has graduated to the whole tree and this now
  // fails only a PR that ADDS a sleep.
  // `scripts/check-new-e2e-sleeps.mjs` remains as the second line of defence: it
  // judges only the lines a change added, and covers anything `ignores` carves
  // out. Never narrow the guard to make a build pass; see the list's own comment.
  {
    files: e2eSleepGuardFiles,
    // `causal-waits.ts` implements both banned forms — see SLEEP_EXEMPT_FILES.
    ignores: SLEEP_EXEMPT_FILES,
    plugins: { "e2e-sleeps": e2eSleepPlugin },
    rules: { "e2e-sleeps/no-unsanctioned-sleep": "error" },
  },
]);

export default eslintConfig;
