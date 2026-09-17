# Unified workspace orchestration

Status: implemented; supersedes the separate Office workflow UX. Full reviewed plan is stored on the Kandev task.

1. Remove workflow creation from persona setup; support stable workspace conversations without a delivery workflow.
2. Bridge runtime task creation/management to existing Kanban task service and preserve account placement.
3. Unify workspace navigation, conversation and existing task access; preserve legacy URLs.
4. Cover authorization, runtime ownership, navigation and end-to-end delegation.
5. Back up and adapt the isolated test setup, rebuild and verify a real chief conversation.

Validation completed on 2026-09-07:

- Backend race tests passed for workspace adapters, Office runtime/service/repository, task repository, orchestrator and agentctl. Added bounded-result and wrong-session/account rejection tests. Final adapter/CLI run also passed with the production `fts5` build tag.
- Go lint reported zero issues. Frontend typecheck, targeted lint, navigation/routing tests, i18n and public documentation checks passed.
- Browser coverage passed for existing-workspace connections, Kanban-first onboarding, persistent conversation and delegation. An outdated chief URL expectation was corrected and its tests rerun successfully.
- Isolated preview rebuilt with `fts5` and restored Node PATH. Default Workspace retains its Development workflow; no Office delivery workflow was created.
- Real chief created KAN-1 (`39bd0d8e-2c2a-41cf-9382-db4521926ba5`) once, launched the personal execution profile through Kanban, and its worker reached Review. Chief subsequently inspected and summarized the bounded worker result without starting another session. Review-event wake and duplicate-event suppression are covered by tests; the real result readback was manually requested.
- Legacy Office configuration was copied into new personas on the existing workspace; legacy personas are paused and their history remains intact in the legacy workspace. No wholesale history or task migration was performed.

Deployment limits: remote SSH hosts are not configured in this preview. Human-required permission/question gates remain human actions. Recurring routines were not enabled. Broader host-service monitoring and recovery are not provided by this integration. No PR was published.
