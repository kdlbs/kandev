---
id: "04-project-creation"
title: "Project creation UX and initial prompt"
status: complete
wave: 4
depends_on:
  - "03-editor-feedback"
plan: "plan.md"
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-001
  - REQ-PROJECTS-AGENT-PROJECTS-005
acceptance_criteria:
  - AC-PROJECTS-AGENT-PROJECTS-001.1
  - AC-PROJECTS-AGENT-PROJECTS-001.2
  - AC-PROJECTS-AGENT-PROJECTS-001.5
  - AC-PROJECTS-AGENT-PROJECTS-001.6
  - AC-PROJECTS-AGENT-PROJECTS-001.7
  - AC-PROJECTS-AGENT-PROJECTS-001.8
  - AC-PROJECTS-AGENT-PROJECTS-001.9
  - AC-PROJECTS-AGENT-PROJECTS-005.2
system_design:
  - ../../specs/projects/system-design/agent-projects.md
---

# Task 04: Project Creation UX and Initial Prompt

## Summary

Make creation familiar to New Task and allow the initial coordinator prompt in
the same form. Reuse repository/session/message contracts and preserve retries.

## In scope

- Own the Project form dialog, fields, controller, focused helper tests, and
  necessary shared remote picker extraction under `apps/web/components/`.
- Reuse the New Task provider picker, identity, search/paste, and inspection
  behavior without importing workflow or branch-policy controls.
- Own the metadata-only task-service remote-selection inspect/register routes
  and backend application provider adapters. Reuse existing provider clients,
  server plugin inspection, trusted descriptor validation, and the repository
  identity lock. Keep generic repository creation unchanged. Enforce workspace
  read access for inspection and `repository.manage` for registration.
- Verify built-in and plugin identities server-side. Treat submitted metadata
  as hints; persist the verified credential-free clone URL and actual default
  branch. Support exact GitHub repositories beyond the initial catalog for
  personal and organization owners, configured self-managed GitLab origins,
  Azure project scope/IDs/full paths, and plugin clone paths. Preserve provider
  visibility and workspace repository scope. Reject mismatches before writes.
- Resolve selected remote records through these repository APIs, preserve
  provider host/scope/ID, cache successful imports, upsert workspace state, and
  apply `lib/agent-projects/repositories.ts` eligibility. Support existing records
  and new selections without a separate Settings step.
- Match the desktop 900px geometry and spacing of New Task; provide the focused
  phone form, stacked fields, wrapping chips, fixed safe-area footer, and 44px targets.
- Keep the form/controller host mounted across responsive navigation changes.
  Preserve open state, draft, and in-flight identity when a phone rotates across
  the desktop breakpoint. Cover both directions with a browser regression.
- Add name and optional multiline prompt, coordinator and primary selectors,
  collapsed advanced worker selectors, inherited defaults, and execution info.
- Preserve explicit profile choices on edit, missing-dependency recovery, and
  explicit workspace-default executor repair.
- Add localized project/profile help with hover/focus and coarse-pointer tap drawers.
- For a nonblank prompt, create the project, ensure its coordinator session via
  the existing session API, and use `hooks/message-request.ts` with a stable caller
  ID. Preserve exact message/session identity during retry and reconcile uncertain
  responses. This starts direct user input through the existing message path.
- Cache a successfully created project before messaging. On messaging failure,
  retain draft and display Retry start and Open coordinator. Retrying must not
  create a second project or import repositories again. Prevent request payload
  changes while retry identity is unresolved. Blank prompt skips agent start.
- Extend desktop/mobile Projects E2E for prompt/no-prompt, repository selection,
  defaults/overrides, primary removal, help, error/retry, and control geometry.
- Update public creation guidance and all supported locale catalogs.

## Out of scope

- A new project start API, backend startup state, worker-launcher reuse, or DB columns.
- New workflow/branch controls, remote executor support, archived project navigation.
- Commit, push, root task-plan edits, or additional agents.

## Acceptance

