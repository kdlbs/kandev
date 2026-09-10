## Fixed during review
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:116 -- auto-fallback decisions retained a configured fallback model in provider-default warning metadata; normalized the policy so auto-fallback ignores the fallback model consistently with the runtime contract (commit 649b4fa22)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:140 -- auto-fallback profiles failed when ACP model selection was unsupported; restored provider-default continuation with selection_unsupported warning metadata (commit fdfbffc9a)
