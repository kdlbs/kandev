---
created: 2026-09-17
status: done
requirements:
  - REQ-UI-COMPOSER-FOCUS-HINT-001
system_design:
  - ../../specs/ui/system-design/composer-focus-hint.md
legacy_specs: []
---

# Implementation Plan: Mobile Composer Focus Hint

## Overview

Hide the phone focus hint and reclaim its editor space in one sequential work
order. Source inspection confirms that current eligibility ignores viewport
width, and `ChatInputBody` uses that value for both hint and `pr-28` padding.
An empty, unfocused phone composer reproduces the defect.

## Scope

### In scope

Shared composer hint presentation, responsive regression coverage, and draft
preservation across the breakpoint.

### Out of scope

Executor configuration, cache cleanup, shortcut bindings, and tablet redesign.

## Technical approach

Apply the canonical `useResponsiveBreakpoint` mobile result in
`ChatInputBody`. Share one effective visibility predicate between hint and
padding. Preserve the state helper and all input handlers. UI owns this reusable
presentation contract; no durable architectural boundary changes.

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

## Tests

Extend `chat-input-body.test.tsx` with mobile absence, no reserved padding,
wider visibility, and resize cases. The mobile case must fail on the existing
implementation before the production change. Retain the state helper tests in
`use-chat-input-container.test.ts` for existing eligibility.

## E2E tests

Extend `e2e/tests/chat/mobile-slash-command-composer.spec.ts` to check the hint
before tapping the empty editor, then retain the slash-command/send flow
(AC-001.1 and AC-001.3). Extend the desktop sibling with empty/unfocused hint
visibility and focus dismissal (AC-001.2). Scope assertions to the active
composer and inspect a phone screenshot. Use existing fixture readiness gates.

## Work orders

- [x] [Task 01: Gate composer focus hint on mobile](task-01-mobile-hint.md)

## Verification commands

```sh
(cd apps/web && pnpm exec vitest run components/task/chat/chat-input-body.test.tsx components/task/chat/use-chat-input-container.test.ts)
(cd apps/web && pnpm exec eslint components/task/chat/chat-input-body.tsx components/task/chat/chat-input-body.test.tsx e2e/tests/chat/mobile-slash-command-composer.spec.ts e2e/tests/chat/slash-command-composer.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome mobile-slash-command-composer.spec.ts)
(cd apps/web && pnpm e2e:run --project=chromium slash-command-composer.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

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


## Risks

Hiding only the hint leaves wasted padding. Using one effective predicate
prevents that mismatch. Do not redefine tablet or coarse-pointer policy.

## Review follow-up

Greptile identified that forced component rerenders could mask a broken
viewport subscription. Responsive cases now use native Happy DOM viewport
changes without parent rerenders. All 38 focused tests pass; disabling the
subscription makes the boundary-transition cases fail. ESLint passes.
Production behavior and screenshots are unchanged.

CodeRabbit identified an incomplete recorded ESLint command. The command now
lists both component files and both browser specs; rerunning it passed.
