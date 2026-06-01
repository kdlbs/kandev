"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { IconChecklist } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useKanbanSnapshots } from "@/hooks/domains/kanban/use-kanban-tasks";
import { cn } from "@/lib/utils";
import { linkToTask } from "@/lib/links";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import type { TaskPR } from "@/lib/types/github";

type PRRowTaskIndicatorProps = {
  tasks: TaskPR[] | undefined;
};

function useTaskStepTitle(workflowStepId: string | undefined): string | null {
  const snapshots = useKanbanSnapshots();
  if (!workflowStepId) return null;
  for (const snap of Object.values(snapshots)) {
    const found = snap.steps.find((s) => s.id === workflowStepId);
    if (found) return found.title;
  }
  return null;
}

function truncateTitle(title: string): string {
  return title.length > 40 ? title.slice(0, 40) + "…" : title;
}

function TaskTitle({ taskId, fallback }: { taskId: string; fallback: string }) {
  const taskData = useTaskById(taskId);
  const title = taskData?.title ?? fallback;
  return <span className="truncate text-foreground/80">{truncateTitle(title)}</span>;
}

function SingleTaskButton({
  task,
  onNavigate,
}: {
  task: TaskPR;
  onNavigate: (taskId: string) => void;
}) {
  const taskData = useTaskById(task.task_id);
  const stepTitle = useTaskStepTitle(taskData?.workflowStepId);
  const title = taskData?.title ?? task.pr_title;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          data-testid="pr-row-task-indicator-single"
          onClick={() => onNavigate(task.task_id)}
          className={buttonClass}
        >
          <IconChecklist className={iconClass} />
          <span className="truncate text-foreground/80">{truncateTitle(title)}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent>
        <div className="flex flex-col gap-0.5 text-xs">
          <span>Task: {title}</span>
          {stepTitle ? <span className="text-muted-foreground">Step: {stepTitle}</span> : null}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

const buttonClass = cn(
  "inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs",
  "hover:bg-muted/70 transition-colors cursor-pointer w-fit max-w-full",
);

const iconClass = "h-3 w-3 shrink-0 text-muted-foreground";

export function PRRowTaskIndicator({ tasks }: PRRowTaskIndicatorProps) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const router = useRouter();

  const navigate = useCallback(
    (taskId: string) => {
      setActiveTask(taskId);
      router.push(linkToTask(taskId));
    },
    [setActiveTask, router],
  );

  if (!tasks || tasks.length === 0) {
    return (
      <span
        data-testid="pr-row-task-indicator-empty"
        className="inline-flex items-center gap-1 text-xs text-muted-foreground"
      >
        No task created yet
      </span>
    );
  }

  if (tasks.length === 1) {
    return <SingleTaskButton task={tasks[0]} onNavigate={navigate} />;
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" data-testid="pr-row-task-indicator-multi" className={buttonClass}>
          <IconChecklist className={iconClass} />
          <span className="text-foreground/80">Tasks</span>
          <Badge variant="outline" className="h-4 px-1 py-0 text-[10px] shrink-0">
            {tasks.length}
          </Badge>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {tasks.map((t) => (
          // Use the TaskPR row id rather than task_id — multi-repo tasks can
          // produce more than one TaskPR row for the same PR, which would
          // collide on task_id.
          <DropdownMenuItem
            key={t.id}
            className="cursor-pointer"
            onSelect={() => navigate(t.task_id)}
          >
            <TaskTitle taskId={t.task_id} fallback={t.pr_title} />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
