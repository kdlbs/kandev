import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useDockviewStore, captureLayoutApplication } from "@/lib/state/dockview-store";
import { storeReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { SavedLayout } from "@/lib/types/http";
import { resolveLayoutApplySessionIds } from "@/components/task/layout-preset-selector-session-ids";

export function useApplySavedLayout() {
  const appStore = useAppStoreApi();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const envId = useDockviewStore((state) => state.currentLayoutEnvId);
  const apply = useDockviewStore((state) => state.applyCustomLayout);
  const { isLoaded, loadSessions } = useTaskSessions(activeTaskId);
  const intent = useRef(0);
  useEffect(
    () => () => {
      intent.current++;
    },
    [],
  );
  return useCallback(
    async (layout: SavedLayout) => {
      const initial = appStore.getState();
      if (
        initial.tasks.activeTaskId !== activeTaskId ||
        initial.tasks.activeSessionId !== activeSessionId
      )
        return;
      if (useDockviewStore.getState().currentLayoutEnvId !== envId) return;
      const request = ++intent.current;
      const scope = storeReadScopeIdentity(appStore);
      const revision = initial.taskRemoval.navigationRevision;
      const layoutIsCurrent = captureLayoutApplication();
      const sessionIds = await resolveLayoutApplySessionIds({
        activeTaskId,
        activeSessionId,
        sessionsLoaded: isLoaded,
        loadSessions,
        getSessionsForTask: (taskId) =>
          appStore.getState().taskSessionsByTask.itemsByTaskId[taskId] ?? [],
      });
      const current = appStore.getState();
      if (
        request !== intent.current ||
        !layoutIsCurrent() ||
        storeReadScopeIdentity(appStore) !== scope ||
        current.tasks.activeTaskId !== activeTaskId ||
        current.tasks.activeSessionId !== activeSessionId ||
        current.taskRemoval.navigationRevision !== revision
      )
        return;
      apply(
        {
          id: layout.id,
          name: layout.name,
          isDefault: layout.is_default,
          layout: layout.layout,
          createdAt: layout.created_at,
        },
        { activeSessionId, sessionIds, envId },
      );
    },
    [appStore, activeTaskId, activeSessionId, envId, apply, isLoaded, loadSessions],
  );
}
