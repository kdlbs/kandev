import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { registerSessionModelsHandlers } from "@/lib/ws/handlers/session-models";
import { registerSessionInfoHandlers } from "@/lib/ws/handlers/session-info";

import { registerMessagesHandlers } from "@/lib/ws/handlers/messages";
import { registerNotificationsHandlers } from "@/lib/ws/handlers/notifications";
import { registerExecutorPrepareHandlers } from "@/lib/ws/handlers/executor-prepare";
import { registerGitStatusHandlers } from "@/lib/ws/handlers/git-status";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { registerTerminalsHandlers } from "@/lib/ws/handlers/terminals";
import { registerTurnsHandlers } from "@/lib/ws/handlers/turns";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
import { registerOfficeHandlers } from "@/lib/ws/handlers/office";
import { registerRunHandlers } from "@/lib/ws/handlers/run";

export function registerWsHandlers(store: StoreApi<AppState>) {
  return {
    ...registerTasksHandlers(store),

    ...registerExecutorPrepareHandlers(store),
    ...registerTaskSessionHandlers(store),
    ...registerSessionModelsHandlers(store),
    ...registerSessionInfoHandlers(store),
    ...registerUsersHandlers(store),
    ...registerTerminalsHandlers(store),
    ...registerMessagesHandlers(store),
    ...registerNotificationsHandlers(store),
    ...registerGitStatusHandlers(store),
    ...registerTurnsHandlers(store),
    ...registerOfficeHandlers(store),
    ...registerRunHandlers(),
  };
}
