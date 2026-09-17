# Workspace orchestration

Experimental Orchestration lives in `app/settings/orchestration`, with shared API helpers in `lib/api/domains/orchestration-api.ts`. Instances are workspace scoped; roles are global settings. Keep workspace cards/tabs, settings discovery, sidebar, native conversations and task return links connected under the same flag. Mobile task headers are fixed: put task-to-orchestrator navigation inside `SessionMobileTopBar`, not in a sibling strip it covers. Configuration uses the settings save contributor; roles own global names, icons and live instructions and profiles supply execution identity.

Orchestration conversations use their own route, API, locale namespace and transport adapters around the core chat renderer. Do not import Office pages, APIs or stores into Orchestration. Shared rendering receives feature-specific comment, retry and identity behavior through contexts.

Automation orchestrator destinations inherit execution configuration from their workspace assignment. Keep stale task profile/executor/repository fields out of those payloads. Delivery history links to `conversation_task_id` and labels dispatch separately from work completion.
