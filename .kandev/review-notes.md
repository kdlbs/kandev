## Fixed during review
- apps/backend/internal/clarification/store.go:340 — closed a retry waiter that could race with cancellation and otherwise wait indefinitely after detached delivery won (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/mcp/handlers/handlers.go:3978 — allowed a finalized detached rejection to reconcile immediately instead of being masked by the delivery-miss receipt (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:116 -- auto-fallback decisions retained a configured fallback model in provider-default warning metadata; normalized the policy so auto-fallback ignores the fallback model consistently with the runtime contract (commit 649b4fa22)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:140 -- auto-fallback profiles failed when ACP model selection was unsupported; restored provider-default continuation with selection_unsupported warning metadata (commit fdfbffc9a)
- Exact-profile assignment required only task read access; it now requires the task prompt scope, with denial coverage (commit 95ffb52e83).
- Exact-profile session binding could outlive the profile revision observed during resolution; the binding write now checks the live enabled profile and revision, and lifecycle checks the snapshot before launch (commit 95ffb52e83).
- Early exact-profile launch denials omitted their immutable failed-closed receipt, while a receipt write error could be hidden on success. Denials now record receipts and storage errors surface without replacing the original launch failure (commit 95ffb52e83).
- A receipt write error after an agent started could release its launch reservation before bookkeeping. Launch ownership and post-launch bookkeeping now finish before that error is returned; race and injected-error regressions cover the path (commit 28640bb65d).

## Follow-up tasks created

None for the exact-profile repair.

## Action required by author

None for the exact-profile repair.

## QA fixes worth reviewer attention

- Two isolated binding-race fixtures now seed the live profile and revision tables required by the guarded binding query (commit 28640bb65d).

## Assumptions made during QA

- The approved contract keeps one immutable launch receipt per task/session and carries only assignment generation in a deferred move; the profile revision is revalidated at launch.

## UI screenshots for PR/MR

Not applicable: this change has no rendered UI behavior.
