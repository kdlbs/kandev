import { useAppStore } from "@/components/state-provider";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { selectSessionTabTitle } from "@/components/task/session-tab-title";
import { useTranslation } from "react-i18next";

export type SenderTaskInfo = {
  id: string;
  /** Captured task title survives sender rename, archive, or unload. */
  snapshotTitle: string;
  /** Sender session ID, when the message came from a specific session. */
  sessionId?: string;
  /** Sender session name captured at send time (empty when unnamed). */
  sessionName?: string;
};

const SENDER_TITLE_MAX = 24;

function truncateTitle(title: string): string {
  if (title.length <= SENDER_TITLE_MAX) return title;
  return title.slice(0, SENDER_TITLE_MAX - 1).trimEnd() + "…";
}

function useSameTaskSessionTitle(
  sender: SenderTaskInfo,
  destinationTaskId: string,
  isSameTaskMessage: boolean,
) {
  return useAppStore((state) => {
    if (!isSameTaskMessage || !sender.sessionId) return null;
    const session = state.taskSessions.items[sender.sessionId];
    if (session?.task_id !== destinationTaskId) return null;
    return selectSessionTabTitle(state, sender.sessionId);
  });
}

function useLiveSenderSessionName(sender: SenderTaskInfo) {
  return useAppStore((state) =>
    sender.sessionId ? (state.taskSessions.items[sender.sessionId]?.name ?? null) : null,
  );
}

function badgeLabel(
  isSameTaskMessage: boolean,
  fullTitle: string,
  sessionName: string,
  sameTaskSessionName: string,
): string {
  if (isSameTaskMessage) return truncateTitle(sameTaskSessionName);
  if (!sessionName) return truncateTitle(fullTitle);
  return `${truncateTitle(fullTitle)} · ${truncateTitle(sessionName)}`;
}

/** Resolves live sender identity, task context, and source-link accessibility text. */
export function useSenderTaskBadgeModel(sender: SenderTaskInfo, destinationTaskId: string) {
  const { t } = useTranslation();
  const liveTask = useTaskById(sender.id);
  const isSameTaskMessage = sender.id === destinationTaskId && Boolean(sender.sessionId);
  const liveSessionTitle = useSameTaskSessionTitle(sender, destinationTaskId, isSameTaskMessage);
  const liveSessionName = useLiveSenderSessionName(sender);
  const fullTitle = liveTask?.title || sender.snapshotTitle || t("task:unknownTaskFallback");
  const sessionName = liveSessionName ?? sender.sessionName ?? "";
  const sameTaskSessionName = liveSessionTitle || sender.sessionName || "";
  const senderSessionId = sender.sessionId ?? "";
  const sameTaskVisibleLabel =
    sameTaskSessionName ||
    t("task:agentSessionFallback", { sessionId: senderSessionId.slice(0, 8) });
  const sameTaskContextName =
    sameTaskSessionName || t("task:agentSessionFallback", { sessionId: senderSessionId });
  const sameTaskContext = t("task:sameTaskSessionContext", {
    sessionName: sameTaskContextName,
    fullTitle,
  });

  return {
    liveTask,
    isSameTaskMessage,
    fullTitle,
    sessionName,
    sameTaskContext,
    truncated: badgeLabel(isSameTaskMessage, fullTitle, sessionName, sameTaskVisibleLabel),
    sourceTaskAriaLabel: t("task:openSourceTask", { fullTitle }),
  };
}
