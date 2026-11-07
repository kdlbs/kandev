---
id: "02-runtime-version-tags"
title: "Exclude SDK tags from runtime build versions"
status: done
wave: 2
depends_on:
  - "01-portable-backup-filenames"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SQLITE-SNAPSHOT-001
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
acceptance_criteria:
  - AC-PLATFORM-SQLITE-SNAPSHOT-001.4
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.7
system_design:
  - ../../specs/platform/system-design/sqlite-snapshot-copy.md
---

# Task 02: Exclude SDK tags from runtime build versions

## Summary

Limit local runtime version defaults to Kandev release tags in both Makefiles.
Prove tag selection with disposable Git history before changing the defaults.

## In scope

- Extend the existing `scripts/check-make-shells` verification entrypoint.
- Probe root `RUNTIME_VERSION` and backend `VERSION` through real Make expansion.
- Filter default Git descriptions with `--match "v[0-9]*" --exclude "*/*"`.
- Preserve existing overrides, fallbacks, and shell compatibility.

## Out of scope

SDK releases, release-channel changes, installed binaries, and persisted version
metadata remain outside this task. Do not add tags to the working repository.

## Acceptance

1. A disposable repository with an older runtime tag and newer SDK tag exposes
   the current bug. After the fix, both Makefiles describe the runtime tag.
   Cover a namespaced SDK tag that begins with a version-like prefix.
   Assert fixture-derived results rather than duplicating the production command.
2. SDK-only history falls back to a commit hash. Dirty suffixes, explicit version
   overrides, and the `dev` fallback outside Git remain effective in both Makefiles.
   Exercise the fallback fixture under a known parent Git worktree.
3. Existing POSIX, Git Bash, and native Windows recipe simulations pass.
   Use double quotes around the tag pattern and preserve redirect handling.

## Verification

Run from the repository root. Run the new regression before production edits.

```bash
bash scripts/check-make-shells
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `Makefile`
- `apps/backend/Makefile`
- `scripts/check-make-shells`
- `docs/plans/upgrade-backup-version-paths/plan.md`
- `docs/plans/upgrade-backup-version-paths/task-02-runtime-version-tags.md`

## Dependencies

Task 01 must complete first. The historical database repair remains necessary
even after runtime builds stop selecting SDK tags.

## Risks

Native Windows shells do not treat single quotes as POSIX shells do.
Temporary Git fixtures need local identity configuration and guaranteed cleanup.
Avoid application builds or network calls in version-discovery tests.

## Parallelism

`sequential`

## Inputs

- [Snapshot requirements](../../specs/platform/requirements/sqlite-snapshot-copy.md)
- [Startup requirements](../../specs/platform/requirements/startup-lifecycle.md)
- [Stored version compatibility design](../../specs/platform/system-design/sqlite-snapshot-copy.md#stored-version-compatibility)
- Root `Makefile`: `RUNTIME_VERSION`
- `apps/backend/Makefile`: `VERSION` and `LDFLAGS`
- Existing disposable command recorders in `scripts/check-make-shells`

## Results

- **RED:** Before changing either Makefile, `bash scripts/check-make-shells`
  failed on the new Git-fixture assertions. With only runtime tag `v1.2.3`, both
  defaults expanded to `v1.2.3-1-gd4459d8`. After adding the newer
  `pluginsdk/v0.1.0` tag at `HEAD`, both returned `pluginsdk/v0.1.0` instead.
  SDK-only history likewise returned the SDK tag instead of fixture hash
  `f8c2b9a`, and dirty output became `pluginsdk/v0.1.0-dirty`. Explicit
  overrides and the outside-Git `dev` fallback passed before the fix.
- **GREEN:** After adding `--match "v[0-9]*"` to both defaults, the final
  `bash scripts/check-make-shells` run passed. Both defaults retained
  `v1.2.3-1-gcf2dcdb` after adding the SDK tag; SDK-only history returned
  `f6b225a`; dirty output was `v1.2.3-1-gcf2dcdb-dirty`. Explicit overrides and
  outside-Git `dev` fallback also passed. POSIX, Git Bash and native Windows
  recipe simulations passed. The native mkdir execution probe was skipped
  because `cmd.exe` is unavailable.
- `python3 scripts/list-docs.py validate` passed: 436 decisions and 1931
  specifications validated.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

Disposable commits use fixture-local identity, signing disabled, and an empty
fixture-owned hooks directory. All fixture files are removed by the harness
cleanup trap.

### PR remediation: slash-tag exclusion and Git ceilings

- **RED:** With a newer `v1/pluginsdk/v0.1.0` tag at `HEAD`, both Make defaults
  returned `v1/pluginsdk/v0.1.0` instead of runtime tag description
  `v1.2.3-1-g4ea4c60`.
- **RED:** Running the prior fallback fixture with `TMPDIR` under the delivery
  worktree made both defaults discover its parent repository and return
  `v0.99.1-8-g702ec0e691c-dirty` instead of `dev`.
- **GREEN:** Both Makefiles now exclude slash-containing tags. The fallback
  fixture sits beneath a tagged disposable repository; its positive control
  resolves the parent version, while `GIT_CEILING_DIRECTORIES` makes both
  defaults return `dev`. The final harness passed both with the default temp
  location and with `TMPDIR` under the delivery worktree. The latter resolved
  `v1.2.3-1-g3175a99`, used hash fallback `7192175` for SDK-only history,
  retained `-dirty`, excluded the `v1/` tag, and passed both `dev` fallbacks.
- The harness uses fixture-local Git identity, disables commit and tag signing,
  and points hooks to an empty fixture-owned directory. Its cleanup trap removes
  the temporary repositories. The native `cmd.exe` mkdir probe was skipped
  because `cmd.exe` is unavailable; shell recipe simulations passed.
- `python3 scripts/list-docs.py validate` passed: 436 decisions and 1931
  specifications validated.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
