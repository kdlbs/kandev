"use client";

import { IconArrowsMaximize, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import type { Task } from "./kanban-card";
import { PreviewSessionTabs } from "./task/preview-session-tabs";

interface TaskPreviewPanelProps {
  task: Task | null;
  sessionId?: string | null;
  ensureSession?: UseEnsureTaskSessionResult;
  onClose: () => void;
  onMaximize?: (task: Task) => void;
  onSessionChange?: (sessionId: string | null) => void;
}

export function TaskPreviewPanel({
  task,
  sessionId = null,
  ensureSession,
  onClose,
  onMaximize,
  onSessionChange,
}: TaskPreviewPanelProps) {
  return (
    <div
      data-testid="task-preview-panel"
      className="flex h-full w-full flex-col border-l bg-background"
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b px-4 py-3">
        <h2 className="text-sm font-semibold truncate">{task?.title ?? "Task Chat"}</h2>
        <div className="flex items-center gap-1">
          {onMaximize && task && (
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={() => onMaximize(task)}
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
      <div className="flex-1 min-h-0 flex flex-col">
        {task ? (
          <PreviewSessionTabs
            taskId={task.id}
            sessionId={sessionId}
            ensureSession={ensureSession}
            onSessionChange={onSessionChange}
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
