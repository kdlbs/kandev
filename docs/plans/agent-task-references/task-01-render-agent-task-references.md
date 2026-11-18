---
id: "01-render-agent-task-references"
title: "Render agent task references"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-AGENT-TASK-REFERENCE-001
acceptance_criteria:
  - AC-UI-AGENT-TASK-REFERENCE-001.1
  - AC-UI-AGENT-TASK-REFERENCE-001.2
  - AC-UI-AGENT-TASK-REFERENCE-001.3
  - AC-UI-AGENT-TASK-REFERENCE-001.4
  - AC-UI-AGENT-TASK-REFERENCE-001.5
  - AC-UI-AGENT-TASK-REFERENCE-001.6
  - AC-UI-AGENT-TASK-REFERENCE-001.7
system_design:
  - ../../specs/ui/system-design/agent-task-references.md
---

# Task 01: Render agent task references

## Summary

Implement explicit agent task links as inline chips with direct task navigation.
Use TDD for recognition, renderer composition, and prompt guidance. Add desktop
and phone evidence in the same vertical slice.

## In scope

- Strict link recognition, shared chip presentation, agent reply/plan wiring.
- Desktop disclosure from loaded state, accessible anchors, mobile geometry.
- Task and Office prompt guidance, translations, focused tests, and public docs.

## Out of scope

New routes, URI schemes, task APIs, metadata requests, lifecycle changes,
composer behavior, and shared-export decoration.

## Acceptance

1. Eligible agent links follow UI-01/UI-02 and open the exact task through
   `TaskLink`, preserving navigation guards and browser history.
2. Recognition, streaming, metadata fallback, raw/copy, and existing user/file
   links satisfy the paired requirement without storage or permission changes.
3. Prompt, unit, and desktop/mobile tests pass. Rendered evidence proves themes,
   keyboard behavior, 44px phone targets, and containment. Public docs describe
   the shipped interaction without claiming that all agent output is recognized.

## UI preview

