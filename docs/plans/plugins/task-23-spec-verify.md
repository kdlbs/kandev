---
id: task-23
title: Spec + docs update + full verification (option C)
status: done
wave: C
depends_on: [task-17, task-19, task-20, task-21, task-22]
plan: docs/plans/plugins/plan.md
---

# Spec + docs update + full verification (option C)

## Title
Reconcile the spec with the native-JS-plugin model actually built and run the full
gate across backend + web + e2e.

## Inputs
- `docs/specs/plugins/spec.md` (draft) and `PLUGIN-API.md`.

## Acceptance
1. Spec edits:
   - Replace the "Out of scope → UI extensions" line: native JS UI plugins are now
     IN (bundle loaded into SPA, registry API). Reference `PLUGIN-API.md`.
   - Add a "Frontend plugin runtime" section summarizing the loading model,
     registry surface, `ui.bundle` manifest field, boot-payload `plugins` list, and
     the security posture (plugin JS runs in-origin; only active operator-registered
     plugins load; sandboxing is future work).
   - Correct credential storage (api_key bcrypt; webhook_secret encrypted/recoverable).
   - Keep marketplace / managed-runtime / agent-session tool wiring / rate limiting /
     JS sandboxing OUT.
   - `status: draft`; dated note if the spec has a changelog convention.
2. A short `docs/public/**` operator page if the repo documents settings pages
   (Plugins settings + how to register a plugin). Use `/docs-maintainer` judgment.
3. Mark all option-C task files `done`; update plan statuses.

## Verification (full gate)
- `make -C apps/backend fmt` (FIRST)
- `make -C apps/backend typecheck test lint build`
- `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test`
- `cd apps/web && pnpm e2e -g "plugin"`

## Output contract
Spec sections changed, docs added, and full verification results (paste failures).
Closes the plan.

## Dependencies
task-17, task-19, task-20, task-21, task-22.
