---
id: "03-diagnostics"
title: "Actionable diagnostic report"
status: done
wave: 3
depends_on: ["01-access-and-trust", "02-installation"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-002.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.2
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-004.5
  - AC-EXECUTORS-SSH-CONNECTIONS-004.11
  - AC-EXECUTORS-SSH-CONNECTIONS-004.12
  - AC-EXECUTORS-SSH-CONNECTIONS-004.17
  - AC-EXECUTORS-SSH-CONNECTIONS-004.18
  - AC-EXECUTORS-SSH-CONNECTIONS-004.19
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 03: Actionable diagnostic report

## Summary

Replace the bare diagnostics button and bullet list with a scoped report and
issue-specific remedies. Keep local setup evidence separate from fresh SSH
login results and executor readiness.

## In scope

Initial/pending/request-error/no-issue/issue states; hop groups; safe code/title/
explanation/remedy projection; manual live-test results; canonical navigation
with continuation context; localized counts and safe unknown-code fallback.
Update public diagnostic guidance and reconcile all package results.

## Out of scope

Invented per-check success/timing, backend streaming, new checks, automatic
repairs or probes, protected-storage replacement, agent lifecycle management,
executor runtime testing, broad unrelated QA/CI work, or deployment.

## Acceptance

1. Setup results explain their scope, group real issues by affected hop and
   expose a relevant remedy. Failed requests and empty issue lists cannot imply
   successful network login. Unknown codes and absent hops have safe fallbacks.
2. Live SSH tests are explicit and independently reported. Results are stamp-
   scoped, stale/late responses cannot replace current state, and navigation
   remedies never mutate remote access. Global initial-load failures stay visible.
3. Desktop/phone users can follow each remedy and return. Keyboard, touch,
   localization and long content work. Public guidance and package receipts match
   the final UI. Earlier completed packages remain historical and linked.

## UI preview

[UI-03 full preview](plan.md#ui-03-diagnostic-report), AC-004.17/18/19.
Structured preview follows the task's no-ASCII instruction.

| Order | Desktop | Phone |
| --- | --- | --- |
| 1 | Check SSH setup, scope, Check setup | Scope then full-width action |
| 2 | Local pending, error, issue count or no-issue summary | Same single status region |
| 3 | Destination/Bastion heading; issue title, cause, remedy; row dividers | Stacked endpoint and issue, action below cause |
| 4 | Optional technical details | Touch-sized disclosure |
| 5 | Separate live SSH test explanation/action/result | Full-width action, same evidence boundary |

No per-step green checklist or repeated nested cards. The settings shell owns
scrolling. Empty means no current report until a request succeeds.

## Verification

TDD: proposed `diagnostic-presentation.test.ts` for known/unknown codes, missing
hop, plural counts, zero issues with either ready value, stamp mismatch and
permitted remedy targets. Proposed `prerequisite-panel.test.tsx` covers request
failure, retained stale report, late responses, explicit test and feedback owner.
Create desktop/phone `ssh-diagnostics.spec.ts` scenarios with UI fixtures for
no issues, failed request, multi-hop issues, unknown code, missing agent/file/
managed key, live auth rejection, stale results and remedy navigation. Keep the
real SSH proof in Task 02; these cases do not claim transport coverage.

```bash
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections hooks/domains/settings/use-ssh-connections.test.tsx lib/settings/ssh-connection-settings-routes.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/settings/ssh-connections e2e/tests/settings/ssh-diagnostics.spec.ts e2e/tests/settings/mobile-ssh-diagnostics.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
make build-backend build-web
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-diagnostics.spec.ts tests/settings/ssh-access-setup.spec.ts tests/settings/ssh-navigation.spec.ts tests/settings/ssh-profile-connection-link.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-diagnostics.spec.ts tests/settings/mobile-ssh-access-setup.spec.ts tests/settings/mobile-ssh-navigation.spec.ts tests/settings/mobile-ssh-profile-connection-link.spec.ts
pnpm --dir apps/web e2e:run --host --project auth tests/auth/ssh-connections-permissions.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Commands run from repo root; browser projects run sequentially on fresh builds.
Inspect real desktop/phone rendered initial, running, failed, multi-issue and
success states in both themes and pseudo locale. Measure actual control bounds
and contrast; verify a narrow fine-pointer viewport and coarse tablet. If a
shared helper changes, include all affected consumers in the final record.
Rerun `.github/scripts/pr-docs.cjs` local `validateCoverage` with the completed
package and its linked contracts. Do not invoke its publishing/GitHub path.

## Files likely touched

Relative to `apps/web/`: `components/settings/ssh-connections/prerequisite-panel.tsx`,
`manager-detail.tsx`, `use-manager-navigation.ts`; proposed
`diagnostic-presentation.ts`, `diagnostic-issue.tsx` and tests; existing error/
operation hooks only as needed; canonical settings route helper; proposed
`e2e/tests/settings/{ssh-diagnostics,mobile-ssh-diagnostics}.spec.ts` and a
bounded shared helper; all affected settings locales.
Also `docs/public/executors.md`, the owning specs and all affected package work
orders. Update old package successor links without rewriting historical results.

## Dependencies

Tasks 01 and 02 establish feedback ownership, Access destinations and wording.

## Risks

Misrepresenting `Preflight.ready` as a fresh network test; inferring successful
individual checks; linking to a nonexistent identity edit/unlock command;
losing executor/bastion return context; stale or unknown errors hidden by success.

## Parallelism

`sequential`

## Inputs

Owning proposed Diagnostics and Operation feedback sections; backend Preflight
and ProbeFailure; existing route/navigation helpers; SettingsGroup and Kubernetes
result vocabulary. Apply TDD/e2e, mobile-parity, better-ui and docs-maintainer.

## Results

Implemented scoped local setup reports, hop groups, safe unknown-code handling, existing remedy targets and a separate live SSH test. Identity remedies return to the original section. Failed and late reports cannot leave a false success. Unit, desktop/phone, localization, public-doc and specification checks passed. Inspected light/dark diagnostic issue and request-failure captures. See [verification](verification.md).
