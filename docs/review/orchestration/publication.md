# Private publication receipt

Date: 2026-09-17. Repository:
[Corey-Fogg/kandev-orchestration](https://github.com/Corey-Fogg/kandev-orchestration).
Independent private repository, preserving public upstream ancestry through exact
v0.94.0 (`bf819a0228e742d069c528293d848c985a4d1bd1`).

## Publication boundary

All existing functional changes, specifications/plans and review records are
included. Original private local snapshot/backup commits are excluded from new
ancestry. Production source is compared byte-for-byte with the original integrated
prototype; deliberate differences are generic test fixtures and sanitized or
expanded documentation. Local backup/configuration/database/log artifacts are
excluded. The older public synthetic media remains linked from the review packet.

The test fixture changes use a fictional issue domain/IDs and a fictional project
name while retaining the same assertions. No application behavior is implemented
or changed by the remaining-work design package.

## Checks and delivery

Pre-publication checks on the clean import:

- Compared 478 original paths with renames represented as delete/add pairs:
  455 identical, two deliberately generalized test fixtures and 21 documentation
  revisions. No unexpected production-source change or extra source file.
- Scanned 506 changed/new files with Gitleaks 8.30.1. Ten generic-key findings
  were reviewed: all are synthetic database fixture `key_value` identifiers
  already present in public v0.94.0. Zero confirmed credentials. No scanner
  exclusions were added. A separate identifier/path review found only a generic
  upstream configuration IP example.
- Targeted Markdown rendering tests: one file, two tests passed. Targeted Office
  conversation-context regression passed. No broader production suite was rerun
  for unchanged source; the historical rebase evidence remains separately labeled.
- Specification linter: 30 tests and all specs passed. Harness linter: 19 tests
  and all 186 harness files passed. Checked 39 current package/review Markdown
  files: no broken relative links, all 74 requirement/criterion IDs resolved and
  covered. All 22 work-order dependency graphs have valid references/waves.
  Public-doc validation passed 61 tests and all 48 published pages. Whitespace passed.
- Code commit: `3536e04cae0eef2bb106dae1a1eca1ea4d1be7ae`; direct parent is the exact v0.94.0 release.
  Normal hooks passed (harness, architecture, gofmt, Go lint, Prettier, frontend
  lint, i18n, E2E sleep guard, public copy and commitlint). Specification hook
  correctly skipped this source-only commit; specs were checked directly above.
  Both hooks were active and no bypass was used. Source comparison/scanner logs
  and the full hook receipt remain local and private.
- Documentation commit and exact remote SHA verification follow this receipt;
  final delivery details will be recorded after the private push.

Inherited GitHub Actions are disabled; local pre-commit and commit-msg hooks are
installed. No release secret, production dataset, new public comment/PR or live
deployment is part of this publication.
