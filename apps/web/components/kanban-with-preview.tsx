"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useRouter } from "next/navigation";
import { KanbanBoard } from "./kanban-board";
import { TaskPreviewPanel } from "./task-preview-panel";
import { useKanbanPreview } from "@/hooks/use-kanban-preview";
import { useKanbanLayout } from "@/hooks/use-kanban-layout";
import { useTaskSession } from "@/hooks/use-task-session";
import { useAppStore } from "@/components/state-provider";
import { Task } from "./kanban-card";
import type { KanbanState } from "@/lib/state/slices";
import { PREVIEW_PANEL } from "@/lib/settings/constants";
import { linkToSession } from "@/lib/links";

type KanbanWithPreviewProps = {
  initialTaskId?: string;
  initialSessionId?: string;
};

function useUrlSync(selectedTaskId: string | null, selectedTaskSessionId: string | null) {
  useEffect(() => {
    if (typeof window === "undefined") return;

    const url = new URL(window.location.href);
    if (selectedTaskId) {
      url.searchParams.set("taskId", selectedTaskId);
    } else {
      url.searchParams.delete("taskId");
      url.searchParams.delete("sessionId");
    }

    if (selectedTaskId && selectedTaskSessionId) {
      url.searchParams.set("sessionId", selectedTaskSessionId);
    }

    window.history.replaceState({}, "", url.toString());
  }, [selectedTaskId, selectedTaskSessionId]);
}

function useEscapeKey(isOpen: boolean, close: () => void) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        close();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, close]);
}

function useResizeHandler(
  isResizingRef: React.RefObject<boolean>,
  previewWidthPx: number,
  updatePreviewWidth: (width: number) => void,
) {
  return useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      (isResizingRef as React.MutableRefObject<boolean>).current = true;

      const startX = e.clientX;
      const startWidth = previewWidthPx;

      const handleMouseMove = (moveEvent: MouseEvent) => {
        if (!isResizingRef.current) return;
        const deltaX = startX - moveEvent.clientX;
        updatePreviewWidth(startWidth + deltaX);
      };

      const handleMouseUp = () => {
        (isResizingRef as React.MutableRefObject<boolean>).current = false;
        window.removeEventListener("mousemove", handleMouseMove);
        window.removeEventListener("mouseup", handleMouseUp);
      };

      window.addEventListener("mousemove", handleMouseMove);
      window.addEventListener("mouseup", handleMouseUp);
    },
    [isResizingRef, previewWidthPx, updatePreviewWidth],
  );
}

export function KanbanWithPreview({ initialTaskId }: KanbanWithPreviewProps) {
  const router = useRouter();

  // Get tasks from the kanban store
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);

  const { selectedTaskId, isOpen, previewWidthPx, open, close, updatePreviewWidth } =
    useKanbanPreview({
      initialTaskId,
      onClose: () => {
        // Cleanup handled by close
      },
    });

  // Use custom hooks for layout and session management
  const { containerRef, shouldFloat, kanbanWidth } = useKanbanLayout(isOpen, previewWidthPx);
  const { sessionId: selectedTaskSessionId } = useTaskSession(selectedTaskId ?? null);

  // Track resize state
  const isResizingRef = useRef(false);

  // Compute selected task from kanbanTasks and selectedTaskId
  const selectedTask = useMemo(() => {
    if (!selectedTaskId || kanbanTasks.length === 0) return null;

    const task = kanbanTasks.find((t: KanbanState["tasks"][number]) => t.id === selectedTaskId);
    if (!task) return null;

    return {
      id: task.id,
      title: task.title,
      workflowStepId: task.workflowStepId,
      state: task.state,
      description: task.description,
      position: task.position,
      repositoryId: task.repositoryId,
    };
  }, [selectedTaskId, kanbanTasks]);

  // Close panel if selected task no longer exists
  useEffect(() => {
    if (isOpen && selectedTaskId && !selectedTask) {
      close();
    }
  }, [isOpen, selectedTaskId, selectedTask, close]);

  const handleNavigateToTask = useCallback(
    (task: Task, sessionId: string) => {
      router.push(linkToSession(sessionId));
    },
    [router],
  );

  useUrlSync(selectedTaskId ?? null, selectedTaskSessionId ?? null);

  const handlePreviewTaskWithData = useCallback(
    (task: Task) => {
      if (isOpen && selectedTaskId === task.id) {
        close();
      } else {
        open(task.id);
      }
    },
    [isOpen, selectedTaskId, open, close],
  );

  useEscapeKey(isOpen, close);

  const handleResizeMouseDown = useResizeHandler(isResizingRef, previewWidthPx, updatePreviewWidth);

  const activeSessionId = selectedTaskId ? selectedTaskSessionId : null;

  return (
    <div ref={containerRef} className="h-screen w-full flex flex-col bg-background relative">
      {shouldFloat ? (
        <FloatingPreviewLayout
          kanbanWidth={kanbanWidth}
          previewWidthPx={previewWidthPx}
          selectedTask={selectedTask}
          activeSessionId={activeSessionId}
          onPreviewTask={handlePreviewTaskWithData}
          onNavigateToTask={handleNavigateToTask}
          onClose={close}
          onResizeMouseDown={handleResizeMouseDown}
        />
      ) : (
        <InlinePreviewLayout
          kanbanWidth={kanbanWidth}
          previewWidthPx={previewWidthPx}
          isOpen={isOpen}
          selectedTask={selectedTask}
          activeSessionId={activeSessionId}
          onPreviewTask={handlePreviewTaskWithData}
          onNavigateToTask={handleNavigateToTask}
          onClose={close}
          onResizeMouseDown={handleResizeMouseDown}
        />
      )}
    </div>
  );
}

