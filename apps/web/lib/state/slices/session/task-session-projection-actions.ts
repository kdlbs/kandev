import type { StateCreator } from "zustand";
import type { SessionSlice } from "./types";

type ImmerSet = Parameters<
  StateCreator<SessionSlice, [["zustand/immer", never]], [], SessionSlice>
>[0];

export function buildTaskSessionProjectionActions(set: ImmerSet) {
  return {
    setTaskSessionPendingAction: (
      sessionId: string,
      pendingAction: Parameters<SessionSlice["setTaskSessionPendingAction"]>[1],
    ) =>
      set((draft) => {
        const session = draft.taskSessions.items[sessionId];
        if (!session) return;
        session.pending_action = pendingAction;
        const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[session.task_id];
        if (sessionsByTask) {
          const match = sessionsByTask.find((item) => item.id === sessionId);
          if (match) match.pending_action = pendingAction;
        }
      }),
  };
}
