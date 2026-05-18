"use client";

import { useCallback } from "react";
import { IconChecklist } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import { replaceTaskUrl } from "@/lib/links";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { aggregatePRStatusColor } from "../pr-task-icon";
import type { TaskPR } from "@/lib/types/github";

type PRRowTaskIndicatorProps = {
  tasks: TaskPR[] | undefined;
};

function TaskTitle({ taskId, fallback }: { taskId: string; fallback: string }) {
  const taskData = useTaskById(taskId);
  const title = taskData?.title ?? fallback;
  const truncated = title.length > 40 ? title.slice(0, 40) + "…" : title;
  return <span className="truncate text-foreground/80">{truncated}</span>;
}

const buttonClass = cn(
  "inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs",
  "hover:bg-muted/70 transition-colors cursor-pointer w-fit max-w-full",
);

export function PRRowTaskIndicator({ tasks }: PRRowTaskIndicatorProps) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);

  const navigate = useCallback(
    (taskId: string) => {
      setActiveTask(taskId);
      replaceTaskUrl(taskId);
    },
    [setActiveTask],
  );

  if (!tasks || tasks.length === 0) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
        No task created yet
      </span>
    );
  }

  const color = aggregatePRStatusColor(tasks);
  const iconClass = cn("h-3 w-3 shrink-0", color);

  if (tasks.length === 1) {
    const primary = tasks[0];
    return (
      <button type="button" onClick={() => navigate(primary.task_id)} className={buttonClass}>
        <IconChecklist className={iconClass} />
        <TaskTitle taskId={primary.task_id} fallback={primary.pr_title} />
      </button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" className={buttonClass}>
          <IconChecklist className={iconClass} />
          <span className="text-foreground/80">Tasks</span>
          <Badge variant="outline" className="h-4 px-1 py-0 text-[10px] shrink-0">
            {tasks.length}
          </Badge>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {tasks.map((t) => (
          <DropdownMenuItem
            key={t.task_id}
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
