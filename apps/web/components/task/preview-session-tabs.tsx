"use client";

import { useCallback, useMemo, useState, type MouseEvent } from "react";
import { TaskCreateDialog } from "../task-create-dialog";
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
  taskTitle: string;
  sessionId: string | null;
  onSessionChange: (sessionId: string | null) => void;
};

export function PreviewSessionTabs({
  taskId,
  taskTitle,
  sessionId,
  onSessionChange,
}: PreviewSessionTabsProps) {
  const { sessions, isLoaded, loadSessions } = useTaskSessions(taskId);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);

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

  const handleDeleteSession = useCallback(
    async (deletedId: string) => {
      const client = getWebSocketClient();
      if (!client) return;
      const remaining = sortedSessions.filter((s) => s.id !== deletedId);
      try {
        await client.request("session.delete", { session_id: deletedId }, 15000);
      } catch (error) {
        console.error("Failed to delete session:", error);
        return;
      }
      loadSessions(true);
      if (deletedId === activeSessionId) {
        // Always propagate — when `remaining` is empty, `next` is null and the
        // parent must clear its userSelectedSessionId so the URL doesn't keep
        // the deleted id.
        onSessionChange(pickActiveSessionId(remaining, null));
      }
    },
    [sortedSessions, activeSessionId, loadSessions, onSessionChange],
  );

  const tabs = useMemo<SessionTab[]>(
    () =>
      sortedSessions.map((session) => ({
        id: session.id,
        label: resolveAgentLabelFor(session, agentLabelsById),
        icon: getSessionStateIcon(session.state, "h-3 w-3"),
        closable: true,
        alwaysShowClose: session.id === activeSessionId,
        testId: `preview-session-tab-${session.id}`,
        closeTestId: `preview-session-tab-close-${session.id}`,
        onClose: (event: MouseEvent) => {
          event.stopPropagation();
          handleDeleteSession(session.id);
        },
      })),
    [sortedSessions, agentLabelsById, activeSessionId, handleDeleteSession],
  );

  if (!isLoaded && sortedSessions.length === 0) {
    return <PreviewLoadingState />;
  }

  if (sortedSessions.length === 0) {
    return (
      <div className="flex h-full flex-col">
        <EmptyTabBar onAdd={() => setShowNewSessionDialog(true)} />
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          No agents yet. Press + to start one.
        </div>
        <NewSessionDialog
          open={showNewSessionDialog}
          onOpenChange={setShowNewSessionDialog}
          taskId={taskId}
          taskTitle={taskTitle}
        />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col min-h-0" data-testid="preview-session-tabs">
      <div className="border-b px-2 py-1">
        <SessionTabs
          tabs={tabs}
          activeTab={activeSessionId ?? ""}
          onTabChange={onSessionChange}
          showAddButton
          onAddTab={() => setShowNewSessionDialog(true)}
        />
      </div>
      <div className="flex-1 min-h-0">
        {activeSession && (
          <PreviewSessionBody key={activeSession.id} session={activeSession} taskId={taskId} />
        )}
      </div>
      <NewSessionDialog
        open={showNewSessionDialog}
        onOpenChange={setShowNewSessionDialog}
        taskId={taskId}
        taskTitle={taskTitle}
      />
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

function EmptyTabBar({ onAdd }: { onAdd: () => void }) {
  return (
    <div className="flex items-center border-b px-2 py-1">
      <button
        type="button"
        onClick={onAdd}
        className="inline-flex items-center justify-center rounded-sm px-2 py-1 h-6 text-sm hover:bg-muted cursor-pointer"
      >
        +
      </button>
    </div>
  );
}

function NewSessionDialog({
  open,
  onOpenChange,
  taskId,
  taskTitle,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  taskTitle: string;
}) {
  return (
    <TaskCreateDialog
      open={open}
      onOpenChange={onOpenChange}
      mode="session"
      workspaceId={null}
      workflowId={null}
      defaultStepId={null}
      steps={[]}
      taskId={taskId}
      initialValues={{ title: taskTitle, description: "" }}
    />
  );
}
