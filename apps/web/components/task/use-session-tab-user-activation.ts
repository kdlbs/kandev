import { useCallback, type KeyboardEvent, type PointerEvent } from "react";
import type { IDockviewPanelHeaderProps } from "dockview-react";
import { useAppStoreApi } from "@/components/state-provider";
import {
  markSessionTabUserActivationIntent,
  shouldActivateSessionTabFromInput,
} from "./session-tab-activation-intent";

export function useSessionTabUserActivationIntent(
  sessionId: string | undefined,
  api: Pick<IDockviewPanelHeaderProps["api"], "isActive">,
) {
  const appStore = useAppStoreApi();
  const activate = useCallback(
    (target: EventTarget | null) => {
      if (!sessionId) return;
      const state = appStore.getState();
      if (
        !shouldActivateSessionTabFromInput({
          sessionId,
          activeSessionId: state.tasks.activeSessionId,
          target,
        })
      )
        return;
      if (api.isActive) {
        const taskId = state.tasks.activeTaskId;
        if (taskId && state.taskSessions.items[sessionId]?.task_id === taskId) {
          state.setActiveSession(taskId, sessionId);
        }
      } else {
        markSessionTabUserActivationIntent(sessionId);
      }
    },
    [api, appStore, sessionId],
  );
  const handlePointerDownCapture = useCallback(
    (event: PointerEvent) => {
      if (event.button === 0) activate(event.target);
    },
    [activate],
  );
  const handleKeyDownCapture = useCallback(
    (event: KeyboardEvent) => {
      if (event.key === "Enter" || event.key === " ") activate(event.target);
    },
    [activate],
  );
  return { handlePointerDownCapture, handleKeyDownCapture };
}
