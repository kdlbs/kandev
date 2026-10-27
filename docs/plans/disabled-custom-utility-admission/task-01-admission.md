---
id: "01-admission"
title: "Reject disabled custom utility execution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-001
acceptance_criteria:
  - AC-AGENTS-UTILITY-AGENT-PROFILES-001.9
system_design:
  - ../../specs/agents/system-design/utility-agent-profiles.md
---

# Task 01: Reject disabled custom utility execution

## Summary

Enforce saved custom-utility availability at the shared execution preparation boundary and expose a clear client error. Preserve built-in behavior, existing profile eligibility, and calls admitted before a later disable.

## In scope

- Add typed service rejection before template, profile, and dynamic routing resolution; map it to HTTP 400 without a call ID.
- Add independent service and registered-HTTP/SQLite regression coverage for both branches and controls described in the plan; correct existing executable fixtures' enabled state where necessary.
- Update the custom utility how-to in `docs/public/developer-tools.md` with the admission behavior.
- Isolate the existing review-checkbox E2E scenario's disposable seed checkout and assert its two intended review files before the first scroll, addressing a hosted CI fixture interaction.
- Carry the separately validated mobile profile-deletion E2E correction that selects the seeded profile by ID instead of assuming the first agent has a profile.

## Out of scope

Built-in policy, settings save/refresh, frontend changes, schema/migrations, cancellation, unrelated validation or error mapping, and ROOT's private qualifier.

## Acceptance

- Disabled custom execution returns HTTP 400 with `utility agent is disabled`, no `call_id`, no call row, no resolver or provider dispatch in either session mode; typed service error precedes template and profile resolution.
- Enabled and re-enabled utilities run; nonexistent and ineligible selections preserve their current contracts, and a disabled built-in still follows its existing default/profile path.
- A call admitted while enabled continues after a subsequent disable, and future calls reject. Public guidance describes the resulting custom-utility behavior.

## Verification

After ROOT's separate implementation release and heavy lease, install workspace dependencies once from `apps/` before any pnpm commands, tests, or hooks. Run independently authored tests in RED before implementing and record the expected failure; then run GREEN and the owning packages with the following from repository root. Preserve each actual process handle until joined. The named regression tests are to be created by this work order.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout -k 10s 300s /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -race -tags fts5 -p 1 ./internal/utility/service ./internal/utility/handlers -run 'TestPreparePromptRequestDisabledCustomAdmission|TestExecuteDisabledCustomAdmission' -count=1)
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout -k 10s 600s /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -race -tags fts5 -p 1 ./internal/utility/service ./internal/utility/handlers -count=1)
env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=3GiB timeout -k 10s 600s bash scripts/lint-go-changed apps/backend/internal/utility/service/service.go apps/backend/internal/utility/service/service_test.go apps/backend/internal/utility/service/admission_test.go apps/backend/internal/utility/handlers/handlers.go apps/backend/internal/utility/handlers/execute_handlers_test.go apps/backend/internal/utility/handlers/admission_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Pass every actually changed Go file to `lint-go-changed`, including any adjacent test file chosen instead of the listed file. The lint command's normal 3 GiB budget is separate from the Go test budget. If `scripts/run-quiet` is used, give each invocation a unique owned `KANDEV_RUN_QUIET_DIR`. Record real results and joins; do not substitute a duplicate run for a lost handle.

For the CI fixture repair, from `apps/web` with the existing fresh build, run the following sequentially, with one worker and zero retries:

```bash
pnpm e2e:run --host --no-build --project chromium e2e/tests/cli-mode/passthrough-toolbar.spec.ts e2e/tests/review/review-viewed-toggling.spec.ts -- --retries=0 --grep 'supports @ prompt/file lookup chips and sends context to passthrough stdin|uploads attachments and delivers them as workspace file paths|keeps a file unchecked until scrolling back and passing it again'
pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/review/mobile-review-viewed-toggling.spec.ts -- --retries=0 --grep 'keeps a file unchecked until scrolling back and passing it again'
pnpm exec eslint --max-warnings 0 e2e/tests/review/review-viewed-toggling-flow.ts
```

The repo's web TypeScript configuration excludes `e2e/`; the browser runs exercise the changed TypeScript but do not provide a separate E2E typecheck.

## Files likely touched

- `apps/backend/internal/utility/service/service.go`, `service_test.go`, and `admission_test.go`
- `apps/backend/internal/utility/handlers/handlers.go`, `execute_handlers_test.go`, and `admission_test.go`
- `docs/public/developer-tools.md`
- `apps/web/e2e/tests/review/review-viewed-toggling-flow.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-delete.spec.ts`
- This plan and work order for status/results; the paired requirement and design for lifecycle reconciliation.

## Dependencies

None. Implementation requires a later explicit ROOT interrupt and heavy lease.

## Risks

False-valued custom test fixtures may encode accidental defaults. A disable racing after the row read is not a cancellation or serialized admission guarantee.

## Parallelism

sequential

## Inputs

- `AC-AGENTS-UTILITY-AGENT-PROFILES-001.9` and the API surface section of the paired system design.
- Existing utility service and handler tests, `store.Provide`, and registered route setup. Independently author tests without consulting private or historical qualifier fixtures.
- ROOT's delivery rules, including actual joins, hosted validation, and separate merge grant.

## Results

Focused RED failed on the intended behavior: disabled utilities returned 200 in both modes and resolved profiles/routes. Focused GREEN passed, followed by both complete owning packages with `-race` (pass). Changed-Go lint passed (0 issues); `list-docs.py validate` and specification lint passed; public-doc test suite passed 63 tests and the validator passed 54 pages. Frozen pnpm installation completed once. PR documentation coverage preflight: covered, no errors. Final diff check passed.

For hosted CI remediation, the saved failing ordered run contained four review inputs (two expected), establishing the new precondition's RED evidence without a repeat. The managed host runner reused fresh assets and ran the same two passthrough predecessors followed by the exact desktop review case once: 3 passed, one worker, zero retries. The exact mobile review case also passed once with one worker and zero retries. Focused ESLint and Prettier checks passed. No UI production code changed. In the contaminated diagnostic, two sentinels crossed together but only the preceding file was marked; the internal cause remains unproven and this fixture repair does not claim to resolve it. Hosted verification remains pending.

The mobile profile-deletion fixture correction is carried byte-for-byte from commit `86913def0773785e99b18f37302f1e203a77d7d9` (Git blob `b7d19f242e703571d9f88f1e435786ea2dba9712`). ROOT verified the producer ran its three mobile cases once with one worker and zero retries, all passing. The carried file passed focused ESLint and Prettier checks here; no duplicate browser run was made. Hosted verification remains pending.