See the full [UI-01, UI-02, and UI-03 descriptions](plan.md#ui-preview).

UI-01 places a task-icon/title chip inside an agent sentence. Desktop hover or
focus shows status and session badges, creation date, workflow, and parent.
The chip uses Kandev's primary accent. Click and Enter navigate directly.
UI-02 uses one wrapping, at least 44px-high chip on phones. A single tap opens
the task without an intermediate surface. UI-03 retains the authored label
when metadata is missing. These views cover AC .1-.6.

## Verification

Run from the repository root. In a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)`.
Use the managed E2E runner sequentially so builds and worker limits apply.

```bash
(cd apps/backend && go test ./internal/sysprompt)
(cd apps/web && pnpm exec vitest run lib/tasks/agent-task-reference.test.ts components/task/chat/messages/agent-task-reference-components.test.tsx hooks/domains/task/use-task-reference-details.test.tsx components/task/chat/messages/task-mention-components.test.tsx components/shared/markdown-components.test.tsx components/shared/memoized-markdown.test.tsx components/routing/task-link.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/agent-task-references.spec.ts tests/chat/chat-task-mentions.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-agent-task-references.spec.ts tests/chat/mobile-chat-task-mentions.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before the first production edit, record failing tests for the missing behavior.
Add any newly extracted hook suite to the focused command. Check no metadata
request occurs on hover. Assert an in-page marker survives navigation and no
main-frame document request occurs. Inspect captures against UI-01/UI-02.
Run `.github/scripts/pr-docs.cjs` coverage validation on the final changed-file
set, including all linked package documents. Record actual commands/results.

## Files likely touched

- New `apps/web/lib/tasks/agent-task-reference.ts` and `.test.ts`.
- New `apps/web/components/task/chat/messages/agent-task-reference-components.tsx`
  and `.test.tsx`, plus a domain hook under `hooks/domains/task/` if needed.
- `apps/web/components/task/chat/messages/task-mention-components.tsx` and tests.
- `apps/web/components/task/chat/messages/agent-message-content.tsx`.
- `apps/web/components/task/chat/messages/agent-plan-message.tsx`.
- `apps/backend/config/prompts/kandev-context.md` and `office-context.md`.
- New `apps/backend/internal/sysprompt/task_references_test.go`.
- New desktop/phone E2E files named in the plan.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json`
  and generated pseudo resources as required by the existing tooling.
- `docs/public/tasks-and-workflows.md`, section "Open a task mentioned in chat".

## Dependencies

None. Existing task chips and `TaskLink` are already implemented.

## Risks

Markdown/file-link precedence, accidental eager subscriptions, and tooltip
trigger composition. Keep identity independent of labels and loaded metadata.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/agent-task-references.md).
- [System design](../../specs/ui/system-design/agent-task-references.md).
- Existing user task-mention tests and `markdown-math.spec.ts` agent fixture.
- Scoped frontend/backend guidance, `/tdd`, `/e2e`, `/mobile-parity`, and
  `/docs-maintainer` before the corresponding implementation work.

## Results

Implementation completed on 2026-11-16.

- Red: new route recognition returned null, reply/plan links lacked task icons,
  loaded metadata was absent, and task/Office prompts lacked reference guidance.
  The desktop browser tests also failed on missing task icons in replies and plans.
- Green: the targeted Vitest command passed 89 tests across seven suites. The
  final test-only type correction was followed by a passing six-test renderer run.
- `go test ./internal/sysprompt`: passed. Guidance is covered with and without
  bound tool packages. The existing prompt-size test now permits 300 additional
  bytes for the reference instructions without changing authorization guidance.
- Web typecheck, lint, and `i18n:check`: passed. All seven real locales and the
  generated pseudo catalog include the new copy.
- Go changed-package lint: passed with zero issues. The host lacked the binary,
  so `nix shell nixpkgs#golangci-lint --command` supplied it. Per-command Git
  configuration selected `origin/main` because `origin/HEAD` was absent.
- Managed Chromium: three tests passed, including existing user task mentions.
  Proved hover/focus/Escape, no hover metadata request, no navigation reload,
  rename and duplicate-title identity, missing destinations, expanded plan
  links, Back, and phone-width fine-pointer sizing.
- Managed mobile-chrome: two tests passed, including existing user task mentions.
  Measured at least 44px hit areas, wrapping/containment, nonoverlap, no horizontal
  overflow, direct touch navigation, and Back. Phone capture was inspected.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- Final catalog validation, specification lint, 36 specification-linter tests,
  document links, whitespace checks, and actual changed-file PR documentation
  coverage all passed. The final coverage result is `covered` with no errors.
- No model delegation, task creation, staging, or commits occurred.

### Host browser commands

The first build attempt became stale after a prompt edit. The next attempts
exposed inherited launcher configuration and missing Chromium shared libraries.
The successful runs used isolated fixture data, removed only inherited config
paths for the command, and used the host's existing browser library bundle.
The frontend was rebuilt with `pnpm --dir apps/web run build:e2e` after final
production edits. Subsequent `--no-build` runs reused that build and the already
fresh managed backend/fixture builds.

```bash
env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE KANDEV_SERVER_HOST=127.0.0.1 LD_LIBRARY_PATH=/nix/store/vj1w29j7kfsvn83z33yb2rpq4ymz24n5-kandev-builtin-playwright-libs/lib CAPTURE_PR_ASSETS=1 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium tests/chat/agent-task-references.spec.ts tests/chat/chat-task-mentions.spec.ts
env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE KANDEV_SERVER_HOST=127.0.0.1 LD_LIBRARY_PATH=/nix/store/vj1w29j7kfsvn83z33yb2rpq4ymz24n5-kandev-builtin-playwright-libs/lib CAPTURE_PR_ASSETS=1 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/chat/mobile-agent-task-references.spec.ts tests/chat/mobile-chat-task-mentions.spec.ts
GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=kandev.lintBaseRef GIT_CONFIG_VALUE_0=origin/main nix shell nixpkgs#golangci-lint --command bash scripts/lint-go-changed apps/backend/internal/sysprompt/mcp_discovery_test.go apps/backend/internal/sysprompt/task_references_test.go
```

The Nix store path is specific to this host. Other hosts should use their normal
Playwright dependencies. Browser captures live in ignored `apps/web/.pr-assets`.
The runner clears that directory before each invocation, so the desktop capture
was repeated after preserving the inspected phone capture in `/tmp`. The repeated
desktop run passed both tests. Desktop light/dark and phone images were inspected
against UI-01/UI-02. No actionable chip layout or interaction issues remained.


Design-package validation on 2026-11-16:

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans`: passed. A separate check read
  all four untracked files and found no trailing whitespace or missing links.
- `.github/scripts/pr-docs.cjs::validateCoverage`: the actual documentation-only
  change is exempt. A prospective check with the planned agent-message source
  path as a runtime trigger passed and resolved the work order, plan,
  requirement, acceptance criteria, and design. No production file was edited.
- Catalog queries discover both new specification files.
- At the design checkpoint, browser and product tests were pending. Implementation
  results above supersede that checkpoint.

### Playground feedback: accent and hover details

The sidebar retained task records in `taskOverview.byId`, but the original
preview read only board collections. A focused hook test and a browser
assertion reproduced the missing description before the fix.

The preview now reads the canonical overview. It shows cached status,
workflow, current title after a rename, and a three-line description excerpt.
Shared task chips use primary-accent and focus-ring tokens in both themes.
No task fetch, subscription, or mobile disclosure was added.

Validation: 37 focused frontend tests, three desktop browser tests, and two
phone browser tests passed. Browser coverage includes existing user mentions.
Typecheck, changed-file lint, specification validation, and the production web
build passed. Light, dark, and phone rendering were inspected. The isolated
playground on Tailscale port 48560 serves the new bundle. Port 9998 remains on
its original process.

### Rich metadata preview

The hover card now shows status and session-count badges, a localized creation
date and time, workflow, and parent. Parent names use the canonical task cache
with an exact-ID fallback. Missing counts and invalid timestamps stay absent.
The description remains a three-line excerpt of the stored description field,
not a generated summary. Existing translations cover all labels and plurals.

Hook coverage checks parent lookup, cache removal, and invalid timestamps.
The desktop fixture includes a parent and two distinct completed sessions.
A browser regression failed on missing metadata before implementation, then
passed with badge, date, parent, and description assertions. Final results:
27 focused frontend tests, two desktop browser tests, and one phone browser
test passed. Typecheck, changed-file lint, translation checks, specification
validation, formatting, and the production build passed. Light and dark cards
were inspected on the refreshed playground at port 48560. Main port 9998 was
not changed.

### Remove description from the preview

Per user feedback, the hover card now omits the description. Status and session
badges, creation date, workflow, parent, and title remain. This supersedes the
excerpt presentation recorded above. Updated the existing browser assertion
to require description absence. The focused desktop test, production build,
changed-file lint, and specification lint passed. The playground was refreshed
and its tooltip was checked through the Tailscale URL.

### Personal task colors and PR validation

References and user mention chips now read the referenced task's persisted
personal color through `useTaskColor`. All seven personal colors have light
and dark variants. An unset or cleared color uses the primary accent.

The browser regression first failed because changing the sidebar color left
the reference unchanged. It now checks a sidebar Red selection, live Green
settings updates, clear-to-accent, reload persistence, and an unchanged sibling.
Phone coverage verifies a live color update before direct tap navigation.
On the isolated playground, sidebar Red, Green, and None selections updated the
reference immediately. Green persisted after reload. Sample tasks retain green,
orange, blue, and pink colors for manual inspection.

Final affected checks: 54 frontend tests, backend sysprompt tests, four desktop
browser tests, two phone browser tests, web typecheck, lint, translation checks,
and specification checks passed. Fresh light, dark, and phone captures cover
agent references and existing user mentions. No production data was changed.

### PR integration validation

PR #5361 initially inherited a failing bootstrap fixture from base `ea6af0d50`.
`TestImproveBootstrapUsesUpgradedInstructions` reproduced the missing workflow
step lister on that unchanged base. Merged upstream fix `8d30872f7` without
conflicts. The same test passed with `go test -tags fts5 -race -p 1
./internal/improvekandev -run '^TestImproveBootstrapUsesUpgradedInstructions$'
-count=1 -timeout=4m`.

After the merge, rebuilt the web, backend, and E2E helper binaries. Re-ran the
54 focused frontend tests, sysprompt tests, typecheck, translation checks, four
desktop and two phone browser tests, documentation catalog validation, and
specification lint. All passed. The task-reference implementation is unchanged.

### Compact phone references

User feedback replaced the 44px agent-reference minimum with a compact 24px
minimum and snug line height. Existing user-mention sizing is unchanged.
The updated mobile regression checks content-fit height, wrapping, and direct
tap navigation. Both phone browser tests, 16 component tests, typecheck,
production web build, and specification lint passed.

### Merge-readiness review

Updated the remaining narrow-desktop assertion to the approved 24px contract.
Keyboard coverage now reaches the link with Tab and scopes its tooltip to the
reference title. All three desktop reference tests passed. Recorded the compact
inline-link exception in the mobile sizing guide. Harness and spec validation
passed. The parent-ID fallback remains the specified cache-only behavior.

### Complete Claude suggestions

Hide the parent row until its title is loaded, with a regression for unavailable
and loaded parents. Resolve known workflow IDs without scanning task snapshots;
memoize legacy fallback lookup over source slices. Cover keyed-snapshot workflow resolution alongside the existing board tests. Clarify in the PR body that uncolored existing
user mentions also use the primary accent. The keyboard, compact sizing, and
stale viewport assertions were already addressed in the preceding commit.

Validation: five hook/metadata regressions, all three desktop browser tests,
web typecheck, production build, catalog validation, and spec lint passed.

### CI session-recovery alignment correction

E2E shard 9 failed in the base branch's mobile progress-alignment test. A held
message-list response did not identify an automatic retry: background history
reads can also be held, while core recovery still displays Loading conversation.
Merged the current base without conflicts to reproduce the combined code.

The desktop and phone alignment tests now hold initial loading, inject a history
failure, and click the visible Retry action before measuring manual core recovery.
They assert the existing loading status for this core-owned path, release every
held response in finally blocks, and verify completion. Production behavior is
unchanged. The phone alignment check passed three consecutive runs with retries
disabled. All eight desktop recovery/reference tests and typecheck passed.
All four phone recovery/reference tests also passed after the correction.
