---
id: "01-mobile-hint"
title: "Gate composer focus hint on mobile"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMPOSER-FOCUS-HINT-001
acceptance_criteria:
  - AC-UI-COMPOSER-FOCUS-HINT-001.1
  - AC-UI-COMPOSER-FOCUS-HINT-001.2
  - AC-UI-COMPOSER-FOCUS-HINT-001.3
system_design:
  - ../../specs/ui/system-design/composer-focus-hint.md
---

# Task 01: Gate Composer Focus Hint on Mobile

## Summary

Suppress hint and reserved padding together below 768px. Keep desktop
eligibility and phone editing functional.

## In scope

One responsive presentation predicate and focused component/browser coverage.

## Out of scope

Executor files, new copy, keyboard binding changes, and composer redesign.

## Acceptance

- Mobile component regression fails before implementation and passes after it.
- Hint and padding disappear together on phone and recover on eligible wider
  viewports; draft survives resizing.
- Phone slash-command and submission flow and desktop hint behavior pass.

## ASCII UI preview

UI-01: Empty, unfocused shared composer. Hint text is illustrative English;
existing localization remains authoritative. Toolbar structure is unchanged.

```text
Desktop >=768px: [ Write a message...          / to focus ]
Phone before:   [ Write a message...  / to focus ]
Phone after:    [ Write a message...            ]
                [ existing toolbar      Send   ]
```

The phone removes both hint and its reserved space (AC-001.1). Desktop keeps
existing eligibility (AC-001.2). The editor remains mounted across resize
(AC-001.3). This is a local region preview, not a pixel layout specification.

See the [full plan](plan.md#ascii-ui-preview).

## Verification

Run tests red first, apply the minimum fix, then run these checks sequentially.
Install workspace dependencies from `apps/` with `pnpm install --frozen-lockfile`
first if this checkout lacks them. Do not overlap browser suites.

```sh
(cd apps/web && pnpm exec vitest run components/task/chat/chat-input-body.test.tsx components/task/chat/use-chat-input-container.test.ts)
(cd apps/web && pnpm exec eslint components/task/chat/chat-input-body.tsx components/task/chat/chat-input-body.test.tsx)
(cd apps/web && pnpm e2e:run --project=mobile-chrome mobile-slash-command-composer.spec.ts)
(cd apps/web && pnpm e2e:run --project=chromium slash-command-composer.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/chat-input-body.tsx`
- `apps/web/components/task/chat/chat-input-body.test.tsx`
- `apps/web/e2e/tests/chat/mobile-slash-command-composer.spec.ts`
- `apps/web/e2e/tests/chat/slash-command-composer.spec.ts`

## Dependencies

None.

## Risks

Assert both hint and padding. Reset responsive mocks between cases. Test
767px and 768px, and preserve desktop eligibility without widening scope.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/composer-focus-hint.md)
- [System design](../../specs/ui/system-design/composer-focus-hint.md)
- Existing `shouldShowChatFocusHint` tests and mobile slash-command fixture.

## Results

Completed on 2026-09-17.

- RED: the new 393px and 767px component cases failed because the hint was
  visible; 36 other tests passed before the production change.
- GREEN: the two focused Vitest suites passed, 38 tests total. Coverage includes
  767px/768px boundaries, hint and padding changes in both resize directions,
  editor identity, draft preservation, and existing state eligibility.
- ESLint passed with no warnings for the two component files and both modified
  browser specs. Prettier was applied to changed code.
- Managed production-build E2E: mobile-chrome passed 1 test; chromium passed
  4 tests. Phone typing, slash-command selection, and sending work; desktop
  hint visibility and focus dismissal work.
- The phone test passed again with `--no-build` to regenerate its screenshot
  after the desktop runner replaced the test output directory. Visual inspection
  confirmed the hint is absent, editing space is available, and toolbar/send
  controls remain visible. This reused the freshly verified production build.
- Specification catalog validation, full specification lint, and `git diff
  --check` passed. The specification linter's 36 tests passed during design.

The initial browser attempts could not find Go. These successful E2E commands
used `PATH=/usr/local/go/bin:$PATH` in this Pod. No production workaround or
runner changes were needed. Unit tests supplied the defect-specific RED gate.

Public docs need no change: keyboard bindings, labels, and documented workflows
are unchanged. Internal requirements, design, and delivery records are updated.

Fresh synthetic desktop and phone PR screenshots were captured and visually
checked. Both capture runs passed. GitHub access and the user-approved commit
identity were restored for publication.

## Review follow-up

Greptile identified that forced component rerenders could mask a broken
viewport subscription. Responsive cases now use native Happy DOM viewport
changes without parent rerenders. All 38 focused tests pass; disabling the
subscription makes the boundary-transition cases fail. ESLint passes.
Production behavior and screenshots are unchanged.
