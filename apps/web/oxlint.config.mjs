/**
 * Required web lint configuration.
 *
 * This is the complete native replacement for `eslint.config.mjs`. It keeps
 * every rule, severity, scope and exemption the ESLint configuration enforced;
 * `docs/plans/native-web-verification/rule-ledger.json` records the mapping.
 *
 * `eslint.config.mjs` remains the compatibility host for RuleTester and the
 * API-based ratchets and inventory tools. It no longer runs the full lint.
 *
 * Default categories and plugins are switched off deliberately. Oxlint enables
 * its `correctness` category and a base plugin set by default, which is not the
 * ESLint rule set this repository reviewed.
 */
import { noLiteralStringOptions } from "./eslint.i18n.options.mjs";
import { e2eSleepGuardFiles, SLEEP_EXEMPT_FILES } from "./eslint-rules/no-unsanctioned-sleep.mjs";

const NO_WARNINGS_CATEGORIES = {
  correctness: "off",
  suspicious: "off",
  pedantic: "off",
  perf: "off",
  style: "off",
  restriction: "off",
  nursery: "off",
};

const testFileExclusions = [
  "**/*.test.ts",
  "**/*.test.tsx",
  "**/*.test-helpers.ts",
  "**/*.test-helpers.tsx",
  "**/*.test-utils.ts",
  "**/*.test-utils.tsx",
];

