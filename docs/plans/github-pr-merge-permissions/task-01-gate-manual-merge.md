---
id: "01-gate-manual-merge"
title: "Gate manual merge by target repository access"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.13
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.15
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.16
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.17
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.18
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.19
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.20
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
---

# Task 01: Gate manual merge by target repository access

## Summary

Replace the merge action with a permission explanation when its effective
actor cannot write to the target repository. Keep pending or unknown access
unavailable and preserve eligible direct and queued merges.

## In scope

- Extend the shared repository projection with explicit push-permission presence.
- Add the actor-scoped, read-only access endpoint through personal-write resolution.
- Add API types and the domain hook with request-generation invalidation.
- Gate both merge button variants and translate every new message.
- Add focused service, client, component, and desktop/mobile regression coverage.

Update public integration guidance with the visible access states. Add an
explicit mock repository-details seed operation that uses
`MockClient.SetRepositoryDetails`. Preserve mock search data as read-only evidence.
Reset the new fixture state through the existing mock-reset path.

## Out of scope

Automatic merge, queue policy, account switching, permission grants, schema
migrations, GitLab, and unrelated UI styling.

## Acceptance

1. A ready PR with target read access and fork admin access shows an explanation
   in every affected surface and submits no merge request.
2. Missing, loading, failed, or stale access never enables a merge. Permission
   refresh performs a read only and cannot reuse another context's result.
3. A permitted human or App retains supported direct and queued merges, actor
   attribution, method fallback, and GitHub's final permission enforcement.

## Implementation sequence

1. Add the behavioral RED case named in the plan to `pr-merge-button.test.tsx`.
   Run it before changing production code and record its assertion failure.
2. Add provider/service tests for permission presence, actor selection, target
   identity, scope failures, and App capability handling. Implement the access read.
3. Add hook/component tests. Implement request invalidation and all access states.
4. Add explicit mock permission fixtures and desktop/mobile E2E. Update existing
   enabled-merge fixtures without granting writes to all mock repositories.
5. Run the exact checks below. Record results and update public documentation,
   the work-order status, and the plan status only after successful verification.

## ASCII UI preview

