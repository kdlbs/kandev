import { useCallback, useEffect, useMemo, useState } from "react";
import type { IDockviewPanelHeaderProps } from "dockview-react";
import { useAppStore } from "@/components/state-provider";
import { useSessionActions } from "@/hooks/domains/session/use-session-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { clearHiddenSessionPanel, hideSessionPanel } from "./dockview-hidden-session-panels";
import { countVisibleSessionPanels } from "./session-tab-visibility";

/** Binds session actions to the tab's panel lifecycle and visible sibling count. */
export function useSessionTabActions(
  sessionId: string | undefined,
  taskId: string | null,
  api: IDockviewPanelHeaderProps["api"],
  containerApi: IDockviewPanelHeaderProps["containerApi"],
) {
  const dockviewApi = useDockviewStore((state) => state.api);
  const taskSessions = useAppStore((state) =>
    taskId ? state.taskSessionsByTask.itemsByTaskId[taskId] : undefined,
  );
  const taskSessionIds = useMemo(
    () => new Set(taskSessions?.map((session) => session.id) ?? []),
    [taskSessions],
  );
  const [visibleSessionCount, setVisibleSessionCount] = useState(0);
  useEffect(() => {
    const update = () =>
      setVisibleSessionCount(countVisibleSessionPanels(containerApi.panels, taskSessionIds));
    update();
    const added = containerApi.onDidAddPanel(update);
    const removed = containerApi.onDidRemovePanel(update);
    return () => {
      added.dispose();
      removed.dispose();
    };
  }, [containerApi, taskSessionIds]);
  const onDeleted = useCallback(() => {
    if (sessionId) clearHiddenSessionPanel(containerApi, sessionId);
    const panel = containerApi.getPanel(api.id);
    if (panel) containerApi.removePanel(panel);
  }, [api.id, containerApi, sessionId]);
  const {
    setPrimary: handleSetPrimary,
    stop: handleStop,
    resume: handleResume,
    restart: handleRestart,
    isRestarting,
    remove: handleDelete,
    removeById,
  } = useSessionActions({ sessionId, taskId, onDeleted });
  const handleCloseOthers = useCallback(() => {
    const toClose = api.group.panels.filter((panel) => panel.id !== api.id);
    for (const panel of toClose) containerApi.removePanel(panel);
  }, [api, containerApi]);
  const handleHide = useCallback(() => {
    if (sessionId) hideSessionPanel(dockviewApi ?? containerApi, sessionId, taskId ?? undefined);
  }, [containerApi, dockviewApi, sessionId, taskId]);
  return {
    handleSetPrimary,
    handleStop,
    handleResume,
    handleRestart,
    isRestarting,
    handleDelete,
    removeById,
    handleCloseOthers,
    handleHide,
    visibleSessionCount,
  };
}