export default {
  $schema: "./node_modules/oxlint/configuration_schema.json",
  categories: NO_WARNINGS_CATEGORIES,
  plugins: ["eslint", "typescript"],
  jsPlugins: [
    { name: "i18next", specifier: "eslint-plugin-i18next" },
    { name: "sonarjs", specifier: "eslint-plugin-sonarjs" },
    { name: "unused-imports", specifier: "eslint-plugin-unused-imports" },
    { name: "@tanstack/query", specifier: "@tanstack/eslint-plugin-query" },
    { name: "task-links", specifier: "./eslint-rules/no-task-link-bypass.mjs" },
    { name: "system-query-owner", specifier: "./eslint-rules/no-migrated-system-query-owner.mjs" },
    { name: "e2e-sleeps", specifier: "./eslint-rules/no-unsanctioned-sleep.mjs" },
  ],
  ignorePatterns: [
    ".next/**",
    "out/**",
    "build/**",
    "dist/**",
    "dist-browser-demo/**",
    "**/test-results/**",
    "**/playwright-report/**",
  ],
  rules: {
    // Explicitly disabled in the ESLint configuration. Oxlint's default
    // categories are already off; these stay named so a category change cannot
    // switch them on.
    "no-control-regex": "off",
    "no-empty": "off",
    "no-undef": "off",
    "no-unsafe-finally": "off",

    // Size and complexity budgets (warnings, promoted to failures by
    // `--deny-warnings`).
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
    "no-unused-vars": [
      "warn",
      { varsIgnorePattern: "^_", argsIgnorePattern: "^_", caughtErrorsIgnorePattern: "^_" },
    ],

    // TypeScript rules formerly enabled by typescript-eslint's recommended set.
    "typescript/ban-ts-comment": "error",
    "no-array-constructor": "error",
    "typescript/no-duplicate-enum-values": "error",
    "typescript/no-empty-object-type": "error",
    "typescript/no-explicit-any": "error",
    "typescript/no-extra-non-null-assertion": "error",
    "typescript/no-misused-new": "error",
    "typescript/no-namespace": "error",
    "typescript/no-non-null-asserted-optional-chain": "error",
    "typescript/no-require-imports": "error",
    "typescript/no-this-alias": "error",
    "typescript/no-unnecessary-type-constraint": "error",
    "typescript/no-unsafe-declaration-merging": "error",
    "typescript/no-unsafe-function-type": "error",
    "no-unused-expressions": [
      "error",
      { allowShortCircuit: false, allowTaggedTemplates: false, allowTernary: false },
    ],
    "typescript/no-wrapper-object-types": "error",
    "typescript/prefer-as-const": "error",
    "typescript/prefer-namespace-keyword": "error",
    "typescript/triple-slash-reference": "error",

    // ESLint's recommended core rules.
    "for-direction": "error",
    "no-async-promise-executor": "error",
    "no-case-declarations": "error",
    "no-compare-neg-zero": "error",
    "no-cond-assign": ["error", "except-parens"],
    "no-constant-binary-expression": "error",
    "no-constant-condition": ["error", { checkLoops: "allExceptWhileTrue" }],
    "no-debugger": "error",
    "no-delete-var": "error",
    "no-dupe-else-if": "error",
    "no-duplicate-case": "error",
    "no-empty-character-class": "error",
    "no-empty-pattern": ["error", { allowObjectPatternsAsParameters: false }],
    "no-empty-static-block": "error",
    "no-ex-assign": "error",
    "no-extra-boolean-cast": "error",
    "no-fallthrough": "error",
    "no-global-assign": "error",
    "no-invalid-regexp": "error",
    "no-irregular-whitespace": [
      "error",
      {
        skipComments: false,
        skipJSXText: false,
        skipRegExps: false,
        skipStrings: true,
        skipTemplates: false,
      },
    ],
    "no-loss-of-precision": "error",
    "no-misleading-character-class": "error",
    "no-nonoctal-decimal-escape": "error",
    "no-prototype-builtins": "error",
    "no-regex-spaces": "error",
    "no-self-assign": ["error", { props: true }],
    "no-shadow-restricted-names": ["error", { reportGlobalThis: false }],
    "no-sparse-arrays": "error",
    "no-unexpected-multiline": "error",
    "no-unsafe-optional-chaining": "error",
    "no-unused-labels": "error",
    "no-unused-private-class-members": "error",
    "no-useless-backreference": "error",
    "no-useless-catch": "error",
    "no-useless-escape": "error",
    "require-yield": "error",
    "use-isnan": ["error", { enforceForSwitchCase: true, enforceForIndexOf: false }],
    "valid-typeof": ["error", { requireStringLiterals: false }],
  },
  overrides: [
    {
      // JavaScript-only rules from the previous `js.configs.recommended`
      // scope. TypeScript owns the corresponding checks in TS files through
      // `typescript-eslint`'s recommended set.
      files: ["**/*.js", "**/*.jsx", "**/*.mjs", "**/*.cjs"],
      rules: {
        "constructor-super": "error",
        "getter-return": ["error", { allowImplicit: false }],
        "no-class-assign": "error",
        "no-const-assign": "error",
        "no-dupe-class-members": "error",
        "no-dupe-keys": "error",
        "no-func-assign": "error",
        "no-import-assign": "error",
        "no-new-native-nonconstructor": "error",
        "no-obj-calls": "error",
        "no-redeclare": ["error", { builtinGlobals: true }],
        "no-setter-return": "error",
        "no-this-before-super": "error",
        "no-unreachable": "error",
        "no-unsafe-negation": ["error", { enforceForOrderingRelations: false }],
        "no-with": "error",
      },
    },
    {
      // These four are TypeScript-only in `typescript-eslint`'s recommended
      // set. Every category is off, so the compiler-owned checks stay off
      // without repeating each disabled rule here.
      files: ["**/*.ts", "**/*.tsx", "**/*.mts", "**/*.cts"],
      rules: {
        "no-var": "error",
        "prefer-const": "error",
        "prefer-rest-params": "error",
        "prefer-spread": "error",
      },
    },
    {
      // The four Query rules keep their existing severities. `no-void-query-fn`
      // remains absent because it needs parser services this project does not
      // enable.
      files: ["**/*.ts", "**/*.tsx"],
      rules: {
        "@tanstack/query/exhaustive-deps": "error",
        "@tanstack/query/no-rest-destructuring": "warn",
        "@tanstack/query/no-unstable-deps": "error",
        "@tanstack/query/stable-query-client": "error",
      },
    },
    {
      // Hardcoded user-facing strings: repo-wide for app source. Test files and
      // `e2e/` build fixtures from literal copy on purpose.
      files: ["**/*.ts", "**/*.tsx"],
      excludeFiles: [...testFileExclusions, "e2e/**"],
      jsPlugins: [{ name: "i18next", specifier: "eslint-plugin-i18next" }],
      rules: { "i18next/no-literal-string": ["error", noLiteralStringOptions] },
    },
    {
      // Task workbench URLs and anchors have one builder and one click
      // transport. Tests and the authority module itself are excluded.
      files: ["**/*.ts", "**/*.tsx"],
      excludeFiles: [...testFileExclusions, "e2e/**", "lib/links.ts"],
      jsPlugins: [{ name: "task-links", specifier: "./eslint-rules/no-task-link-bypass.mjs" }],
      rules: { "task-links/no-task-link-bypass": "error" },
    },
    {
      // Keep graph libraries behind their renderer and layout adapters in the
      // lint configuration used by the full web check.
      files: ["**/*.ts", "**/*.tsx"],
      excludeFiles: [
        "lib/task-graph/layout.ts",
        "components/task-graph/xyflow-task-graph-renderer.tsx",
        ...testFileExclusions,
      ],
      rules: {
        "no-restricted-imports": [
          "error",
          {
            paths: [
              {
                name: "@dagrejs/dagre",
                message: "Only lib/task-graph/layout.ts may import dagre.",
              },
            ],
            patterns: [
              {
                group: ["@xyflow/*"],
                message:
                  "Only components/task-graph/xyflow-task-graph-renderer.tsx may import xyflow.",
              },
            ],
          },
        ],
      },
    },
    {
      // The four migrated System Query owners.
      files: [
        "lib/state/slices/system/types.ts",
        "lib/state/slices/system/system-slice.ts",
        "lib/state/slices/system/index.ts",
        "lib/state/hydration/hydrator.ts",
      ],
      jsPlugins: [
        {
          name: "system-query-owner",
          specifier: "./eslint-rules/no-migrated-system-query-owner.mjs",
        },
      ],
      rules: { "system-query-owner/no-migrated-system-zustand-owner": "error" },
    },
    {
      // Playwright specs use `use()` and `test.extend()` shapes the hooks rules
      // misread, and carry long test bodies.
      files: ["e2e/**/*.ts"],
      rules: {
        "react-hooks/rules-of-hooks": "off",
        "react-hooks/exhaustive-deps": "off",
        "max-lines-per-function": "off",
        "max-lines": "off",
        "sonarjs/no-duplicate-string": "off",
      },
    },
    {
      // Unconditional wall-clock sleeps in E2E specs are errors across `e2e/`.
      files: e2eSleepGuardFiles,
      excludeFiles: SLEEP_EXEMPT_FILES,
      jsPlugins: [{ name: "e2e-sleeps", specifier: "./eslint-rules/no-unsanctioned-sleep.mjs" }],
      rules: { "e2e-sleeps/no-unsanctioned-sleep": "error" },
    },
  ],
};
