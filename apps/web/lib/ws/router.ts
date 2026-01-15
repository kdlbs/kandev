import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import { registerAgentsHandlers } from '@/lib/ws/handlers/agents';
import { registerAgentSessionHandlers } from '@/lib/ws/handlers/agent-session';
import { registerBoardsHandlers } from '@/lib/ws/handlers/boards';
import { registerColumnsHandlers } from '@/lib/ws/handlers/columns';
import { registerMessagesHandlers } from '@/lib/ws/handlers/messages';
import { registerDiffsHandlers } from '@/lib/ws/handlers/diffs';
import { registerEnvironmentsHandlers } from '@/lib/ws/handlers/environments';
import { registerExecutorsHandlers } from '@/lib/ws/handlers/executors';
import { registerGitStatusHandlers } from '@/lib/ws/handlers/git-status';
import { registerKanbanHandlers } from '@/lib/ws/handlers/kanban';
import { registerSystemEventsHandlers } from '@/lib/ws/handlers/system-events';
import { registerTasksHandlers } from '@/lib/ws/handlers/tasks';
import { registerTerminalsHandlers } from '@/lib/ws/handlers/terminals';
import { registerUsersHandlers } from '@/lib/ws/handlers/users';
import { registerWorkspacesHandlers } from '@/lib/ws/handlers/workspaces';

export function registerWsHandlers(store: StoreApi<AppState>) {
  return {
    ...registerKanbanHandlers(store),
    ...registerTasksHandlers(store),
    ...registerBoardsHandlers(store),
    ...registerColumnsHandlers(store),
    ...registerWorkspacesHandlers(store),
    ...registerExecutorsHandlers(store),
    ...registerEnvironmentsHandlers(store),
    ...registerAgentsHandlers(store),
    ...registerAgentSessionHandlers(store),
    ...registerUsersHandlers(store),
    ...registerTerminalsHandlers(store),
    ...registerDiffsHandlers(store),
    ...registerMessagesHandlers(store),
    ...registerGitStatusHandlers(store),
    ...registerSystemEventsHandlers(store),
  };
}