function ResizeHandle({ onMouseDown }: { onMouseDown: (e: React.MouseEvent) => void }) {
  return (
    <div
      className="w-1 bg-border hover:bg-primary cursor-col-resize flex-shrink-0 relative group"
      onMouseDown={onMouseDown}
    >
      <div className="absolute inset-y-0 -left-2 -right-2" />
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-8 bg-border group-hover:bg-primary rounded-full transition-colors" />
    </div>
  );
}

type PreviewLayoutProps = {
  kanbanWidth: number;
  previewWidthPx: number;
  selectedTask: Task | null;
  activeSessionId: string | null;
  onPreviewTask: (task: Task) => void;
  onNavigateToTask: (task: Task, sessionId: string) => void;
  onClose: () => void;
  onResizeMouseDown: (e: React.MouseEvent) => void;
};

function FloatingPreviewLayout({
  kanbanWidth,
  previewWidthPx,
  selectedTask,
  activeSessionId,
  onPreviewTask,
  onNavigateToTask,
  onClose,
  onResizeMouseDown,
}: PreviewLayoutProps) {
  return (
    <>
      <div className="flex-1 overflow-hidden" style={{ width: `${kanbanWidth}px` }}>
        <KanbanBoard onPreviewTask={onPreviewTask} onOpenTask={onNavigateToTask} />
      </div>
      <div
        className="fixed inset-0 bg-black/30 z-30"
        onClick={onClose}
        aria-label="Close preview"
      />
      <div
        className="fixed right-0 top-0 bottom-0 z-40 shadow-2xl bg-background flex"
        style={{
          width: `${previewWidthPx}px`,
          maxWidth: `${PREVIEW_PANEL.MAX_WIDTH_VW}vw`,
        }}
      >
        <ResizeHandle onMouseDown={onResizeMouseDown} />
        <div className="flex-1 min-w-0 overflow-hidden">
          <TaskPreviewPanel
            task={selectedTask}
            sessionId={activeSessionId}
            onClose={onClose}
            onMaximize={
              activeSessionId ? (task) => onNavigateToTask(task, activeSessionId) : undefined
            }
          />
        </div>
      </div>
    </>
  );
}

function InlinePreviewLayout({
  kanbanWidth,
  previewWidthPx,
  isOpen,
  selectedTask,
  activeSessionId,
  onPreviewTask,
  onNavigateToTask,
  onClose,
  onResizeMouseDown,
}: PreviewLayoutProps & { isOpen: boolean }) {
  return (
    <div className="flex-1 flex overflow-hidden">
      <div className="overflow-hidden" style={{ width: `${kanbanWidth}px` }}>
        <KanbanBoard onPreviewTask={onPreviewTask} onOpenTask={onNavigateToTask} />
      </div>
      {isOpen && (
        <div
          className="flex-shrink-0 border-l bg-background flex"
          style={{ width: `${previewWidthPx}px` }}
        >
          <ResizeHandle onMouseDown={onResizeMouseDown} />
          <div className="flex-1 min-w-0 overflow-hidden">
            <TaskPreviewPanel
              task={selectedTask}
              sessionId={activeSessionId}
              onClose={onClose}
              onMaximize={
                activeSessionId ? (task) => onNavigateToTask(task, activeSessionId) : undefined
              }
            />
          </div>
        </div>
      )}
    </div>
  );
}
