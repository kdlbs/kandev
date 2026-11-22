---
id: "02-installation"
title: "Guided optional installation"
status: done
wave: 2
depends_on: ["01-access-and-trust"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-002.2
  - AC-EXECUTORS-SSH-CONNECTIONS-002.3
  - AC-EXECUTORS-SSH-CONNECTIONS-003.1
  - AC-EXECUTORS-SSH-CONNECTIONS-003.2
  - AC-EXECUTORS-SSH-CONNECTIONS-003.3
  - AC-EXECUTORS-SSH-CONNECTIONS-003.4
  - AC-EXECUTORS-SSH-CONNECTIONS-003.5
  - AC-EXECUTORS-SSH-CONNECTIONS-003.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.11
  - AC-EXECUTORS-SSH-CONNECTIONS-004.15
  - AC-EXECUTORS-SSH-CONNECTIONS-004.16
  - AC-EXECUTORS-SSH-CONNECTIONS-004.17
  - AC-EXECUTORS-SSH-CONNECTIONS-004.19
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 02: Guided optional installation

## Summary

Replace the repeated enrollment form and cramped confirmation with explicit
method choice, readable inline review, and distinct result states. Preserve the
same request and security contract across direct and bastion connections.

## In scope

Three method choices, selected fields only, full-target review, manual key copy,
local pending and result feedback, exact secret lifecycle, route/viewport guards,
keyboard/focus/touch behavior, and translations. Installation replaces the ordinary
Access body while active. Update public enrollment instructions with this slice.

## Out of scope

Backend changes, private-key export, automatic install/retry/rollback, new SSH
authentication methods, generic wizard framework, and manual lab mutation.

## Acceptance

1. Method selection is explicit. Only its fields mount. Review shows the named
   account, optional bastion, separately labelled server/key fingerprints and
   effect. Only the final Install public key action submits, exactly once.
2. Passwords clear on dispatch, method/target/section/record changes, review Back,
   cancel and responsive invalidation. Unknown or installed-unverified outcomes
   stay warnings with a fresh-test path. Late responses never reopen another form.
3. Desktop and phone complete password, identity and manual paths. Real disposable
   direct/bastion enrollment still preserves authorization entries, verifies using
   the new identity, and respects host pins. Documentation matches rendered labels.

## UI preview

[UI-02 full preview](plan.md#ui-02-optional-installation), AC-003.1/2/5 and AC-004.15-19.
Structured preview follows the task's no-ASCII instruction.

| View | Content | Main action |
| --- | --- | --- |
| Choose access | Password / Existing identity / Install manually rows; selected fields | Review installation |
| Review | Account, route, server fingerprint, key identity/fingerprint, method, effect | Install public key |
| Running | One Installing key and checking access status, no credentials | Duplicate submit disabled |
| Result | One success, failure, installed-unverified or unknown outcome | Test access or correct inputs |

Phone uses the page width, stacked labels/values and full-width actions. The
progress label replaces ordinary Access milestones. No popover or extra modal.
Back from review clears the password. Manual copying never claims installation.

## Verification

TDD: adapt `enrollment-confirmation.test.tsx`, `enrollment-panel.test.tsx` and
`access-flow.test.tsx` before production edits. Add cases for all methods and five
terminal/pending outcomes, Enter-to-review without submission, stale route/key,
responsive decision change, Back/Escape clearing, same-tick duplicate submit and
late completion. Preserve request-shape/one-time-credential assertions.

Use disposable SSH fixtures, not the manual lab or main instance. Provision a
separate Docker daemon for the containers project and explicitly set DOCKER_HOST
to its socket before that command. Use guarded runner concurrency. Do not print
fixture passwords/private keys in results, traces, screenshots or reports.

```bash
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections hooks/domains/settings/use-ssh-connections.test.tsx lib/api/domains/ssh-connections-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/settings/ssh-connections e2e/helpers/ssh-access-setup.ts e2e/tests/settings/ssh-access-setup.spec.ts e2e/tests/settings/mobile-ssh-access-setup.spec.ts e2e/tests/ssh/managed-enrollment.spec.ts e2e/tests/ssh/managed-connections.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
make build-backend build-web
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-access-setup.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-access-setup.spec.ts
: "${DOCKER_HOST:?Set the separate disposable verification daemon socket}"
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --project containers tests/ssh/managed-enrollment.spec.ts tests/ssh/managed-connections.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inspect every stage in light/dark desktop and phone, long/pseudo text, keyboard
and reduced motion. Assert real hit areas, full fingerprint visibility and public
copy, page containment and focus return. Preserve all actual SSH safety cases
when updating selectors. Run additionally changed helper consumers, not only
these two container files, and record the exact final command set.

## Files likely touched

Relative to `apps/web/`: `components/settings/ssh-connections/{connection-access,enrollment-panel,public-key-details}.tsx`;
proposed `enrollment-methods.tsx`, `enrollment-review.tsx`, `enrollment-result.tsx`;
existing enrollment/confirmation/access tests; access E2E helpers and specs;
real managed-enrollment/managed-connections specs and affected shared helpers;
settings locale catalogs. Also `docs/public/executors.md` and this package.
`confirmation.tsx` remains available for deletion and unrelated consumers.

## Dependencies

Task 01 provides local operation feedback and inline trust review conventions.

## Risks

Progress claims unsupported by the synchronous API; dismissed requests still
finishing remotely; secrets retained between credential and review views;
incorrect success after uncertain enrollment; old tests tied to popover selectors.

## Parallelism

`sequential`

## Inputs

Proposed Optional installation and Mobile sections in the owning design, accepted
custody ADR, existing enrollment request/result types, and real fixture tests.
Use TDD/e2e, mobile-parity, better-ui and docs-maintainer.

## Results

Implemented explicit password/identity/manual method choice, an inline final review, immediate password cleanup, pending status, and distinct terminal outcomes. Unit coverage includes Back/Escape, target/key and viewport changes, duplicate clicks, late completion and all result states. Desktop and phone exercised password, identity and manual paths. The real SSH cases preserved existing authorization entries and authenticated after restart. Public instructions and all locale catalogs were updated. See [verification](verification.md), including the isolated-fixture network repair.
