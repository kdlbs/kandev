"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { markSessionTabUserActivationIntent } from "./session-tab-activation-intent";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useTranslation } from "react-i18next";
import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import type { SessionRecoveryOwner } from "@/lib/session-recovery-presentation";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";

export type TaskLaunchErrorContextValue = {
  taskId: string;
  revealSessionRecovery?: (sessionId: string) => void;
  workspaceId: string;
  statusSummary?: TaskStatusSummary | null;
  repositories?: TaskRepository[];
  automaticRecovery?: SessionRecoveryOwner | null;
  /** Claims one assertive announcement for a task error stamp. */
  claimTaskErrorAnnouncement?: (stamp: string) => boolean;
};

const TaskLaunchErrorContext = createContext<TaskLaunchErrorContextValue | null>(null);

export function TaskLaunchErrorProvider({
  value,
  children,
}: {
  value: TaskLaunchErrorContextValue;
  children: ReactNode;
}) {
  const store = useAppStoreApi();
  const { t } = useTranslation();
  const focusCleanup = useRef<(() => void) | null>(null);
  useEffect(() => () => focusCleanup.current?.(), []);
  const revealSessionRecovery = useCallback(
    (sessionId: string) => {
      focusCleanup.current?.();
      store.getState().setActiveSession(value.taskId, sessionId);
      markSessionTabUserActivationIntent(sessionId);
      store.getState().setMobileSessionPanel(sessionId, "chat");
      const dock = useDockviewStore.getState();
      const panel = dock.api?.getPanel(`session:${sessionId}`);
      if (panel) panel.api.setActive();
      else if (dock.api) addSessionPanel(dock.api, dock.centerGroupId, sessionId, t("task:chat"));
      const focus = () => {
        const owner = document.getElementById(`session-recovery-${sessionId}`);
        if (!owner || (owner.checkVisibility && !owner.checkVisibility())) return false;
        owner.focus();
        return true;
      };
      if (focus()) return;
      const observer = new MutationObserver(() => {
        if (focus()) focusCleanup.current?.();
      });
      observer.observe(document.body, { childList: true, subtree: true });
      const timeout = setTimeout(() => observer.disconnect(), 5000);
      focusCleanup.current = () => {
        observer.disconnect();
        clearTimeout(timeout);
      };
    },
    [store, value.taskId, t],
  );
  const statusSummary = useTaskStatusSummary(value.taskId, value.statusSummary);
  const announcedTaskErrorRef = useRef<string | null>(null);
  const claimTaskErrorAnnouncement = useCallback(
    (stamp: string) => {
      const key = `${value.taskId}:${stamp}`;
      if (!stamp || announcedTaskErrorRef.current === key) return false;
      announcedTaskErrorRef.current = key;
      return true;
    },
    [value.taskId],
  );

  const contextValue = useMemo(
    () => ({ ...value, statusSummary, claimTaskErrorAnnouncement, revealSessionRecovery }),
    [claimTaskErrorAnnouncement, statusSummary, value, revealSessionRecovery],
  );
  return (
    <TaskLaunchErrorContext.Provider value={contextValue}>
      <SessionErrorAnnouncement taskId={value.taskId} summary={statusSummary} />
      {children}
    </TaskLaunchErrorContext.Provider>
  );
}

export function useTaskLaunchErrorContext(): TaskLaunchErrorContextValue | null {
  return useContext(TaskLaunchErrorContext);
}

function SessionErrorAnnouncement({
  taskId,
  summary,
}: {
  taskId: string;
  summary?: TaskStatusSummary | null;
}) {
  const { t } = useTranslation();
  const current = summary?.active_error;
  const identity =
    current?.scope === "session" ? `${taskId}:${current.session_id}:${current.stamp}` : null;
  const previous = useRef({ taskId, identity, ready: Boolean(summary) });
  const seen = useRef(new Set(identity ? [identity] : []));
  const [announcement, setAnnouncement] = useState<string | null>(null);
  useEffect(() => {
    if (!previous.current.ready || previous.current.taskId !== taskId) {
      seen.current = new Set(identity ? [identity] : []);
      setAnnouncement(null);
    } else if (previous.current.identity !== identity) {
      const unseen = identity && !seen.current.has(identity);
      setAnnouncement(unseen ? identity : null);
      if (identity) seen.current.add(identity);
    }
    previous.current = { taskId, identity, ready: Boolean(summary) };
  }, [taskId, identity, summary]);

  return (
    <span
      className="sr-only"
      role="status"
      aria-live="polite"
      data-testid="session-error-announcement"
    >
      {announcement ? <span key={announcement}>{t("task:sessionRecoveryFailed")}</span> : null}
    </span>
  );
}
