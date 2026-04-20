"use client";

import { useCallback, useMemo } from "react";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { useAppStore } from "@/components/state-provider";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { getSessionStateIcon } from "@/lib/ui/state-icons";
import type { TaskSession } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";
import { PassthroughTerminal } from "./passthrough-terminal";
import { TaskChatPanel } from "./task-chat-panel";
import {
  buildAgentLabelsById,
  pickActiveSessionId,
  resolveAgentLabelFor,
  sortSessions,
} from "./session-sort";

type PreviewSessionTabsProps = {
  taskId: string;
  sessionId: string | null;
  onSessionChange: (sessionId: string | null) => void;
};

/**
 * Read-only session tabs for the kanban preview panel.
 *
 * Tabs only switch between existing sessions — creating or deleting sessions
 * is deliberately restricted to the full-page task view.
 */
export function PreviewSessionTabs({
  taskId,
  sessionId,
  onSessionChange,
}: PreviewSessionTabsProps) {
  const { sessions, isLoaded } = useTaskSessions(taskId);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);

  const sortedSessions = useMemo(() => sortSessions(sessions), [sessions]);
  const agentLabelsById = useMemo(() => buildAgentLabelsById(agentProfiles), [agentProfiles]);

  const activeSessionId = useMemo(
    () => pickActiveSessionId(sortedSessions, sessionId),
    [sortedSessions, sessionId],
  );
  const activeSession = useMemo(
    () => sortedSessions.find((s) => s.id === activeSessionId) ?? null,
    [sortedSessions, activeSessionId],
  );

  const tabs = useMemo<SessionTab[]>(
    () =>
      sortedSessions.map((session) => ({
        id: session.id,
        label: resolveAgentLabelFor(session, agentLabelsById),
        icon: getSessionStateIcon(session.state, "h-3 w-3"),
        testId: `preview-session-tab-${session.id}`,
      })),
    [sortedSessions, agentLabelsById],
  );

  if (!isLoaded && sortedSessions.length === 0) {
    return <PreviewLoadingState />;
  }

  if (sortedSessions.length === 0) {
    return (
      <div className="flex h-full flex-col">
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          No agents yet.
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col min-h-0" data-testid="preview-session-tabs">
      <div className="border-b px-2 py-1">
        <SessionTabs tabs={tabs} activeTab={activeSessionId ?? ""} onTabChange={onSessionChange} />
      </div>
      <div className="flex-1 min-h-0">
        {activeSession && (
          <PreviewSessionBody key={activeSession.id} session={activeSession} taskId={taskId} />
        )}
      </div>
    </div>
  );
}

function PreviewSessionBody({ session, taskId }: { session: TaskSession; taskId: string }) {
  const handleSendMessage = useCallback(
    async (content: string) => {
      const client = getWebSocketClient();
      if (!client) return;
      try {
        await client.request(
          "message.add",
          { task_id: taskId, session_id: session.id, content },
          10000,
        );
      } catch (error) {
        console.error("Failed to send message:", error);
      }
    },
    [taskId, session.id],
  );

  if (session.is_passthrough) {
    return (
      <div className="h-full bg-card">
        <PassthroughTerminal sessionId={session.id} mode="agent" />
      </div>
    );
  }

  return (
    <div className="h-full p-4 flex flex-col">
      <TaskChatPanel onSend={handleSendMessage} sessionId={session.id} hideSessionsDropdown />
    </div>
  );
}

function PreviewLoadingState() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      Loading agents…
    </div>
  );
}
