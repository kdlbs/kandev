# Web verification toolchain

## Required commands

- `pnpm run lint` runs `oxlint -c oxlint.config.mjs --deny-warnings .`. The configuration is the complete
  rule, scope, and exemption authority; the migration mapping lives in
  `docs/plans/native-web-verification/rule-ledger.json`.
- `pnpm run lint:i18n <path>` and `pnpm run lint:e2e-sleeps <path>` are focused previews. They import the same
  shared options and exemption lists instead of restating them.
- `pnpm run typecheck` keeps `pretypecheck` generation, runs `rich-output:check`, then runs
  `tsrs --noEmit --fast`. Keep the project settings in `tsconfig.json` authoritative.

## Retained API hosts

ESLint and classic TypeScript remain installed for RuleTester, API-based ratchets and inventory tools, and the
focused adapter suites. Normal full lint must not add a residual ESLint CLI pass, and other packages keep their
own compiler commands. The focused ESLint configs (`eslint.config.mjs`, `eslint.i18n.config.mjs`,
`eslint.e2e-sleeps.config.mjs`) remain the API hosts for those tools.

First-party guard modules keep their named exports for the ESLint harness and add a default export for the Oxlint
JS-plugin registration. The native wiring suite (`scripts/lib/native-lint-wiring.test.ts`) executes the required
CLI against the real configuration and original owner filenames.