The approved `UI-03`/`UI-04` preview in [plan.md](plan.md#ascii-ui-preview) is
implemented with the existing visual system. Creation works from an empty remote
repository list; blank prompts remain idle and nonblank prompts appear exactly
once in the coordinator. Failed message submission retains a usable saved project
and prompt. Profile inheritance, edit preservation, repository eligibility,
responsive containment, keyboard focus, and touch help have regression evidence.

## Verification

Use TDD for controller/repository/profile/message logic. Add tests under
`components/agent-projects/` or the relevant hook/helper beside production code.
Include uncertain transport, retry payload identity, partial repository failure,
and post-create message failure. Preserve existing message admission tests.
Add backend tests for inspection without writes, registration without clone/task
side effects, concurrent retry deduplication, authorization, scoped identity,
canonical URL/default branch, mismatched hints, unavailable providers, and the
unchanged generic clone-URL boundary. Exercise personal-owner GitHub lookup
outside the initial catalog, self-managed GitLab, Azure, and plugin descriptors.
Run the affected Go service/handler/backend-application/provider packages and
backend lint. Extend the shared New Task picker/identity tests for compatibility.

Run from the repository root:

```bash
pnpm --dir apps/web exec vitest run components/agent-projects hooks/domains/agent-projects lib/api/domains/agent-projects-api.test.ts hooks/use-message-handler.test.ts components/task-create-dialog-handlers.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/projects/agent-projects.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/projects/mobile-agent-projects.spec.ts
git diff --check
```

Lint every changed TS/TSX source/test with the local eslint command. Run browser
commands sequentially against managed fresh builds. Capture desktop creation,
advanced settings, picker, phone creation, and tap-help screenshots for coordinator
inspection. Record exact paths and measured target/viewport/footer geometry.

## Results

The project form, remote repository selection/inspection, prompt-start flow,
responsive host, provider adapters, translations, and public creation guidance
are implemented. The QA-30 regression preserves ordinary plugin preflight
behavior through `TestCreateTaskPreflightsPluginRepositoryBeforePersistence`.

Passed checks:

- Focused controller, picker, repository hook, context format/resolution, and
  shared New Task picker Vitest: 7 files, 86 tests.
- Web typecheck, full changed-file ESLint (`--max-warnings 0`, log:
  `/tmp/projects-3920-frontend-final-lint-fixed.log`), `i18n:zh-hant`,
  `i18n:check`, and `i18n:ratchet`.
- Eight affected Go packages, plus the remote-registration concurrency test
  under `-race`.
- Public docs validation tests (62/62), validation of all 47 published pages,
  and `git diff --check`.
- Root-owned backend lint/build and isolated API/SQLite validation passed;
  receipts are in `/tmp/projects-3920-live-repository-results.json` and the
  root QA report.
- Deferred forced-refresh regression was first Red (the queued refresh did not
  settle after a competing refresh), then Green after releasing the queued
  marker before the queued continuation. `pnpm vitest run
  hooks/domains/agent-projects/use-agent-projects.test.tsx`: 3/3; focused
  ESLint on the hook, test, and mobile Projects spec passed. This also resolved
  the mobile archive drawer remaining in `Saving…` after the archive endpoint
  returned HTTP 200.
- Desktop managed Projects E2E: 7/7 passed with
  `pnpm e2e:run --host --shards 1 --project chromium
  tests/projects/agent-projects.spec.ts` (session 57878).
- Mobile managed Projects E2E: 2/2 passed with
  `pnpm e2e:run --host --shards 1 --project mobile-chrome
  tests/projects/mobile-agent-projects.spec.ts` (session 96458). The archive
  lifecycle check restored through the API, bounded retries to the specific
  cleanup-cancellation race, and verified context while archived and after
  restore. It also confirmed the coordinator's worker-creation reply and an
  agent-authored worker message before archiving.

CI correction QA-36 also passes. Closed forms now return a stable empty
repository snapshot before loading or without a workspace. Three real-store
regressions fail before the repair and pass afterward. Focused lint, typecheck,
and formatting pass. Sequential fresh production browser gates pass ten desktop
and three phone cases, including ordinary Settings with Projects disabled.
No flag, UI, or API contract changed. Corrective logs and captures are retained
in `/tmp/projects-3920-startup-loop-fix/` and
`/tmp/projects-3920-startup-loop-e2e/`.

CI correction QA-37 also passes. The shared mobile repository provider tabs
retain 44px targets and use a non-layout divider to prevent 1px clipping.
The final mobile GitLab case passes 1/1 and the focused picker file passes 7/7.
Desktop provider switching passes 1/1 with 28px controls in its existing 32px
strip. Both device checks retain containment assertions and capture geometry.
Five provider utility tests, lint, typecheck, formatting, and the fresh managed
production build pass. The coordinator reviewed the diff and both settled
screenshots. Final evidence is in `/tmp/projects-3920-mobile-picker-fix/`.

Desktop evidence is preserved at
`/tmp/kandev-projects-refinements-evidence/task04-desktop-post-refresh-fix/`
and phone evidence at
`/tmp/kandev-projects-refinements-evidence/task04-mobile-post-refresh-fix/`.
Desktop includes default/advanced/picker, narrow-fine, and coarse-tablet
captures; phone includes form/help/sidebar/context and delete-confirmation
captures. The earlier mobile archive hang and its HTTP 200/session snapshot are
preserved at
`/tmp/kandev-projects-refinements-evidence/task04-mobile-archive-diagnosis/`.
