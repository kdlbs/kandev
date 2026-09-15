## Fixed during review
- apps/backend/internal/clarification/store.go:340 — closed a retry waiter that could race with cancellation and otherwise wait indefinitely after detached delivery won (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/mcp/handlers/handlers.go:3978 — allowed a finalized detached rejection to reconcile immediately instead of being masked by the delivery-miss receipt (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
