"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { KanbanBoard } from "./kanban-board";
import { TaskPreviewPanel } from "./task-preview-panel";
import { useKanbanPreview } from "@/hooks/use-kanban-preview";
import { useAppStore } from "@/components/state-provider";
import { Task } from "./kanban-card";
import { PREVIEW_PANEL } from "@/lib/settings/constants";
import { getWebSocketClient } from "@/lib/ws/connection";
import { linkToSession } from "@/lib/links";

type KanbanWithPreviewProps = {
  initialTaskId?: string;
  initialSessionId?: string;
};

export function KanbanWithPreview({ initialTaskId, initialSessionId }: KanbanWithPreviewProps) {
  const router = useRouter();
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState<number>(0);
  const isResizingRef = useRef(false);

  // Get tasks from the kanban store
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);
  const taskSessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);

  const {
    selectedTaskId,
    isOpen,
    previewWidthPx,
    enablePreviewOnClick,
    open,
    close,
    setEnablePreviewOnClick,
    updatePreviewWidth,
  } = useKanbanPreview({
    initialTaskId,
    onClose: () => {
      // Cleanup handled by close
    },
  });
  // Use initialSessionId if provided, otherwise fetch from WebSocket
  const [selectedTaskSessionId, setSelectedTaskSessionId] = useState<string | null>(initialSessionId ?? null);
  const hasLoadedInitialSession = useRef(false);

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

  useEffect(() => {
    if (!selectedTaskId) {
      return;
    }

    // If we have an initial session ID and haven't loaded yet, skip the first fetch
    if (initialSessionId && !hasLoadedInitialSession.current) {
      hasLoadedInitialSession.current = true;
      return;
    }

    let isActive = true;
    const loadLatestSession = async () => {
      const client = getWebSocketClient();
      if (!client) {
        if (isActive) {
          setSelectedTaskSessionId(null);
        }
        return;
      }
      try {
        const response = await client.request<{ sessions: Array<{ id: string }> }>(
          "task.session.list",
          { task_id: selectedTaskId },
          10000
        );
        if (isActive) {
          setSelectedTaskSessionId(response.sessions[0]?.id ?? null);
        }
      } catch (error) {
        console.error("Failed to load task sessions:", error);
        if (isActive) {
          setSelectedTaskSessionId(null);
        }
      }
    };

    loadLatestSession();

    return () => {
      isActive = false;
    };
  }, [selectedTaskId, initialSessionId]);

  // Measure container width on mount and resize
  useEffect(() => {
    if (!containerRef.current) return;

    const updateWidth = () => {
      if (containerRef.current) {
        setContainerWidth(containerRef.current.offsetWidth);
      }
    };

    updateWidth();
    const resizeObserver = new ResizeObserver(updateWidth);
    resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
    };
  }, []);

  // Calculate if we should be in floating mode
  // Float when preview would make kanban less than minimum percentage
  const minKanbanWidthPx = (PREVIEW_PANEL.MIN_KANBAN_WIDTH_PERCENT / 100) * containerWidth;
  const availableForKanban = containerWidth - previewWidthPx;
  const shouldFloat = isOpen && containerWidth > 0 && availableForKanban < minKanbanWidthPx;

  // Close panel if selected task no longer exists
  useEffect(() => {
    if (isOpen && selectedTaskId && !selectedTask) {
      close();
    }
  }, [isOpen, selectedTaskId, selectedTask, close]);

  const handleNavigateToTask = useCallback(
    (task: Task, sessionId: string) => {
      close();
      router.push(linkToSession(sessionId));
    },
    [close, router]
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

  // Preview handler - task data is computed from selectedTaskId
  // Toggle behavior: clicking the same task closes the panel
  const handlePreviewTaskWithData = useCallback(
    async (task: Task) => {
      if (enablePreviewOnClick) {
        // Preview mode: open/toggle preview panel
        if (isOpen && selectedTaskId === task.id) {
          close();
        } else {
          open(task.id);
        }
      } else {
        // Navigate mode: go directly to session details
        // First check if we have session data in the store
        const sessionsFromStore = taskSessionsByTaskId[task.id];
        const sessionIdFromStore = sessionsFromStore?.[0]?.id;

        if (sessionIdFromStore) {
          // We have a session ID in the store - use it directly
          handleNavigateToTask(task, sessionIdFromStore);
          return;
        }

        // No session in store - fetch from backend
        const client = getWebSocketClient();
        if (!client) {
          // If no WebSocket client, fall back to opening preview
          open(task.id);
          return;
        }

        try {
          const response = await client.request<{ sessions: Array<{ id: string }> }>(
            "task.session.list",
            { task_id: task.id },
            10000
          );
          const sessionId = response.sessions[0]?.id;

          if (sessionId) {
            handleNavigateToTask(task, sessionId);
          } else {
            // If no session exists, fall back to opening preview
            open(task.id);
          }
        } catch (error) {
          console.error("Failed to load task sessions:", error);
          // Fall back to opening preview on error
          open(task.id);
        }
      }
    },
    [enablePreviewOnClick, isOpen, selectedTaskId, taskSessionsByTaskId, open, close, handleNavigateToTask]
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

  // Calculate kanban width - keep it at min width when floating to avoid repainting
  const kanbanWidth = shouldFloat
    ? minKanbanWidthPx
    : (isOpen ? containerWidth - previewWidthPx : containerWidth);

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
              enablePreviewOnClick={enablePreviewOnClick}
              onTogglePreviewOnClick={setEnablePreviewOnClick}
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
              enablePreviewOnClick={enablePreviewOnClick}
              onTogglePreviewOnClick={setEnablePreviewOnClick}
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