UI-01: Existing merge-action region. See the
[full preview and mobile contract](plan.md#ascii-ui-preview).

```text
Denied:     zeval cannot merge into svkozak/pi-acp.
Loading:    Checking merge permissions.
Unknown:    Merge permissions are unavailable. [ Check again ]
Allowed:    [ Squash and merge ]
            as zeval
```

Desktop details and popovers use the existing action group. Phone status uses
the existing inset drawer. Phone Review uses the existing review surface.
Reuse their scroll owner, safe-area handling, focus return, and dismissal.
The explanation is inline and wraps. No disabled-button tooltip is required.
Check again is a read-only action with a 44px phone/coarse-pointer target.
All copy goes through `t()` and every supported locale.

This view covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.16, .17, and .19.

## Verification

Run from the repository root. Install workspace dependencies once in this worktree
before the first pnpm command. Commands use isolated working directories.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/github/pr-merge-button.test.tsx -t 'does not expose a merge before access is confirmed')
```

The first targeted run must fail for the missing permission gate. After the
implementation, run:

```bash
(cd apps/backend && go test -trimpath ./internal/github -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race ./internal/github -count=1)
bash scripts/lint-go-changed $(git diff --name-only -- '*.go') $(git ls-files --others --exclude-standard -- '*.go')
(cd apps/web && pnpm exec vitest run components/github/pr-merge-button.test.tsx components/github/pr-task-icon.test.ts components/github/pr-status-chip.test.tsx hooks/domains/github/use-repo-merge-access.test.ts hooks/domains/github/use-repo-merge-methods.test.ts lib/api/domains/github-api.test.ts lib/api/domains/github-pr-api.test.ts lib/github-auth.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/github/pr-merge-button.tsx components/github/pr-merge-access-notice.tsx hooks/domains/github/use-repo-merge-access.ts lib/api/domains/github-pr-api.ts lib/types/github.ts lib/github-auth.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-merge-permissions.spec.ts tests/pr/pr-merge-queue.spec.ts tests/pr/pr-detail-layout.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-merge-permissions.spec.ts tests/pr/mobile-pr-merge-queue.spec.ts tests/pr/mobile-pr-rerequest-review.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Use the managed E2E runner's fresh build. Run desktop and mobile commands
sequentially. Set `CAPTURE_PR_ASSETS=1` to capture the denied and unavailable states in both existing
surfaces. Confirm no horizontal page overflow and the actual refresh hitbox.

The Go command covers the shared repository projection and its existing fork
consumers. If a changed suite or additional helper lies outside the listed
checks, add its exact command before marking this work order complete.
Run the repository PR-documentation coverage preflight against the linked
package as part of final documentation validation.

## Files likely touched

Backend ownership:

- `apps/backend/internal/github/models.go`, `repository_api.go`, and focused repository tests.
- New `apps/backend/internal/github/service_merge_access.go` and `service_merge_access_test.go`.
- `apps/backend/internal/github/controller.go` plus a focused controller test file.
- `apps/backend/internal/github/gh_client_reads_test.go` and relevant PAT repository tests.
- `apps/backend/internal/github/mock_client.go`, `mock_controller.go`, and their focused tests.

Frontend ownership:

- `apps/web/lib/types/github.ts` and `lib/api/domains/github-pr-api.ts`.
- `apps/web/lib/api/domains/github-pr-api.test.ts`.
- New `apps/web/hooks/domains/github/use-repo-merge-access.ts` and its test file.
- `apps/web/components/github/pr-merge-button.tsx` and its test file.
- `apps/web/src/locales/*/github.json`, including generated Traditional Chinese and pseudo catalogs.

E2E and documentation ownership:

- `apps/web/e2e/helpers/api-client.ts` for the explicit permission fixture helper.
- New `apps/web/e2e/tests/pr/pr-merge-permissions.spec.ts` and `mobile-pr-merge-permissions.spec.ts`.
- Existing `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`, `mobile-pr-merge-queue.spec.ts`,
  `pr-detail-layout.spec.ts`, and `mobile-pr-rerequest-review.spec.ts` fixtures.
- `docs/public/integrations.md` and this plan's result records.

## Dependencies

None. The existing repository reader and credential resolver provide the required
boundaries. No new library or database migration is needed.

## Risks

Keep human repository access separate from installation capabilities. Preserve
unknown permission information without changing existing contribution behavior.
Reject delayed results after context changes and keep credentials out of payloads.
Do not use `viewerCanMergeAsAdmin` or source-fork ownership to grant target access.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-merge-queue.md), manual action criteria.
- [System design](../../specs/integrations/system-design/github-pr-merge-queue.md#manual-merge-access).
- [ADR 0047](../../decisions/0047-github-authentication-ownership.md), actor resolution and isolation.
- `repositoryDetailsReader`, `projectGitHubRepository`, and `resolvePersonalWriteClient`.
- Existing merge-button unit tests and desktop/mobile merge-queue E2E.
- `/tdd`, `/e2e`, `/mobile-parity`, and the scoped backend/frontend engineering guides.

## Results

Implementation started after the user's later `continue` request.

- RED: the component regression found an enabled merge button while permission
  was unconfirmed. The new controller test returned 404 before route implementation.
- GREEN: the backend GitHub package passed, including GH/PAT permission projection,
  target and actor selection, App capabilities, scope denial, provider failures,
  and mock-fixture reset. Focused frontend tests passed after the gate was added.
- The E2E fixture requires the new provider metadata route. The component test
  supplied the behavioral RED gate; browser tests use the completed real backend
  path to verify the rendered states and retain merge-action coverage.
- TypeScript passed with `NODE_OPTIONS=--max-old-space-size=8192`. The default
  Node heap was insufficient for this repository. Frontend lint and translation
  checks passed. The pinned Go linter was installed in a temporary tool directory.
- Specification and public documentation validation passed. PR documentation
  coverage passed with this work order and its three referenced documents.
- Go tests passed both normally and with `-race`. The changed-Go lint helper
  and a complete `golangci-lint run ./internal/github --timeout=5m --concurrency=2`
  each reported zero issues, including the new files.
- Initial focused frontend unit run: 178 tests passed across eight files. TypeScript
  was checked again after the additional regression cases and passed.
- The commit hook caught the API test file's 600-line limit. The permission
  regression now lives in the existing focused PR API test file. The final
  unit run includes that file and its existing merge-outcome regression.
- Managed browser tests: eight desktop tests and five phone tests passed,
  without retries or skipped tests. They cover target READ with fork ADMIN,
  absent or failed permission reads, explicit recovery, no accidental mutation,
  direct merge, queue submission, existing header layout, and review switching.
- Desktop retry height was 28px. Phone retry height was at least 44px, with
  no document overflow at 320px. Permission checks leave PR status colors intact.
- The first browser launch inherited a non-loopback host setting and stopped
  before UI assertions. Final runs set `KANDEV_SERVER_HOST=127.0.0.1` and loaded
  the missing Chromium libraries through a temporary Nix environment. The
  successful runs reused the freshly built backend, web assets, and plugin fixture
  with `--no-build`; no production source changed between build and verification.
- Browser logs: `/tmp/kandev-run.e2e.Zu8lyE01.log` (desktop) and
  `/tmp/kandev-run.e2e.c5udhd7q.log` (phone). Denied and unavailable screenshots
  are retained in `/tmp/kandev-merge-ui/`. All four screenshots were inspected.
  Text wraps, controls remain inside their surfaces, and the desktop popover
  capture waits for its opening animation to finish.

### Review remediation

The lookup now clears its snapshot when its effect ends. A readiness
disable/enable cycle cannot restore an earlier grant in the first committed
render. Its request ownership key updates in a layout effect, so a suspended
render cannot invalidate the visible lookup or its refresh callback.

Two focused regressions failed before this correction: re-enabling returned
`allowed` before effects ran, and a suspended render left the committed lookup
in `loading`. Both now pass. The hook suite passes all 15 cases, the eight-file
frontend command above passes 178 tests, and TypeScript and targeted ESLint pass.
These changes enforce the existing context-invalidation contract; public copy,
API behavior, and the UI layout remain as documented.

The managed browser commands above passed again after this correction: eight
desktop tests and five phone tests, without retries or skips. Desktop verification
rebuilt the runtime and web assets; the phone run reused that build. Logs:
`/tmp/kandev-run.e2e.LwaeCxtw.log` and `/tmp/kandev-run.e2e.R1BUWPdG.log`.

### CI remediation

Frontend Tests (2/2), run `38887399635`, exposed an existing footer assertion
that selected the only status region in the entire popover. The new permission
notice is a second valid live region. The query now targets the footer and still
requires its status role and refresh text. No product behavior or public contract
changed, so the existing specifications and UI screenshots remain applicable.

The exact failing test reproduced locally before the edit. These commands then
passed from `apps/web`:

```bash
NODE_ENV=production pnpm run test components/github/pr-ci-popover.footer.test.tsx
pnpm exec vitest run components/github hooks/domains/github/use-repo-merge-access.test.ts hooks/domains/github/use-repo-merge-methods.test.ts lib/api/domains/github-api.test.ts lib/api/domains/github-pr-api.test.ts lib/github-auth.test.ts
pnpm exec eslint components/github/pr-ci-popover.footer.test.tsx --max-warnings 0
pnpm exec prettier --check components/github/pr-ci-popover.footer.test.tsx
```

The focused footer suite passed all five tests. The expanded GitHub suite passed
669 tests across 73 files. Logs: `/tmp/pr-4532-footer-red.log`,
`/tmp/pr-4532-footer-green.log`, and `/tmp/pr-4532-github-consumers-green.log`.

### Notice presentation correction

The displayed first letter is uppercase. Compact permission notices use the
same horizontal inset as Review follow-up. The account identifier remains
unchanged. Phone captures wait for the drawer animation and scroll the notice
into view before recording it.

The two merge-button and popover-footer unit suites passed all 16 tests.
Targeted ESLint and Prettier passed. The managed permission browser suites
passed two desktop cases and one phone case with the rebuilt web assets.
Logs: `/tmp/pr-4532-notice-unit.log`, `/tmp/kandev-run.e2e.OLiUYpg8.log`,
and `/tmp/kandev-run.e2e.fW1Fp0m0.log`.
