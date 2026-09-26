# Review components

Review file identity is repository-scoped, not path-scoped: preserve
`repository_name` and use `reviewFileKey` or an equivalent composite identity.
Preserve explicit `base_ref` and `is_submodule` metadata; nested submodule
changes remain anchored to the parent gitlink. The backend contract tests are
in `apps/backend/internal/agentctl/server/api/git_multi_repo_review_test.go`.
