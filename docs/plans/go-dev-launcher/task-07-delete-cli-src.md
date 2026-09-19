---
id: "07-delete-cli-src"
title: "Delete apps/cli/src and drop CLI devDependencies"
status: done
wave: 3
depends_on: ["06-makefile-cutover"]
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 07: Delete `apps/cli/src` and drop CLI devDependencies

With `make dev` on the Go path, `apps/cli/src/**` has no consumer. Remove it and reduce
the package to what npm actually publishes.

## Acceptance

- `apps/cli/src/`, `apps/cli/tsconfig.json`, and `apps/cli/vitest.config.ts` are deleted.
  What remains is `package.json` and `bin/{cli.js,native-shim.js,native-shim.test.mjs}`.
- `apps/cli/bin/native-shim.test.mjs` runs on the Node built-in test runner
  (`node:test` + `node:assert`) instead of Vitest, with equivalent coverage;
  `apps/cli/package.json` has no `devDependencies` and its `test` script is
  `node --test "bin/*.test.mjs"` (implemented as such; a bare `node --test bin/`
  fails on this Node version, and bare `node --test` picks up the file by
  pattern). The `dev`, `bundle`, and `prepublishOnly` scripts are removed or
  reduced to what still applies.
- `name`, `version`, `bin`, `files`, `main`, `license`, `engines`, and
  `optionalDependencies` in `apps/cli/package.json` are byte-identical to before, so
  `scripts/release/publish-npm.sh:134,259` and `.github/workflows/release.yml:224,251,381`
  keep working.
- `make test-cli`, `make test`, and `make test-windows` pass; `make typecheck` passes
  after being converted from `pnpm -r exec tsc` to explicit `--filter` arguments
  (`Makefile:650`), since `apps/cli` no longer has a `tsconfig.json`.
- `cd apps && pnpm install --frozen-lockfile` succeeds with a regenerated
  `apps/pnpm-lock.yaml`, and `pnpm run dead-code` reports nothing new.

## Verification

~~~bash
cd apps && pnpm install && pnpm --filter kandev test && pnpm run format:check && pnpm run dead-code
cd ..
make typecheck && make test-cli
~~~

Then confirm the published surface is unchanged:

~~~bash
cd apps/cli && npm pack --dry-run
~~~

The file list must be exactly `bin/cli.js`, `bin/native-shim.js`, `package.json`.

## Files

- Deleted: `apps/cli/src/**` (all 20 modules, their tests, `service/`, `supervisor/`),
  `apps/cli/tsconfig.json`, `apps/cli/vitest.config.ts`
- Modified: `apps/cli/package.json`, `apps/cli/bin/native-shim.test.mjs`,
  `apps/pnpm-lock.yaml`, `Makefile` (`typecheck`)
- Check but likely unchanged: `apps/package.json` (`format` globs still cover `cli`),
  `apps/pnpm-workspace.yaml` (keep `cli` in the workspace),
  `.github/workflows/frontend-tests.yml` (the `apps/cli/**` path trigger stays valid)

## Inputs

- `apps/cli/package.json` — current `files`/`bin`/`optionalDependencies`.
- `apps/cli/bin/native-shim.test.mjs` — the Vitest-based tests to port.
- `scripts/release/publish-npm.sh` and `scripts/release/publish-npm.test.mjs` — what the
  release flow reads from this package.

## Risks

- Deleting `tsconfig.json` breaks `pnpm -r exec tsc` (Makefile:650) for the whole
  workspace, not just `apps/cli`. Convert that target in the same change or the
  typecheck job fails.
- `esbuild`, `tar`, `tree-kill`, and `tsx` disappear from the lockfile. Re-run the full
  frontend test job locally, not just the CLI one, before pushing.
- Do not remove `cli` from `apps/pnpm-workspace.yaml`; prettier's `format` script and
  the shim test both still expect it to be a workspace member.
