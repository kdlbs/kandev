import type { StoreApi } from "zustand";
import type { QueryClient } from "@tanstack/react-query";
import type { AppState } from "@/lib/state/store";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { registerAvailableCommandsHandlers } from "@/lib/ws/handlers/available-commands";
import { registerAgentCapabilitiesHandlers } from "@/lib/ws/handlers/agent-capabilities";
// session.mode_changed / session.poll_mode_changed / session.todos_updated are
// now mirrored into TanStack Query by bridge/session-runtime.ts only; their
// Zustand handlers were removed (D6 migration). registerSessionModelsHandlers
// is retained solely for its client-only activeModel-clearing side effect.
import { registerSessionModelsHandlers } from "@/lib/ws/handlers/session-models";
import { registerPromptUsageHandlers } from "@/lib/ws/handlers/prompt-usage";

import { registerMessagesHandlers } from "@/lib/ws/handlers/messages";
import { registerNotificationsHandlers } from "@/lib/ws/handlers/notifications";
import { registerDiffsHandlers } from "@/lib/ws/handlers/diffs";
// executor.prepare.progress / executor.prepare.completed are now mirrored into
// TanStack Query by bridge/session-runtime.ts only; their Zustand handlers were
// removed (D6 prepare-progress migration).
import { registerGitStatusHandlers } from "@/lib/ws/handlers/git-status";
import { registerSystemEventsHandlers } from "@/lib/ws/handlers/system-events";
import { registerTaskPlansHandlers } from "@/lib/ws/handlers/task-plans";
import { registerTerminalsHandlers } from "@/lib/ws/handlers/terminals";
import { registerTurnsHandlers } from "@/lib/ws/handlers/turns";
import { registerWorkspacesHandlers } from "@/lib/ws/handlers/workspaces";
import { registerRunHandlers } from "@/lib/ws/handlers/run";

export function registerWsHandlers(store: StoreApi<AppState>, queryClient: QueryClient) {
  return {
    ...registerTaskPlansHandlers(store, queryClient),

    ...registerWorkspacesHandlers(store),
    ...registerAgentsHandlers(store),
    ...registerTaskSessionHandlers(store, queryClient),
    ...registerAvailableCommandsHandlers(store),
    ...registerAgentCapabilitiesHandlers(store),
    ...registerSessionModelsHandlers(store),
    ...registerPromptUsageHandlers(store),
    ...registerTerminalsHandlers(store),
    ...registerDiffsHandlers(store),
    ...registerMessagesHandlers(store),
    ...registerNotificationsHandlers(store),
    ...registerGitStatusHandlers(store),
    ...registerSystemEventsHandlers(store),
    ...registerTurnsHandlers(store),
    ...registerRunHandlers(),
  };
}
