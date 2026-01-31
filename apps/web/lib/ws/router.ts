import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import { registerAgentsHandlers } from '@/lib/ws/handlers/agents';
import { registerTaskSessionHandlers } from '@/lib/ws/handlers/agent-session';
import { registerBoardsHandlers } from '@/lib/ws/handlers/boards';

import { registerMessagesHandlers } from '@/lib/ws/handlers/messages';
import { registerNotificationsHandlers } from '@/lib/ws/handlers/notifications';
import { registerDiffsHandlers } from '@/lib/ws/handlers/diffs';
import { registerEnvironmentsHandlers } from '@/lib/ws/handlers/environments';
import { registerExecutorsHandlers } from '@/lib/ws/handlers/executors';
import { registerGitStatusHandlers } from '@/lib/ws/handlers/git-status';
import { registerKanbanHandlers } from '@/lib/ws/handlers/kanban';
import { registerSystemEventsHandlers } from '@/lib/ws/handlers/system-events';
import { registerTasksHandlers } from '@/lib/ws/handlers/tasks';
import { registerTaskPlansHandlers } from '@/lib/ws/handlers/task-plans';
import { registerTerminalsHandlers } from '@/lib/ws/handlers/terminals';
import { registerTurnsHandlers } from '@/lib/ws/handlers/turns';
import { registerUsersHandlers } from '@/lib/ws/handlers/users';
import { registerWorkspacesHandlers } from '@/lib/ws/handlers/workspaces';

export function registerWsHandlers(store: StoreApi<AppState>) {
  return {
    ...registerKanbanHandlers(store),
    ...registerTasksHandlers(store),
    ...registerTaskPlansHandlers(store),
    ...registerBoardsHandlers(store),

    ...registerWorkspacesHandlers(store),
    ...registerExecutorsHandlers(store),
    ...registerEnvironmentsHandlers(store),
    ...registerAgentsHandlers(store),
    ...registerTaskSessionHandlers(store),
    ...registerUsersHandlers(store),
    ...registerTerminalsHandlers(store),
    ...registerDiffsHandlers(store),
    ...registerMessagesHandlers(store),
    ...registerNotificationsHandlers(store),
    ...registerGitStatusHandlers(store),
    ...registerSystemEventsHandlers(store),
    ...registerTurnsHandlers(store),
  };
}
