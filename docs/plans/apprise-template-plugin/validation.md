# Apprise template plugin design validation

## Scope

This record covers the design package, not implementation readiness. All code,
package, import, UI, upgrade, and recovery checks in the work orders remain
unexecuted. Maintainer split-direction approval and operator Q1-Q8 agreement
resolve the earlier discussion blocker. Tasks 01-05 are pending implementation
in dependency order; this document does not claim any runtime change.

## Evidence

- Research worktree: branch `docs/apprise-template-plugin`, v0.99.0 source
  `f0e2e774cfe2df8a0d15e4c21dea7b48580e38fb`, clean before document creation.
- Fetched upstream main: `08a36b3fe0d49e1dd23007f30fc82ac426a2c1c8`.
  Implementation must refresh/reconcile main; no production code was changed.
- Read the original issue and all three comments. Maintainer feedback on the
  original feature: <https://github.com/kdlbs/kandev/issues/5011#issuecomment-6600724735>.
- Posted and read back the revised split proposal:
  <https://github.com/kdlbs/kandev/issues/5011#issuecomment-6682190312>.
- Subsequently read and verified the split-direction approval:
  <https://github.com/kdlbs/kandev/issues/5011#issuecomment-6683963471>.
  The maintainer invites two contributions, API review in those contributions,
  and desktop/mobile screenshots. Do not retain the old discussion blocker.
- Grilling Q1-Q8 confirms the repositories/PR targets, latest-main contribution,
  latest-stable candidate/operational upgrade, six-event scope, preparation before
  activation, coherent backup/compatible rollback, and immutable Save/CAS behavior.
- Latest stable observed during grilling: v0.99.1, commit
  `813a741b21108ea8ed10ab58954d85c2e5205999`; main
  `5264536ba9845f966966c2a371c7e8136ccdf119`. Public SDK 0.1.0 differs from the
  unreleased main 0.2.0 metadata. Re-resolve inputs before implementation/rollout.
- Read the live upstream CONTRIBUTING.md and the release worktree version;
  both require architectural discussion before implementation/upstream PR.
- Exact SDK/proto, notification service, runtime-batch, webhook, and action-scope
  source reads support the design's boundary findings. No LSP call graph or
  runtime experiment was used to establish those static contract observations.
- Read the official plugin template Makefile through GitHub. It defines
  `check-format`, `test`, `vet`, `build`, `verify-package-host`, `sdk-check`, and
  `verify-package`. Pin the template source at later bootstrap rather than
  treating the moving default branch as a reproducible artifact.

## Checks

Observed on 2026-11-06:

| Check | Result |
| --- | --- |
| `python3 scripts/list-docs.py validate` | Passed: 432 decisions and 1,922 specifications validated. |
| `python3 scripts/lint-spec-files.test.py` | Passed: 36 tests. |
| `python3 scripts/lint-spec-files.py --all` | Passed. |
| `python3 scripts/list-docs.py specs --text notification-formatter --format paths` | Both new requirement/design paths discovered. |
| Local `validateCoverage` from `.github/scripts/pr-docs.cjs` | Passed: four work orders, all declared requirement/AC/design/plan references covered, zero errors. A synthetic runtime filename forced the structural validation branch; no runtime file was created or changed. |
| Local relative-link and whitespace validation | Passed: 17 local links and ten new files; no missing targets or trailing whitespace. |
| Documentation `style-lint.mjs` across ten files | Zero ERROR. Warning and informational dispositions below. |
| `git diff --check` and `git status --short --branch` | Passed for tracked changes; status identifies only the ten new documentation files in the design worktree. Untracked contents also received the explicit whitespace check above. Canonical checkout remains clean on main. |

Style warning dispositions: requirement `shall` statements follow the repository's
explicit normative requirement format; the posted issue proposal intentionally
uses first-person correspondence and conditional future behavior because it is
a proposal, not shipped API documentation; UI-01 preserves the proper names
Notifications and Apprise. Informational passive-voice and serial-comma heuristics
were reviewed; they do not change the specified contracts. No linter error was
waived. The issue proposal file retains the text posted upstream.

These are document checks. They do not satisfy any product implementation,
package, import, upgrade, browser, or recovery acceptance criterion.

## Decision reconciliation validation

Reran catalog validation (432 decisions and 1,922 specifications), all spec lint,
and the 36 spec-linter tests: passed. The repository's `validateCoverage` function
accepted all five work orders with zero errors under a synthetic runtime-path
preflight; no runtime file was created. All 18 local links resolve across the
eleven-file package; whitespace checks passed. Style lint reported zero ERROR;
normative wording and the historical correspondence retain the dispositions above.
The plan's Q1-Q8 section and Task 05 record the confirmed operational scope.
All product tests and operational phases remain unexecuted.

## Delivery status after grilling

The design package remains unstaged/uncommitted for review. No new plugin
repository, implementation PR, merge, release, production settings change, or
operational upgrade is claimed. The design worktree is retained for review.
The operator has authorized the future guarded upgrade, including a minimal
patched latest-stable build when necessary. Authorization does not establish
that the upgrade or any implementation check has occurred.
