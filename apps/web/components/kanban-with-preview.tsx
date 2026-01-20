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
import { PREVIEW_PANEL } from "@/lib/settings/constants";
import { linkToSession } from "@/lib/links";

type KanbanWithPreviewProps = {
  initialTaskId?: string;
  initialSessionId?: string;
};

export function KanbanWithPreview({ initialTaskId }: KanbanWithPreviewProps) {
  const router = useRouter();

  // Get tasks from the kanban store
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);

  const {
    selectedTaskId,
    isOpen,
    previewWidthPx,
    open,
    close,
    updatePreviewWidth,
  } = useKanbanPreview({
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

    const task = kanbanTasks.find((t) => t.id === selectedTaskId);
    if (!task) return null;

    return {
      id: task.id,
      title: task.title,
      columnId: task.columnId,
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
    [router]
  );

  // Update URL query params when task selection changes
  useEffect(() => {
    if (typeof window === 'undefined') return;

    const url = new URL(window.location.href);
    if (selectedTaskId) {
      url.searchParams.set('taskId', selectedTaskId);
    } else {
      url.searchParams.delete('taskId');
      url.searchParams.delete('sessionId');
    }

    // Update sessionId param when it's available
    if (selectedTaskId && selectedTaskSessionId) {
      url.searchParams.set('sessionId', selectedTaskSessionId);
    }

    // Use replaceState to avoid adding to browser history on every click
    window.history.replaceState({}, '', url.toString());
  }, [selectedTaskId, selectedTaskSessionId]);

  // Preview handler - opens/toggles the preview panel
  // Toggle behavior: clicking the same task closes the panel
  const handlePreviewTaskWithData = useCallback(
    (task: Task) => {
      // Toggle preview: if already open for this task, close it; otherwise open
      if (isOpen && selectedTaskId === task.id) {
        close();
      } else {
        open(task.id);
      }
    },
    [isOpen, selectedTaskId, open, close]
  );

  // Handle Escape key to close preview
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        close();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, close]);

  // Unified resize handler for both inline and floating modes
  const handleResizeMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    isResizingRef.current = true;

    const startX = e.clientX;
    const startWidth = previewWidthPx;

    const handleMouseMove = (moveEvent: MouseEvent) => {
      if (!isResizingRef.current) return;

      // Calculate new width (moving left increases width)
      const deltaX = startX - moveEvent.clientX;
      const newWidth = startWidth + deltaX;

      // No maximum limit - can resize all the way to the left
      // updatePreviewWidth enforces minimum width
      updatePreviewWidth(newWidth);
    };

    const handleMouseUp = () => {
      isResizingRef.current = false;
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("mouseup", handleMouseUp);
    };

    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("mouseup", handleMouseUp);
  }, [previewWidthPx, updatePreviewWidth]);

  const activeSessionId = selectedTaskId ? selectedTaskSessionId : null;

  return (
    <div ref={containerRef} className="h-screen w-full flex flex-col bg-background relative">
      {shouldFloat ? (
        // Floating mode: kanban at min width, preview overlays
        <>
          <div
            className="flex-1 overflow-hidden"
            style={{
              width: `${kanbanWidth}px`,
            }}
          >
            <KanbanBoard
              onPreviewTask={handlePreviewTaskWithData}
              onOpenTask={handleNavigateToTask}
            />
          </div>

          {/* Backdrop */}
          <div
            className="fixed inset-0 bg-black/30 z-30"
            onClick={close}
            aria-label="Close preview"
          />

          {/* Floating Panel */}
          <div
            className="fixed right-0 top-0 bottom-0 z-40 shadow-2xl bg-background flex"
            style={{
              width: `${previewWidthPx}px`,
              maxWidth: `${PREVIEW_PANEL.MAX_WIDTH_VW}vw`,
            }}
          >
            {/* Resize handle on the left edge */}
            <div
              className="w-1 bg-border hover:bg-primary cursor-col-resize flex-shrink-0 relative group"
              onMouseDown={handleResizeMouseDown}
            >
              <div className="absolute inset-y-0 -left-2 -right-2" />
              <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-8 bg-border group-hover:bg-primary rounded-full transition-colors" />
            </div>

            {/* Panel content */}
            <div className="flex-1 min-w-0 overflow-hidden">
              <TaskPreviewPanel
                task={selectedTask}
                sessionId={activeSessionId}
                onClose={close}
                onMaximize={
                  activeSessionId
                    ? (task) => handleNavigateToTask(task, activeSessionId)
                    : undefined
                }
              />
            </div>
          </div>
        </>
      ) : (
        // Inline mode: side by side with custom resize
        <div className="flex-1 flex overflow-hidden">
          {/* Kanban Board */}
          <div
            className="overflow-hidden"
            style={{
              width: `${kanbanWidth}px`,
            }}
          >
            <KanbanBoard
              onPreviewTask={handlePreviewTaskWithData}
              onOpenTask={handleNavigateToTask}
            />
          </div>

          {/* Preview Panel */}
          {isOpen && (
            <div
              className="flex-shrink-0 border-l bg-background flex"
              style={{
                width: `${previewWidthPx}px`,
              }}
            >
              {/* Resize handle */}
              <div
                className="w-1 bg-border hover:bg-primary cursor-col-resize flex-shrink-0 relative group"
                onMouseDown={handleResizeMouseDown}
              >
                <div className="absolute inset-y-0 -left-2 -right-2" />
                <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-8 bg-border group-hover:bg-primary rounded-full transition-colors" />
              </div>

              {/* Panel content */}
              <div className="flex-1 min-w-0 overflow-hidden">
                <TaskPreviewPanel
                  task={selectedTask}
                  sessionId={activeSessionId}
                  onClose={close}
                  onMaximize={
                    activeSessionId
                      ? (task) => handleNavigateToTask(task, activeSessionId)
                      : undefined
                  }
                />
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
