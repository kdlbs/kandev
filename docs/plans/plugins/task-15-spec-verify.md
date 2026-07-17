---
id: task-15
title: Spec update + docs + full verification
status: pending
wave: 4
depends_on: [task-09, task-13, task-14]
plan: docs/plans/plugins/plan.md
---

# Spec update + docs + full verification

## Title
Reconcile the spec with what was built, update docs, and run the full
format/typecheck/test/lint gate across backend and web.

## Inputs
- Spec `docs/specs/plugins/spec.md` (status: draft).
- Implemented deltas from this plan.

## Acceptance
1. Spec edits to `docs/specs/plugins/spec.md`:
   - Add the `ui.pages[]` manifest field (key/title/path/surface) and a new
     "UI page proxy" subsection under API surface (GET/POST
     `/api/plugins/{id}/ui/*`, header stripping for iframe embedding).
   - Narrow "Out of scope → UI extensions": iframe pages are now IN; native JS
     slots / registry remain OUT.
   - Note tool invocation is implemented as a client + listing endpoint but NOT
     wired into live agent sessions yet (deferred).
   - Correct the credential-storage line: api_key stored as bcrypt hash;
     webhook_secret stored encrypted (recoverable) because kandev must HMAC-sign
     outbound deliveries. Keep "returned once at registration".
   - Keep marketplace / managed-runtime / process-management OUT of scope.
   - Keep `status: draft`; add a dated note if the spec has a changelog convention.
2. Update `docs/plans/plugins/plan.md` task statuses to `done`.
3. If a public-docs surface changed, run `/docs-maintainer` judgment (new
   operator-facing `/api/plugins` API + settings page likely warrants a short
   `docs/public/**` page — add one if the repo documents other settings pages).

## Verification (full gate)
- `make -C apps/backend fmt` (FIRST — formatters may split lines)
- `make -C apps/backend typecheck test lint`
- `make -C apps/backend build`
- `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test`
- `cd apps/web && pnpm e2e -g "plugin"`

## Output contract
Report: spec sections changed, docs added, and the full verification results
(paste failing output if any). This task closes the plan.

## Dependencies
task-09, task-13, task-14.
