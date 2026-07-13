"use client";

import { useCallback } from "react";
import { useRouter } from "@/lib/routing/client-router";
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
import { cn } from "@/lib/utils";
import { linkToTask } from "@/lib/links";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import type { KanbanState } from "@/lib/state/slices";

export type TaskRowLink = {
  id: string;
  taskId: string;
  fallbackTitle: string;
};

type TaskRowIndicatorProps = {
  tasks: TaskRowLink[] | undefined;
  testIdPrefix: string;
  emptyLabel?: string;
};

function useTaskStepTitle(workflowStepId: string | undefined): string | null {
  return useAppStore((state) => {
    if (!workflowStepId) return null;
    const findIn = (steps: KanbanState["steps"]) =>
      steps.find((step) => step.id === workflowStepId)?.title ?? null;
    const activeTitle = findIn(state.kanban.steps);
    if (activeTitle) return activeTitle;
    for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
      const title = findIn(snapshot.steps);
      if (title) return title;
    }
    return null;
  });
}

function truncateTitle(title: string): string {
  return title.length > 40 ? `${title.slice(0, 40)}…` : title;
}

function TaskTitle({ task }: { task: TaskRowLink }) {
  const taskData = useTaskById(task.taskId);
  return (
    <span className="truncate text-foreground/80">
      {truncateTitle(taskData?.title ?? task.fallbackTitle)}
    </span>
  );
}

const buttonClass = cn(
  "inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs",
  "hover:bg-muted/70 transition-colors cursor-pointer w-fit max-w-full",
);
const iconClass = "h-3 w-3 shrink-0 text-muted-foreground";

function SingleTaskButton({
  task,
  testIdPrefix,
  onNavigate,
}: {
  task: TaskRowLink;
  testIdPrefix: string;
  onNavigate: (taskId: string) => void;
}) {
  const taskData = useTaskById(task.taskId);
  const stepTitle = useTaskStepTitle(taskData?.workflowStepId);
  const title = taskData?.title ?? task.fallbackTitle;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          data-testid={`${testIdPrefix}-single`}
          onClick={() => onNavigate(task.taskId)}
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

export function TaskRowIndicator({ tasks, testIdPrefix, emptyLabel }: TaskRowIndicatorProps) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const router = useRouter();
  const navigate = useCallback(
    (taskId: string) => {
      setActiveTask(taskId);
      router.push(linkToTask(taskId));
    },
    [router, setActiveTask],
  );

  if (!tasks || tasks.length === 0) {
    return emptyLabel ? (
      <span
        data-testid={`${testIdPrefix}-empty`}
        className="inline-flex items-center gap-1 text-xs text-muted-foreground"
      >
        {emptyLabel}
      </span>
    ) : null;
  }
  if (tasks.length === 1) {
    return <SingleTaskButton task={tasks[0]} testIdPrefix={testIdPrefix} onNavigate={navigate} />;
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" data-testid={`${testIdPrefix}-multi`} className={buttonClass}>
          <IconChecklist className={iconClass} />
          <span className="text-foreground/80">Tasks</span>
          <Badge variant="outline" className="h-4 px-1 py-0 text-[10px] shrink-0">
            {tasks.length}
          </Badge>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {tasks.map((task) => (
          <DropdownMenuItem
            key={task.id}
            className="cursor-pointer"
            onSelect={() => navigate(task.taskId)}
          >
            <TaskTitle task={task} />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
