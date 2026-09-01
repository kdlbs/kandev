## Fixed during review
- apps/backend/internal/github/service_ci_run_request.go:143 — Rejected non-hex 40-character head identifiers at the MCP schema and backend admission boundaries. (commit d80b944223c848c44018da8d5614f1d2826ae616)
- apps/backend/internal/github/store_ci_run_request.go — Extended the atomic provider-start guard to require the live task repository and open, non-detached canonical PR association.
- apps/backend/internal/github/service_ci_run_request.go — Denied workflow dispatch before mutation when a required workflow input has no default.
- apps/backend/internal/github/token_client_actions.go — Requested dispatch run details on the scoped Actions API and verified returned run identities while retaining 204 reconciliation.
