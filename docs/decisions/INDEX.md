# Decision Log

Architecture Decision Records (ADRs) for the Kandev project. Each decision captures the context, choice, consequences, and alternatives for significant architectural or design decisions.

Read individual ADRs for full context. Create new ones via `/record decision` or manually following the template in `0001-file-based-knowledge-system.md`.

| # | Title | Status | Area | Date |
|---|-------|--------|------|------|
| 0001 | [File-based knowledge system](0001-file-based-knowledge-system.md) | accepted | infra | 2026-03-28 |
| 0002 | [Host utility agentctl for sessionless ACP flows](0002-host-utility-agentctl-for-sessionless-flows.md) | accepted | backend | 2026-04-08 |
| 0003 | [executors_running as the single source of truth for agent_execution_id](0003-executors-running-as-execution-id-source-of-truth.md) | accepted | backend | 2026-05-03 |
| 0004 | [Task model unification — shared base, per-strategy meta, shared kernel](0004-task-model-unification.md) | proposed | backend, frontend | 2026-05-05 |
| 0005 | [Agent model unification — one `agents` table](0005-agent-model-unification.md) | proposed | backend, frontend | 2026-05-06 |
| 0006 | [Tier routing vs cheap_agent_profile_id coexistence](0006-tier-routing-vs-cheap-agent-profile-coexistence.md) | superseded | backend | 2026-05-11 |
| 0007 | [profiles.yaml — runtime defaults for prod / dev / e2e](0007-runtime-feature-flags.md) | accepted | backend, frontend | 2026-05-16 |
