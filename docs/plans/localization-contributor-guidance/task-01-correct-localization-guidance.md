---
id: "01-correct-localization-guidance"
title: "Correct localization guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-I18N-001
acceptance_criteria:
  - AC-PLATFORM-I18N-001.9
  - AC-PLATFORM-I18N-001.10
system_design:
  - ../../specs/platform/system-design/i18n.md
---

# Task 01: Correct localization guidance

## Summary

Make the current contributor instructions accurate and executable from the
repository root. Preserve the guide's historical migration and audit narrative.

## In scope

- Update the localization section of `docs/public/web-development.md` and the
  current quick-start and command guidance in `docs/i18n.md`.
- State English plus `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`, and `ko` are
  required, generate `zh-tw` and `zh-hk` with `i18n:zh-hant`, and explain the
  real catalog hard gate.
- Explain the repo-wide non-JSX scan and its relationship to JSX lint, and
  distinguish required checks from the advisory parity and sweep reports.
- Make each revised command explicitly runnable from the repository root;
  retain correct argument paths for the web package working directory.

## Out of scope

- Source, catalogs, scripts, policy, CI, harness, hooks, AGENTS files, new
  permanent tests, browser tests, captures, and unrelated guide sections.
- Rewriting historical migration or audit narrative in `docs/i18n.md`.

## Acceptance

1. Both current guidance sections agree with the locale and guard behavior in
   the package scripts and implementations, including report input failures.
2. Revised command examples run from the root; readers can identify mandatory
   checks, advisory reports, and the Traditional Chinese generation procedure.
3. Targeted checks and direct source-to-guidance review pass, with no change to
   runtime behavior or historical narrative.

## Verification

After the separately granted implementation and global HEAVY serial lease,
install dependencies once from `apps/` in a fresh worktree. Setup errors are
not evidence of a product regression. Run these commands from the
repository root with the session-local pinned tools; do not change global
environment configuration. Record actual exit results. Use a unique owned
`KANDEV_RUN_QUIET_DIR` for each `scripts/run-quiet` invocation if used; do not
pass `--summary` for these non-E2E commands.

```bash
PATH=/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH pnpm --dir apps install --frozen-lockfile
PATH=/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH pnpm --dir apps/web run i18n:check
PATH=/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH pnpm --dir apps/web run i18n:ratchet
PATH=/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH pnpm --dir apps/web run i18n:parity
PATH=/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH pnpm --dir apps/web run i18n:sweep lib/i18n
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node --test scripts/validate-public-docs.test.mjs
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Compare the revised text directly with `apps/web/package.json`,
`apps/web/scripts/check-i18n-keys.mjs`, `apps/web/scripts/check-nonjsx-copy.mjs`,
`apps/web/scripts/i18n-parity.mjs`, and `apps/web/scripts/i18n-sweep.mjs`.
Review the diff for preserved historical text and unrelated sections. Execute
any additional revised non-mutating command examples with valid arguments;
inspect generator commands against the package script without writing catalogs.
Record the commands and outcomes in Results. Never infer an exit from a second
run: retain and join each native handle.

## Files likely touched

- `docs/public/web-development.md`
- `docs/i18n.md`
- This plan, work order, and the existing platform i18n pair for lifecycle and
  verification records after successful implementation.

## Dependencies

None. Implementation starts only after design review and a later separate
implementation and HEAVY release.

## Risks

- Commands interpreted in the wrong working directory can fail or inspect the
  wrong path. Reports can fail on malformed or unreadable input despite their
  findings being advisory.

## Parallelism

Sequential.

## Inputs

- [Platform i18n requirements](../../specs/platform/requirements/i18n.md) and
  [system design](../../specs/platform/system-design/i18n.md).
- The two target guides and the package scripts and check implementations.

## Results

The following commands ran from the repository root with the pinned Node 24.21.0
and pnpm 9.15.9 paths above. Every command exited 0. The install and check had
native handles 60845 and 99096, respectively, both joined to exit 0. The lint
preview and direct non-JSX check had handles 40922 and 25846, both joined to
exit 0. All other commands completed directly.

- Frozen install: `/tmp/kandev-localization-033-install/kandev-run.install.reKQumX3.log`.
- `i18n:check`: `/tmp/kandev-localization-033-check/kandev-run.i18n-check.C8VKRayE.log`.
- `i18n:ratchet`: `/tmp/kandev-localization-033-ratchet/kandev-run.i18n-ratchet.aJKcVbXJ.log`.
- `i18n:parity`: `/tmp/kandev-localization-033-parity/kandev-run.i18n-parity.HYY6fIhN.log`.
- `i18n:sweep lib/i18n`: `/tmp/kandev-localization-033-sweep/kandev-run.i18n-sweep.zPG8zxW1.log`.
- `lint:i18n lib/i18n`: `/tmp/kandev-localization-033-lint-preview/kandev-run.lint-preview.aECfohfg.log`.
- Direct non-JSX scan: `/tmp/kandev-localization-033-nonjsx/kandev-run.nonjsx.xo1m5lFb.log`.
- `i18n:removed`: `/tmp/kandev-localization-033-removed/kandev-run.i18n-removed.LsSpLNK6.log`.
- Public-doc validator tests and live validator: `/tmp/kandev-localization-033-public-tests/kandev-run.public-tests.NY5SMKRB.log` and `/tmp/kandev-localization-033-public-validation/kandev-run.public-validation.vvQh1r2q.log`.
- Catalog validation and specification lint: `/tmp/kandev-localization-033-catalog/kandev-run.catalog.irEWONTL.log` and `/tmp/kandev-localization-033-spec-lint/kandev-run.spec-lint.rTig5m2C.log`.
- After the final guidance and lifecycle edits, public validation, specification
  lint, and catalog validation again exited 0: `/tmp/kandev-localization-033-public-final/kandev-run.public-final.kTGL8D3E.log`, `/tmp/kandev-localization-033-spec-final/kandev-run.spec-final.a900aZxB.log`, and `/tmp/kandev-localization-033-catalog-final/kandev-run.catalog-final.syKa9POI.log`.

The generating commands were checked against `apps/web/package.json` without
writing catalogs. The revised guidance was compared to the package scripts and
the key, non-JSX, parity, and sweep implementations. The ROOT qualification
reported repository-root `pnpm run i18n:check` failing with
`ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND` and the web-scoped command passing; this
documentation-only correction does not need a synthetic failing test.

Review remediation: `eslint.i18n.config.mjs` applies the focused preview to
caller-supplied paths, independent of `i18nGuardFiles`. Corrected the two system
design statements to match. `python3 scripts/list-docs.py validate`,
`python3 scripts/lint-spec-files.py --all`, and `git diff --check` exited 0.
