"use client";

import { IconArrowsMaximize, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { Task } from "./kanban-card";
import { TaskChatPanel } from "./task/task-chat-panel";
import { useTaskMessages } from "@/hooks/use-task-messages";
import { useTaskChatSession } from "@/hooks/use-task-chat-session";
import { getWebSocketClient } from "@/lib/ws/connection";

interface TaskPreviewPanelProps {
  task: Task | null;
  sessionId?: string | null;
  onClose: () => void;
  onMaximize?: (task: Task, sessionId: string) => void;
}

const AGENTS = [
  { id: "codex", label: "Codex" },
  { id: "claude", label: "Claude Code" },
];

export function TaskPreviewPanel({ task, sessionId = null, onClose, onMaximize }: TaskPreviewPanelProps) {
  const { taskSessionId, isTaskSessionWorking } = useTaskChatSession(task?.id ?? null);
  const activeSessionId = sessionId ?? taskSessionId;
  const isActiveSessionWorking = sessionId ? isTaskSessionWorking && sessionId === taskSessionId : isTaskSessionWorking;
  const { isLoading: isLoadingMessages } = useTaskMessages(task?.id ?? null, activeSessionId);

  const handleSendMessage = async (content: string) => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    if (!activeSessionId) {
      console.error("No active task session. Start an agent before sending a message.");
      return;
    }

    try {
      await client.request(
        "message.add",
        { task_id: task.id, task_session_id: activeSessionId, content },
        10000
      );
    } catch (error) {
      console.error("Failed to send message:", error);
    }
  };

  return (
    <div className="flex h-full w-full flex-col border-l bg-background">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-4 py-3">
        <h2 className="text-sm font-semibold truncate">
          {task?.title ?? "Task Chat"}
        </h2>
        <div className="flex items-center gap-1">
          {onMaximize && activeSessionId && task && (
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={() => onMaximize(task, activeSessionId)}
              title="Open full page"
            >
              <IconArrowsMaximize className="h-4 w-4" />
              <span className="sr-only">Open full page</span>
            </Button>
          )}
          <Button variant="ghost" size="icon" className="h-8 w-8 cursor-pointer" onClick={onClose}>
            <IconX className="h-4 w-4" />
            <span className="sr-only">Close preview</span>
          </Button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 min-h-0 p-4 flex flex-col">
        {task ? (
          <TaskChatPanel
            agents={AGENTS}
            onSend={handleSendMessage}
            isLoading={isLoadingMessages}
            isAgentWorking={isActiveSessionWorking}
            taskId={task.id}
            taskDescription={task.description}
          />
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            Select a task to start chatting
          </div>
        )}
      </div>
    </div>
  );
}
