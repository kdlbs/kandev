"use client";

import { Badge } from "@kandev/ui/badge";
import { Separator } from "@kandev/ui/separator";
import { formatRelativeTime } from "@/lib/utils";
import type { Issue } from "./types";
import { StatusIcon } from "./status-icon";

type TaskPropertiesProps = {
  task: Issue;
};

function PropertyRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between py-2 border-b border-border/50">
      <span className="text-sm text-muted-foreground w-24 shrink-0">{label}</span>
      <div className="text-sm text-right">{children}</div>
    </div>
  );
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function StatusDisplay({ status }: { status: Issue["status"] }) {
  const labels: Record<Issue["status"], string> = {
    backlog: "Backlog",
    todo: "Todo",
    in_progress: "In Progress",
    in_review: "In Review",
    done: "Done",
    cancelled: "Cancelled",
    blocked: "Blocked",
  };
  return (
    <span className="flex items-center gap-1.5">
      <StatusIcon status={status} className="h-3.5 w-3.5" />
      {labels[status]}
    </span>
  );
}

function PriorityDisplay({ priority }: { priority: Issue["priority"] }) {
  const labels: Record<Issue["priority"], { label: string; className: string }> = {
    critical: { label: "Critical", className: "text-red-600" },
    high: { label: "High", className: "text-orange-600" },
    medium: { label: "Medium", className: "text-yellow-600" },
    low: { label: "Low", className: "text-blue-600" },
  };
  const config = labels[priority];
  return <span className={config.className}>{config.label}</span>;
}

export function TaskProperties({ task }: TaskPropertiesProps) {
  return (
    <div className="space-y-0">
      <PropertyRow label="Status">
        <StatusDisplay status={task.status} />
      </PropertyRow>
      <PropertyRow label="Priority">
        <PriorityDisplay priority={task.priority} />
      </PropertyRow>
      <PropertyRow label="Labels">
        {task.labels.length > 0 ? (
          <div className="flex flex-wrap gap-1 justify-end">
            {task.labels.map((label) => (
              <Badge key={label} variant="outline" className="text-xs">
                {label}
              </Badge>
            ))}
          </div>
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </PropertyRow>
      <PropertyRow label="Assignee">
        {task.assigneeName ?? <span className="text-muted-foreground">Unassigned</span>}
      </PropertyRow>
      <PropertyRow label="Project">
        {task.projectName ? (
          <span className="flex items-center gap-1.5 justify-end">
            {task.projectColor && (
              <span
                className="h-2.5 w-2.5 rounded-sm shrink-0"
                style={{ backgroundColor: task.projectColor }}
              />
            )}
            {task.projectName}
          </span>
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </PropertyRow>
      <PropertyRow label="Parent">
        {task.parentIdentifier ? (
          <span>
            {task.parentIdentifier} {task.parentTitle}
          </span>
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </PropertyRow>

      <Separator className="my-2" />

      <PropertyRow label="Blocked by">
        {task.blockedBy.length > 0 ? (
          task.blockedBy.join(", ")
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </PropertyRow>
      <PropertyRow label="Blocking">
        {task.blocking.length > 0 ? (
          task.blocking.join(", ")
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </PropertyRow>
      <PropertyRow label="Sub-issues">
        {task.children.length > 0 ? (
          <span>{task.children.length} sub-issues</span>
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </PropertyRow>

      <Separator className="my-2" />

      <PropertyRow label="Reviewers">
        {task.reviewers.length > 0 ? (
          task.reviewers.join(", ")
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </PropertyRow>
      <PropertyRow label="Approvers">
        {task.approvers.length > 0 ? (
          task.approvers.join(", ")
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </PropertyRow>

      <Separator className="my-2" />

      <PropertyRow label="Created by">{task.createdBy}</PropertyRow>
      <PropertyRow label="Started">
        {task.startedAt ? (
          formatDate(task.startedAt)
        ) : (
          <span className="text-muted-foreground">--</span>
        )}
      </PropertyRow>
      <PropertyRow label="Completed">
        {task.completedAt ? (
          formatDate(task.completedAt)
        ) : (
          <span className="text-muted-foreground">--</span>
        )}
      </PropertyRow>
      <PropertyRow label="Created">{formatDate(task.createdAt)}</PropertyRow>
      <PropertyRow label="Updated">{formatRelativeTime(task.updatedAt)}</PropertyRow>
    </div>
  );
}
