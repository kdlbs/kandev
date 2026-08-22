---
id: "09-e2e-duplicate-submit"
title: "E2E: overlay closes on a lost claim"
status: done
wave: 7
depends_on: ["07-web-clarification-client"]
plan: "plan.md"
spec: "../../specs/external-question-answering/spec.md"
---

# Task 09: E2E — overlay closes on a lost claim

The one user-visible surface this spec touches is the clarification overlay. The existing spec file
already covers it end to end and already intercepts the respond route; extend it rather than adding
a file.

- **Scenario:** GIVEN a clarification bundle rendered in chat, WHEN the user submits their answers
  and the backend reports that another answerer already won, THEN the overlay closes and the
  messages render resolved instead of stranding on `pending`.

- **Acceptance:**
  1. A new case in the existing spec file intercepts `**/api/v1/clarification/*/respond` and
     fulfils `200 {"success":true,"claimed":false,"status":"answered","resume":"published"}`.
  2. The carousel closes and the bundle renders resolved.
  3. No new spec file is added and the existing cases still pass.

- **Verification:**
  ```
  cd apps && pnpm install --frozen-lockfile
  cd apps/web && pnpm run build:e2e && \
    pnpm e2e:raw e2e/tests/chat/clarification.spec.ts && \
    pnpm run e2e:sleep-ratchet && pnpm run lint:e2e-sleeps
  ```

- **Files likely touched:**
  - `apps/web/e2e/tests/chat/clarification.spec.ts` (extend; the route-interception idiom is
    already there at `:803`)

- **Dependencies:** 07.
- **Parallelism:** `sequential`.

- **Inputs:**
  - Spec § *Verification notes* → **E2E decision**, W2, R2.
  - Plan § *E2E Tests*.
  - The two MCP tools get no E2E — they have no browser surface (spec, same section).
  - `apps/web/e2e/README.md` for fixture and project conventions; no sleep-based waits.

- **Output contract:** summary, files changed, tests run with counts, blockers, risks, and the
  task/plan status update in the same conversation. Reconcile **Files likely touched** with the
  actual diff, including the modified existing spec used as E2E evidence.

## Results

Implemented in commit `94c9187af` ("test(web): cover a lost clarification race in the E2E overlay
flow"). Added one new case to the existing `apps/web/e2e/tests/chat/clarification.spec.ts` (no new
spec file), inserted between the "select option (happy path)" and "moves answered task from Review to
In progress without reload" tests. It intercepts `**/api/v1/clarification/*/respond` and fulfils
`200 {"success":true,"claimed":false,"status":"answered","resume":"published"}` with a `custom_text`
response body (chosen over `selected_options` so the assertion does not need to know the real
backend-generated `option_id`), then asserts the carousel closes and no leftover option-label text
(`"PostgreSQL"`) remains anywhere in `session.chat`.

Verified in this session's final gauntlet (Wave 10): after one-time environment setup
(`make build-backend` and `make -C apps/backend e2e-plugin-package`, both required by this worktree
having no prebuilt backend binary or E2E fixture plugin package yet), `pnpm run build:e2e && pnpm
e2e:raw e2e/tests/chat/clarification.spec.ts` passed all 31 cases (30 pre-existing + 1 new), and
`pnpm run e2e:sleep-ratchet` was clean. `pnpm run lint:e2e-sleeps` with no path argument produced
~374 errors across hundreds of unrelated files (`Definition for rule '...' was not found`) — a
pre-existing structural characteristic of `eslint.e2e-sleeps.config.mjs`, a narrow standalone config
meant to be path-scoped, unrelated to this diff; re-run scoped to the touched file only
(`pnpm run lint:e2e-sleeps e2e/tests/chat/clarification.spec.ts`) is clean. `git diff --check` reports
no whitespace errors.

No external side effects. The route interception intercepts at the browser fetch level, so no real
backend session is resolved by this test — consistent with the file's existing precedent for the
respond route.
</content>
