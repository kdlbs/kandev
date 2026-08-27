---
id: "07-web-clarification-client"
title: "Web clarification client sends credentials and handles a lost claim"
status: done
wave: 6
depends_on: ["04-rest-endpoints"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/external-question-answering.md"
---

# Task 07: Web clarification client sends credentials and handles a lost claim

Authorization becomes load-bearing on the respond route, so the two bare `fetch` calls that skip
the session cookie have to be fixed here, and the overlay has to behave correctly when it loses the
claim.

- **Acceptance:**
  1. Both clarification POSTs send `credentials: "include"`, matching the shared client (W1). In
     split-origin dev mode (`__KANDEV_API_PORT` set) the session cookie now travels.
  2. A `claimed: false` success closes the overlay exactly like a win (W2).
  3. On `claimed: false` the hook does **not** write its own answers into message metadata — the
     winner's answers are not this client's. The status still flips so the carousel cannot strand
     on `pending`.

- **Verification:**
  ```
  cd apps && pnpm install --frozen-lockfile && \
    pnpm --filter @kandev/web test -- hooks/domains/session/use-clarification-group.test.ts
  cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:ratchet
  ```

- **Files likely touched:**
  - `apps/web/hooks/domains/session/use-clarification-group.ts` (`postClarificationBatch` at `:41`
    and `postClarificationSkip` at `:99` — add `credentials`, parse the body, return
    `{ state, claimed }`; the caller gates `safeApplyResolvedStatus` on `claimed !== false`)
  - `apps/web/hooks/domains/session/use-clarification-group.test.ts`

- **Dependencies:** 04 — the response envelope this reads is defined there.
- **Parallelism:** `sequential`.

- **Inputs:**
  - Spec W1, W2, and the **BREAKING CHANGE** note explaining why this client is the only in-tree
    production caller affected.
  - Plan § *Frontend*.
  - `lib/api/client.ts:80` is the reference for `credentials: "include"`; `lib/config.ts:36-40` is
    the split-origin case that makes the omission observable.
  - Keep the `res.status === 409` branch as a compatibility path even though the backend no longer
    emits it after task 04.
  - Any new user-facing string must go through `t()` / `<Trans>`; `console.error` diagnostics stay
    English and are not copy.

- **Output contract:** summary, files changed, tests run with counts, blockers, risks, and the
  task/plan status update in the same conversation.

## Results

Implemented in commit `da4ec994b` ("fix(web): harden clarification overlay for lost races and long
answers"). Both `postClarificationBatch` and `postClarificationSkip` in
`use-clarification-group.ts` now send `credentials: "include"`, parse the response body, and return
`{ state, claimed }`; the caller gates `safeApplyResolvedStatus(..., localAnswers)` on
`claimed !== false` so a lost claim closes the overlay without writing this client's own (unrecorded)
answers into message metadata. The `res.status === 409` branch was kept as a compatibility path.

Verified in this now-summarized prior segment of this session: `pnpm --filter @kandev/web test --
hooks/domains/session/use-clarification-group.test.ts` passed with new cases for
`credentials: "include"` on both POSTs and for `claimed:false` closing the overlay without applying
local answers; `pnpm run typecheck`, `pnpm run lint`, and `pnpm run i18n:ratchet` from `apps/web` were
clean. Re-confirmed in this session's Wave 10 gauntlet via the full `pnpm --filter @kandev/web test`
and `pnpm run lint` runs (see plan.md § Verification Results for the consolidated command list).

No external side effects. This is the BREAKING CHANGE compatibility fix noted in the spec — the web
client is the only in-tree production caller of the REST envelope task 04 changed.
</content>
