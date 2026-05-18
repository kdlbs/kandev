"use client";

import { useRouter } from "next/navigation";
import { IconChevronRight, IconLoader2 } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { selectLiveSessionForTask } from "@/lib/state/slices/session/selectors";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { StatusIcon } from "./status-icon";
import { ExecutionIndicator } from "../components/execution-indicator";

type TaskRowProps = {
  task: OfficeTask;
  level: number;
  hasChildren: boolean;
  expanded: boolean;
  onToggleExpand: (id: string) => void;
  agentName?: string;
};

export function TaskRow({
  task,
  level,
  hasChildren,
  expanded,
  onToggleExpand,
  agentName,
}: TaskRowProps) {
  const router = useRouter();
  // Show an animated yellow spinner instead of the static status icon
  // while any session for this task is RUNNING. Drives the "this task
  // is being worked on right now" affordance in the task list.
  const isRunning = useAppStore((s) => selectLiveSessionForTask(s, task.id) !== null);

  const handleClick = () => {
    router.push(`/office/tasks/${task.id}`);
  };

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleExpand(task.id);
  };

  return (
    <div
      className="flex items-center gap-2 px-4 py-2.5 text-sm rounded-md hover:bg-muted/60 transition-colors cursor-pointer"
      style={{ paddingLeft: `${16 + level * 24}px` }}
      onClick={handleClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === "Enter" && handleClick()}
    >
      {hasChildren ? (
        <button
          onClick={handleToggle}
          className="shrink-0 cursor-pointer p-0 border-0 bg-transparent"
          aria-label={expanded ? "Collapse" : "Expand"}
        >
          <IconChevronRight
            className={cn(
              "h-3.5 w-3.5 text-muted-foreground transition-transform",
              expanded && "rotate-90",
            )}
          />
        </button>
      ) : (
        <span className="w-3.5 shrink-0" />
      )}
      {isRunning ? (
        <IconLoader2
          className="h-4 w-4 shrink-0 animate-spin text-yellow-500"
          data-testid="task-row-running-spinner"
        />
      ) : (
        <StatusIcon status={task.status} className="h-4 w-4 shrink-0" />
      )}
      <span className="text-xs text-muted-foreground font-mono shrink-0">{task.identifier}</span>
      <span className="flex-1 truncate">{task.title}</span>
      {task.isSystem && (
        <span className="shrink-0 inline-flex items-center rounded-md border border-border px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          System
        </span>
      )}
      {Array.isArray(task.labels) && task.labels.length > 0 && (
        <span className="flex items-center gap-1 shrink-0">
          {(task.labels as Array<{ name: string; color: string } | string>).map((label) => {
            const name = typeof label === "string" ? label : label.name;
            const color = typeof label === "string" ? undefined : label.color;
            return (
              <span
                key={name}
                className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                style={color ? { backgroundColor: `${color}20`, color } : undefined}
              >
                {name}
              </span>
            );
          })}
        </span>
      )}
      <ExecutionIndicator status={task.status} />
      {agentName && (
        <span className="text-xs text-muted-foreground shrink-0 truncate max-w-[100px]">
          {agentName}
        </span>
      )}
      <span className="text-xs text-muted-foreground shrink-0">
        {formatRelativeTime(task.updatedAt)}
      </span>
    </div>
  );
}
