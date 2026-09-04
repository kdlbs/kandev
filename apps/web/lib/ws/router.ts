import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { registerAvailableCommandsHandlers } from "@/lib/ws/handlers/available-commands";
import { registerSessionModeHandlers } from "@/lib/ws/handlers/session-mode";
import { registerSessionPollModeHandlers } from "@/lib/ws/handlers/session-poll-mode";
import { registerAgentCapabilitiesHandlers } from "@/lib/ws/handlers/agent-capabilities";
import { registerSessionModelsHandlers } from "@/lib/ws/handlers/session-models";
import { registerSessionMCPStatusHandlers } from "@/lib/ws/handlers/session-mcp-status";
import { registerSessionInfoHandlers } from "@/lib/ws/handlers/session-info";
import { registerSessionPendingActionHandlers } from "@/lib/ws/handlers/session-pending-action";
import { registerSessionTodosHandlers } from "@/lib/ws/handlers/session-todos";
import { registerPromptUsageHandlers } from "@/lib/ws/handlers/prompt-usage";
import { registerWorkflowsHandlers } from "@/lib/ws/handlers/workflows";

import { createMessagesHandlerRegistration } from "@/lib/ws/handlers/messages";
import { registerNotificationsHandlers } from "@/lib/ws/handlers/notifications";
import { registerDiffsHandlers } from "@/lib/ws/handlers/diffs";
import { registerExecutorsHandlers } from "@/lib/ws/handlers/executors";
import { registerExecutorProfileHandlers } from "@/lib/ws/handlers/executor-profiles";
import { registerExecutorPrepareHandlers } from "@/lib/ws/handlers/executor-prepare";
import { registerGitStatusHandlers } from "@/lib/ws/handlers/git-status";
import { registerKanbanHandlers } from "@/lib/ws/handlers/kanban";
import { registerSystemEventsHandlers } from "@/lib/ws/handlers/system-events";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { registerTaskPlansHandlers } from "@/lib/ws/handlers/task-plans";
import { registerWalkthroughsHandlers } from "@/lib/ws/handlers/walkthroughs";
import { registerReviewHandlers } from "@/lib/ws/handlers/review";
import { registerTerminalsHandlers } from "@/lib/ws/handlers/terminals";
import { registerTurnsHandlers } from "@/lib/ws/handlers/turns";
import { registerSecretsHandlers } from "@/lib/ws/handlers/secrets";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
import { registerWorkspacesHandlers } from "@/lib/ws/handlers/workspaces";
import { registerRepositorySetsHandlers } from "@/lib/ws/handlers/repository-sets";
import { registerRepositoryBranchPoliciesHandlers } from "@/lib/ws/handlers/repository-branch-policies";
import { registerGitHubHandlers } from "@/lib/ws/handlers/github";
import { registerGitLabHandlers } from "@/lib/ws/handlers/gitlab";
import { registerOfficeHandlers } from "@/lib/ws/handlers/office";
import { registerRunHandlers } from "@/lib/ws/handlers/run";

export function registerWsHandlers(store: StoreApi<AppState>) {
  const messages = createMessagesHandlerRegistration(store);
  const handlers = {
    ...registerKanbanHandlers(store),
    ...registerTasksHandlers(store),
    ...registerTaskPlansHandlers(store),
    ...registerWalkthroughsHandlers(store),
    ...registerReviewHandlers(store),
    ...registerWorkflowsHandlers(store),

    ...registerWorkspacesHandlers(store),
    ...registerRepositorySetsHandlers(store),
    ...registerRepositoryBranchPoliciesHandlers(store),
    ...registerExecutorsHandlers(store),
    ...registerExecutorProfileHandlers(store),
    ...registerExecutorPrepareHandlers(store),
    ...registerAgentsHandlers(store),
    ...registerTaskSessionHandlers(store),
    ...registerAvailableCommandsHandlers(store),
    ...registerSessionModeHandlers(store),
    ...registerSessionPollModeHandlers(store),
    ...registerAgentCapabilitiesHandlers(store),
    ...registerSessionModelsHandlers(store),
    ...registerSessionMCPStatusHandlers(store),
    ...registerSessionInfoHandlers(store),
    ...registerSessionPendingActionHandlers(store),
    ...registerSessionTodosHandlers(store),
    ...registerPromptUsageHandlers(store),
    ...registerUsersHandlers(store),
    ...registerTerminalsHandlers(store),
    ...registerDiffsHandlers(store),
    ...messages.handlers,
    ...registerNotificationsHandlers(store),
    ...registerSecretsHandlers(store),
    ...registerGitStatusHandlers(store),
    ...registerSystemEventsHandlers(store),
    ...registerTurnsHandlers(store, messages.scheduler),
    ...registerGitHubHandlers(store),
    ...registerGitLabHandlers(store),
    ...registerOfficeHandlers(store),
    ...registerRunHandlers(),
  };
  return { handlers, dispose: messages.dispose };
}
