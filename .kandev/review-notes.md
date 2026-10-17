## Fixed during review
- apps/backend/internal/clarification/store.go:340 — closed a retry waiter that could race with cancellation and otherwise wait indefinitely after detached delivery won (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/mcp/handlers/handlers.go:3978 — allowed a finalized detached rejection to reconcile immediately instead of being masked by the delivery-miss receipt (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:116 -- auto-fallback decisions retained a configured fallback model in provider-default warning metadata; normalized the policy so auto-fallback ignores the fallback model consistently with the runtime contract (commit 649b4fa22)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:140 -- auto-fallback profiles failed when ACP model selection was unsupported; restored provider-default continuation with selection_unsupported warning metadata (commit fdfbffc9a)

## Mobile workspace pointer restoration

Task 47e59734-ca4e-4cb0-a759-39b24db0bcea restores mobile workspace navigation from
the Tasks drawer. A closing Radix menu remains mounted for its exit animation;
disposing its enclosing drawer during that interval could lose the original body
pointer style. Shared dropdowns inside modal drawer content now use the drawer's
existing modal owner. Standalone menus and explicit modal settings retain their
behavior. No body-style recovery setter, delay, dependency change, or backend
change is introduced.

The animation-aware regression failed on the baseline and passes with the fix,
including restoration of empty and `auto` inline styles. Focused coverage checks
both workspace directions, reconnect recovery, task selection, menu/drawer
Escape ordering, focus return, archive confirmation, and editing from the drawer.
Independent Review and QA conclusions and candidate/runtime identities are
recorded in the task plan after the committed candidate is verified.

### UI screenshots for PR/MR

Task-local screenshots and traces are retained under `.kandev/evidence/green/`.
MAIN opened every final image: both workspace menus, both destination
conversations, and the drawer retained after menu dismissal. The independent QA
session supplies its own final candidate screenshots and inspection record.

### Action required by author

No new product or architecture decision is required. Parent material-report
delivery currently returns `human_input_pending`; unsent receipts are preserved
in the task plan. No independent gate is waived by this delivery condition.
