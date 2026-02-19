"use client";

import { useMemo } from "react";
import { useDroppable } from "@dnd-kit/core";
import { KanbanCard, Task } from "./kanban-card";
import { Badge } from "@kandev/ui/badge";
import { cn, getRepositoryDisplayName } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";

export interface WorkflowStep {
  id: string;
  title: string;
  color: string;
  events?: {
    on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
}

interface KanbanColumnProps {
  step: WorkflowStep;
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveTask?: (task: Task, targetStepId: string) => void;
  steps?: WorkflowStep[];
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  hideHeader?: boolean;
}

export function KanbanColumn({
  step,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveTask,
  steps,
  showMaximizeButton,
  deletingTaskId,
  hideHeader = false,
}: KanbanColumnProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: step.id,
  });

  // Access repositories from store to pass repository names to cards
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspace).flat() as Repository[],
    [repositoriesByWorkspace],
  );

  // Helper function to get repository name for a task
  const getRepositoryName = (repositoryId?: string): string | null => {
    if (!repositoryId) return null;
    const repository = repositories.find((repo) => repo.id === repositoryId);
    return repository ? getRepositoryDisplayName(repository.local_path) : null;
  };

  return (
    <div
      ref={setNodeRef}
      className={cn(
        "flex flex-col flex-1 h-full min-w-0 px-3 py-2 sm:min-h-[200px]",
        "border-r border-dashed border-border/50 last:border-r-0",
        isOver && "bg-primary/5",
      )}
    >
      {/* Column Header */}
      {!hideHeader && (
        <div className="flex items-center justify-between pb-2 mb-3 px-1">
          <div className="flex items-center gap-2">
            <div className={cn("w-2 h-2 rounded-full", step.color)} />
            <h2 className="font-semibold text-sm">{step.title}</h2>
            <Badge variant="secondary" className="text-xs">
              {tasks.length}
            </Badge>
          </div>
        </div>
      )}

      {/* Tasks */}
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden space-y-2 px-1">
        {tasks.map((task) => (
          <KanbanCard
            key={task.id}
            task={task}
            repositoryName={getRepositoryName(task.repositoryId)}
            onClick={onPreviewTask}
            onOpenFullPage={onOpenTask}
            onEdit={onEditTask}
            onDelete={onDeleteTask}
            onMove={onMoveTask}
            steps={steps}
            showMaximizeButton={showMaximizeButton}
            isDeleting={deletingTaskId === task.id}
          />
        ))}
      </div>
    </div>
  );
}
