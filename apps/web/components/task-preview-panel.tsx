"use client";

import { IconX, IconCircleCheck, IconCircleX, IconLoader2, IconAlertTriangle, IconArrowsMaximize } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { getRepositoryDisplayName } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { useMemo } from "react";
import type { Task } from "./kanban-card";

interface TaskPreviewPanelProps {
  task: Task | null;
  onClose: () => void;
  onMaximize?: (task: Task) => void;
}

function getStateDisplay(state?: string) {
  switch (state) {
    case "IN_PROGRESS":
    case "SCHEDULING":
      return {
        label: state === "SCHEDULING" ? "Scheduling" : "In Progress",
        icon: <IconLoader2 className="h-3 w-3 animate-spin" />,
        variant: "default" as const,
      };
    case "COMPLETED":
      return {
        label: "Completed",
        icon: <IconCircleCheck className="h-3 w-3" />,
        variant: "default" as const,
        className: "bg-emerald-500/10 text-emerald-500 border-emerald-500/20",
      };
    case "FAILED":
    case "CANCELLED":
      return {
        label: state === "FAILED" ? "Failed" : "Cancelled",
        icon: <IconCircleX className="h-3 w-3" />,
        variant: "destructive" as const,
      };
    case "BLOCKED":
    case "WAITING_FOR_INPUT":
      return {
        label: state === "BLOCKED" ? "Blocked" : "Waiting for Input",
        icon: <IconAlertTriangle className="h-3 w-3" />,
        variant: "default" as const,
        className: "bg-yellow-500/10 text-yellow-500 border-yellow-500/20",
      };
    case "TODO":
      return {
        label: "To Do",
        icon: null,
        variant: "secondary" as const,
      };
    case "REVIEW":
      return {
        label: "Review",
        icon: null,
        variant: "default" as const,
      };
    case "CREATED":
    default:
      return {
        label: "Created",
        icon: null,
        variant: "secondary" as const,
      };
  }
}

export function TaskPreviewPanel({ task, onClose, onMaximize }: TaskPreviewPanelProps) {
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repository = useMemo(() => {
    if (!task?.repositoryId) return null;
    return Object.values(repositoriesByWorkspace)
      .flat()
      .find((repo) => repo.id === task.repositoryId) ?? null;
  }, [repositoriesByWorkspace, task]);
  const repoName = getRepositoryDisplayName(repository?.local_path);

  if (!task) {
    return (
      <div className="flex h-full w-full flex-col border-l bg-background">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <h2 className="text-sm font-semibold">Task Preview</h2>
          <Button variant="ghost" size="icon" className="h-8 w-8 cursor-pointer" onClick={onClose}>
            <IconX className="h-4 w-4" />
            <span className="sr-only">Close preview</span>
          </Button>
        </div>
        <div className="flex flex-1 items-center justify-center p-8">
          <p className="text-sm text-muted-foreground">Select a task to preview</p>
        </div>
      </div>
    );
  }

  const stateDisplay = getStateDisplay(task.state);

  return (
    <div className="flex h-full w-full flex-col border-l bg-background">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-4 py-3">
        <h2 className="text-sm font-semibold">Task Preview</h2>
        <div className="flex items-center gap-1">
          {onMaximize && (
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
      <div className="flex-1 overflow-y-auto p-4">
        <div className="space-y-4">
          {/* Repository */}
          {repoName && (
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1">Repository</p>
              <p className="text-sm">{repoName}</p>
            </div>
          )}

          {/* Title */}
          <div>
            <p className="text-xs font-medium text-muted-foreground mb-1">Title</p>
            <h3 className="text-base font-semibold">{task.title}</h3>
          </div>

          {/* State */}
          {task.state && (
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1">Status</p>
              <Badge variant={stateDisplay.variant} className={stateDisplay.className}>
                {stateDisplay.icon && <span className="mr-1">{stateDisplay.icon}</span>}
                {stateDisplay.label}
              </Badge>
            </div>
          )}

          {/* Description */}
          {task.description && (
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1">Description</p>
              <p className="text-sm text-muted-foreground whitespace-pre-wrap">
                {task.description}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
