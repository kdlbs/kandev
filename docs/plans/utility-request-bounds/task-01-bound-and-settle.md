---
id: "01-bound-and-settle"
title: "Bound utility requests and settle cancelled calls"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-002
acceptance_criteria:
  - AC-AGENTS-UTILITY-AGENT-PROFILES-002.1
  - AC-AGENTS-UTILITY-AGENT-PROFILES-002.2
  - AC-AGENTS-UTILITY-AGENT-PROFILES-002.3
  - AC-AGENTS-UTILITY-AGENT-PROFILES-002.4
system_design:
  - ../../specs/agents/system-design/utility-agent-profiles.md
---

# Task 01: Bound utility requests and settle cancelled calls

## Summary

Enhance prompt sends changed file names instead of the diff, every utility
request is bounded before rendering, untouched stored Enhance prompts pick up
the new template, and a call abandoned by its caller ends as `cancelled`.

## In scope

- `apps/web/hooks/use-utility-agent-generator.ts`: no diff for
  `enhance-prompt`, 200-name file list, diff cut on a UTF-8 boundary 64
  bytes below the 100,000-byte server cap, leaving room for its marker.
- `config/utilityagents/enhance-prompt.md`: `{{ChangedFiles}}` replaces
  `{{GitDiff}}`.
- `internal/utility/controller`: `truncateContext`, newest-history retention,
  and the per-field caps.
- `internal/utility/store`: hash-guarded refresh of the stored
  Enhance prompt, run after the placeholder migration.
- `internal/utility/handlers`, `service`, `models`: atomic profile-attributed
  call creation, detached terminal writes for completion and failure,
  `CancelCall`, `CallStatusCancelled`, and the 499 response.

## Out of scope

- Client-side cancel controls, per-provider budgets, and other templates.

## Acceptance

1. Enhance prompt requests carry `changed_files` and no `git_diff`; commit
   message requests still carry the diff; a list longer than 200 names ends
   with the remaining count.
2. Oversized template inputs are cut at a rune boundary with a marker. History
   keeps the newest text in chronological order and places the marker first.
3. A stored Enhance prompt equal to a shipped revision is refreshed, including
   after profile-only edits; customized or unknown prompt text is kept.
4. Profile IDs are present in the initial call insert. A request cancelled
   before a result answers 499 and leaves a `cancelled` call row with a
   completion time. A successful execution leaves a `completed` row after a
   client disconnect on both execution paths.

## Verification

```bash
# from the repository root
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/utility/...)
(cd apps/web && pnpm exec vitest run hooks/use-utility-agent-generator.test.tsx)
(cd apps/web && pnpm run i18n:check && pnpm run typecheck)
(cd apps/web && pnpm e2e:run tests/chat/quick-chat.spec.ts -- --grep "keeps an edited active-session prompt and offers recovery")
(cd apps/web && pnpm e2e:run tests/task/enhance-prompt-session-context.spec.ts)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

## Likely files

- `apps/backend/config/utilityagents/enhance-prompt.md`
- `apps/backend/internal/utility/controller/{controller,context_limits,context_limits_test}.go`
- `apps/backend/internal/utility/handlers/{handlers,fixture_test,cancellation_test}.go`
- `apps/backend/internal/utility/models/models.go`
- `apps/backend/internal/utility/service/service.go`
- `apps/backend/internal/utility/store/{builtins,sqlite,builtins_refresh_test}.go`
- `apps/web/hooks/use-utility-agent-generator{.ts,.test.tsx}`

## Results

- Before the change, the Vitest cases "does not send the diff when enhancing a
  prompt" and "limits the changed-file list it sends" failed; the new Go tests
  need the new caps, settlement, and refresh code, so they do not compile
  against the previous package API.
- The caps, constants (`Max*Bytes`), and truncation marker all count UTF-8
  bytes, on the client and the server; a Vitest case checks the client cut on
  a multibyte boundary.
- The client cuts 64 bytes below the server cap, so a client-cut diff and its
  marker pass the server cap unchanged: a 3 MB diff reaches the template as
  `[... truncated 2900064 bytes ...]`. Without the reserve the server cut it
  again and reported only the 34 bytes of the client marker. Vitest checks the
  client output stays under the cap; Go tests pass a client-shaped diff (cut
  and uncut) through `truncateContext` unchanged.
- The handler fixture's fake repository now honors context cancellation like
  the SQLite store, which is what exposed the pending row: a write through the
  cancelled request context fails.
- `go test ./internal/utility/...`, the hook Vitest file, `pnpm run
  i18n:check`, `pnpm run typecheck`, Oxlint, Prettier, and
  `scripts/lint-go-changed` pass.
- Fixup regression tests confirm that Enhance does not read file diffs, history
  keeps recent messages, call creation inserts both profile IDs once, detached
  success writes finish for session-bound and sessionless calls, profile fallback
  attribution survives cancellation, and profile-only edits do not block an
  exact shipped prompt refresh. Focused Go and Vitest checks pass.
- Fixup checks also enforce each full rendered field cap, preserve a successful
  profile result when cancellation races with its return, and inspect the active
  Quick Chat Enhance request in a browser test. The refresh fixture now applies
  the profile edit before replay, and its call repository returns independent
  snapshots so rejected writes cannot look persisted.
- Contributor follow-up: `enhance-prompt-session-context.spec.ts` runs Enhance
  in a task session whose workspace has 205 changed files and checks the request:
  200 names, the remaining count, and no `git_diff`. It passes on chromium
  together with `enhance-prompt.spec.ts`.
