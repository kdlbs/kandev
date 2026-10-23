---
id: "04-browser-coverage"
title: "Enforce and document browser coverage"
status: done
wave: 4
depends_on: ["03-composition-and-mobile"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.10
  - AC-UI-MARKDOWN-FILE-EDITING-001.11
  - AC-UI-MARKDOWN-FILE-EDITING-001.12
  - AC-UI-MARKDOWN-FILE-EDITING-003.6
  - AC-UI-MARKDOWN-FILE-EDITING-003.11
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 04: Enforce and document browser coverage

## Summary

Make the focused browser matrix reproducible in CI and document verified
browser behavior. Preserve the user's uncommitted testing guide and isolated
evaluation data while updating its runtime after successful implementation.

## In scope

- A dedicated Markdown CI job with actual Firefox and WebKit artifacts and
  required dependencies. Keep existing Chromium shards and concurrency guards.
- Regression coverage for project selection, install/build steps, and worker
  limits in the workflow/runner's existing test conventions.
- Public how-to updates in `docs/public/sessions-and-review.md`, E2E setup,
  browser coverage records, and accurate linked plan results.
- Update `TESTING-PR3113.md` while keeping it unstaged/uncommitted. Rebuild the
  isolated evaluation runtime at its direct Tailscale endpoint only after
  targeted checks pass. Preserve SQLite, fixture repositories, user edits,
  port, stop helper, and the main :9998 process.

## Out of scope

- Changing general suite shard policy, publishing a release, merge, commit,
  push, or claiming unavailable native-device checks passed.

## Acceptance

- CI selects and runs the focused real Firefox/WebKit tests with one worker
  per shard. The workflow contract tests prove installation and selection.
- Public instructions explain the working modes and actual failure recovery.
  They do not classify missing EditContext as unsupported-browser behavior.
  Coverage records identify automated, native-device, and screen-reader results.
- The isolated evaluation task serves the tested artifact, retains user data,
  and has a verified ownership-guarded stop command. The workspace guide stays
  uncommitted and names the deployed artifact and outstanding acceptance gaps.

## Verification

Run the existing workflow/runner contract tests affected by the CI job. Add a
focused Node test `e2e/scripts/markdown-browser-coverage.test.ts` for its project
selection and pinned CI commands. Commands start at the repository root.

```bash
pnpm --dir apps/web exec vitest run e2e/scripts/markdown-browser-coverage.test.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project markdown-firefox tests/task/markdown-file-editing.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project markdown-webkit tests/task/markdown-file-editing.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project markdown-mobile-webkit tests/task/mobile-markdown-file-editing.spec.ts
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Also rerun existing Chromium/phone Markdown files after any product changes.
Use the live evaluation task in Zen only after confirming its artifact identity.
Do not reset its repositories or recreate its database to obtain a clean demo.

## Files likely touched

- `.github/workflows/e2e-tests.yml` and focused workflow contract tests.
- `apps/web/e2e/scripts/markdown-browser-coverage.test.ts` (new).
- `apps/web/e2e/README.md`, `apps/web/AGENTS.md` if its input conventions change.
- `docs/public/sessions-and-review.md`.
- This package and linked historical plan status notes.
- `TESTING-PR3113.md` (must remain uncommitted).

## Dependencies

Task 03. All runtime changes and their task-defined checks must pass first.

## Risks

CI currently provisions Chromium, so adding project names without installing
native browsers would create false coverage. A new artifact must not replace
user fixture data. Native-device evidence can remain open after CI passes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [Validation strategy](../../specs/ui/system-design/markdown-file-editing.md#validation-strategy).
- Existing guarded E2E runner, CI provisioning, and evaluation ownership helpers.

## Results

The focused CI matrix and report/gate wiring are implemented. Its three
regression tests pass. External-runner workflow contracts pass all five tests.
The E2E workflow suite passes all 16 tests with a temporary fixture adapter
for this Nix host's missing `/bin/sleep`; the unchanged original command has
one environment failure at that hardcoded fixture path. No repository or host
system path was changed to obtain that result.

Public how-to instructions and E2E setup now describe scoped native input,
actual Firefox/WebKit installation, and separate physical-device acceptance.
The public-doc validator passes all 47 published pages. All 53 distinct browser
scenarios pass on the final artifact. Firefox and WebKit each pass 13 desktop
cases; Chromium combines 11 full-run passes with two focused passes after
replacing invalid native-element assertions with public model selection. Each
phone project passes seven cases. Firefox also passes the three final public
selection assertions in a focused rerun. No production change followed these runs.

The refreshed isolated runtime preserves its SQLite database, 66 fixture file
hashes, and fixture Git states. All four read-only live smoke checks pass:
desktop Chromium, Firefox, WebKit, and phone WebKit. Firefox and WebKit opened
rendered Edit at the same HTTP Tailscale origin with EditContext absent.
The ownership-guarded stop helper verifies the new evaluation PID 4087040;
the main :9998 listener remains PID 1155. The workspace guide is untracked.
Asset SHA-256:
`af406b0332755f284e75c0953116faa890e4334d9691f99908e0f60f57146984`.

Native OS IME, physical Android/iOS, Zen/macOS, Safari/macOS, NVDA, and
VoiceOver checks remain explicitly pending in Task 03. They are not claimed
as passing from browser automation. No commit, push, or GitHub CI run occurred.
