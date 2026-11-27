---
id: "01-warn-before-start"
title: "Warn before starting a watched fork review"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.7
system_design:
  - ../../specs/integrations/system-design/github-fork-review-start.md
---

# Warn before starting a watched fork review

Deliver the waiting state and pre-launch warning in [the plan](plan.md). Consume the backend waiting result in use-ensure-task-session.ts. Scope includes chat and preview feedback, the edit dialog, translations, and focused tests. Exclude provider settings and changes to executor permissions.

1. Opening a marked task uses the existing backend waiting result and leaves the task without a session.
2. Desktop and phone render the warning for marked tasks in the empty chat and Edit dialog. Closing the dialog leaves the task idle. An explicit launch starts one session. Refresh displays the existing session.
3. Verify same-repository reviews with complete identity retain configured behavior and ordinary PR-link tasks do not show the warning. Cover unknown identity with the same marked-task test.

## ASCII UI preview

UI-01. Empty chat before the first session. The warning and action sit within the chat panel.

```text
Manual start required
Kandev could not confirm that this PR comes from the target repository.
Starting the review can run setup scripts from the PR.
Those scripts may have access to executor credentials.
[Start Task]
```

Desktop and phone use the same reading order. Text wraps within the existing scroll area. Use the existing accessible alert and a touch target of at least 44 px on phones. In Edit, place the warning next to the existing launch control. The wording and spacing shown here are illustrative.

## Verification

Run from the repository root:

```sh
pnpm --dir apps/web exec vitest run hooks/domains/session/use-ensure-task-session.test.ts
pnpm --dir apps/web run i18n:check
make build-web-e2e
make -C apps/backend build-kandev build-agentctl build-mock-agent e2e-plugin-package build-e2e-delivery-fixture build-e2e-send-now-fixture
pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium tests/task/fork-review-start.spec.ts
pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-fork-review-start.spec.ts
```


## Results

- The implementation reuses the backend waiting response delivered in #5443.
- Focused frontend tests, typecheck, lint, locale validation, and the web build passed.
- Desktop and phone Playwright tests passed against the current build. They
  verified the warning inside chat, cancel, manual start, and reload. The desktop test
  also checked the waiting state in the task preview. The warning sits near
  the bottom, where the composer normally appears.
- All build and test processes ran in systemd scopes with an 8 GB memory cap.
