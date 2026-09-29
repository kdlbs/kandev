---
id: "05-docs-e2e-and-verification"
title: "Document and prove the complete workflow"
status: done
wave: 5
depends_on:
  - "04-capture-screenshot-regions"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-001
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-003
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.7
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.6
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.5
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
---

# Task 05: Document and Prove the Complete Workflow

## Summary

Exercise the completed workflow through real browser and container-backed paths,
publish concise user guidance, update artifact statuses and results, and run the
requested repository checks before commit.

## In scope

- A desktop Chromium E2E that reviews multiple routes on a local development
  page, selects script-generated text and verifies its rendered DOM position,
  verifies the element candidate hint before selection, saves all three capture
  kinds, closes/reopens the preview, sends direct and queued feedback, and
  verifies the transcript and real image attachment.
- A mobile Chrome E2E for inline HTML preview, Drawer interaction, text or
  element capture, touch screenshot region, source return, 44-pixel controls,
  safe containment, and no horizontal overflow.
- A `containers` project E2E that launches a Docker task session, opens its
  published HTTP port in Browser, saves feedback, and delivers it.
- Replacement or removal of the skipped Browser-local annotation suite so no
  obsolete local-only contract remains as dormant coverage.
- `docs/public/developer-tools.md` guidance for capture, pending ownership,
  navigation, Send, screenshot limits, recovery, and raster limitations.
- Public-doc validation, specification lifecycle/status updates, work-order
  results, final repository checks, and a Conventional Commit.

## Out of scope

- Exhaustive executor duplication after one real Docker proof and existing
  shared port-proxy contract coverage.
- New screenshots or diagrams for public docs unless rendered review shows that
  prose and the existing UI are insufficient.
- Unrelated broad refactoring discovered during final checks.

## Acceptance

- Desktop, phone, and Docker-backed flows demonstrate the same durable pending
  set and ordinary Send outcome through their native compositions.
- Public guidance accurately distinguishes durable feedback from ephemeral
  preview execution and explains confirmed screenshot limitations and recovery.
- Focused checks, `make fmt`, and `make typecheck test lint` pass before the
  implementation is committed.

## TDD sequence

1. Write the desktop, mobile, and container E2E assertions against the completed
   product flow; confirm each new scenario fails for the expected missing or
   incorrect behavior before making its final fixture/wiring adjustment.
2. Make the minimum product or fixture fixes needed for first-attempt GREEN;
   avoid retry-based assertions and fixed sleeps.
3. Update public guidance and durable artifact statuses/results, run their
   validators, then run the E2E files again after the production rebuild.
4. Run the requested repository-wide checks once after focused checks settle.
   Fix only causal failures, rerun affected focused checks, then rerun the failed
   broad command before committing.

## Verification

```bash
make -C apps/backend build
cd apps/web
pnpm run build:e2e
pnpm e2e:run --project chromium tests/preview/preview-feedback.spec.ts
pnpm e2e:run --project mobile-chrome tests/preview/mobile-preview-feedback.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/docker/preview-feedback.spec.ts
cd ../../
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
make fmt
make typecheck test lint
```

## Files likely touched

- `apps/web/e2e/tests/preview/preview-feedback.spec.ts`
- `apps/web/e2e/tests/preview/mobile-preview-feedback.spec.ts`
- `apps/web/e2e/tests/docker/preview-feedback.spec.ts`
- `apps/web/e2e/tests/preview/preview-annotations.spec.ts`
- `apps/web/e2e/pages/` and focused preview helpers/fixtures.
- `docs/public/developer-tools.md`
- `docs/specs/tasks/requirements/web-preview-feedback.md`
- `docs/specs/tasks/system-design/web-preview-feedback.md`
- `docs/specs/ui/requirements/native-html-preview.md`
- `docs/specs/ui/system-design/native-html-preview.md`
- `docs/decisions/2026-09-15-task-owned-web-preview-feedback.md`
- `docs/plans/web-preview-feedback/`

## Dependencies

- Tasks 01 through 04 supply the complete persistence, delivery, responsive
  capture, and screenshot behavior.

## Risks

- Container setup can dominate test time; the test must reuse the standard
  Docker fixture and avoid adding a new image.
- An E2E can accidentally inspect a fixture-local panel state instead of
  proving backend durability; reload and a second view must read the server
  snapshot.
- The managed E2E runner requires fresh backend, mock-agent, agentctl, and web
  artifacts before it starts.
- Broad checks can expose unrelated pre-existing failures. Record the command,
  causal evidence, and narrow follow-up instead of weakening the test.

## Parallelism

`sequential`

## Inputs

- All acceptance criteria and the complete ASCII preview in `plan.md`.
- Current public HTML-preview guidance and E2E runner requirements.

## Results

- Added a desktop Chromium scenario for multi-route text, element, and region
  capture; generated-text position evidence; candidate hinting; durable reopen;
  direct and queued Send; transcript rendering; and screenshot attachment
  delivery.
- Added mobile Chrome coverage for the inline HTML preview Drawer, touch
  capture, source return, touch target sizing, containment, and overflow, plus
  a Docker-backed Browser scenario for a published local HTTP page.
- Removed the obsolete Browser-local annotation E2E and documented task-owned
  pending feedback, navigation, delivery, limits, raster behavior, and recovery
  in `docs/public/developer-tools.md`.
- Verified rebuilt host and Linux backend artifacts, the E2E web bundle, all
  three focused Playwright scenarios, public docs, specs, CLI, 88 focused web
  tests, the affected backend packages, and the full 2,095-file web suite with
  18,041 passing tests and 4 intentional skips.
- The full backend suite passes every package except the unchanged real-process
  probe integration in the elevated runner, whose host PID namespace conflicts
  with container `/proc/uptime`; that package passes 20 consecutive runs in the
  normal namespace. Synthetic Git fixtures now set repository-local empty hook
  paths and remain independent of machine-wide Git policy.
