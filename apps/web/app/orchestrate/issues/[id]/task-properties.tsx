"use client";

import { Badge } from "@kandev/ui/badge";
import { Separator } from "@kandev/ui/separator";
import { useAppStore } from "@/components/state-provider";
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

const FALLBACK_STATUS_LABELS: Record<string, string> = {
  backlog: "Backlog",
  todo: "Todo",
  in_progress: "In Progress",
  in_review: "In Review",
  done: "Done",
  cancelled: "Cancelled",
  blocked: "Blocked",
};

const FALLBACK_PRIORITY_CONFIG: Record<string, { label: string; className: string }> = {
  critical: { label: "Critical", className: "text-red-600" },
  high: { label: "High", className: "text-orange-600" },
  medium: { label: "Medium", className: "text-yellow-600" },
  low: { label: "Low", className: "text-blue-600" },
};

function StatusDisplay({ status }: { status: Issue["status"] }) {
  const meta = useAppStore((s) => s.orchestrate.meta);
  const metaStatus = meta?.statuses.find((s) => s.id === status);
  const label = metaStatus?.label ?? FALLBACK_STATUS_LABELS[status] ?? status;
  return (
    <span className="flex items-center gap-1.5">
      <StatusIcon status={status} className="h-3.5 w-3.5" />
      {label}
    </span>
  );
}

function PriorityDisplay({ priority }: { priority: Issue["priority"] }) {
  const meta = useAppStore((s) => s.orchestrate.meta);
  const metaPriority = meta?.priorities.find((p) => p.id === priority);
  const config = metaPriority
    ? { label: metaPriority.label, className: metaPriority.color }
    : FALLBACK_PRIORITY_CONFIG[priority] ?? { label: priority, className: "" };
  return <span className={config.className}>{config.label}</span>;
}

const NONE_LABEL = <span className="text-muted-foreground">None</span>;

function IdentitySection({ task }: { task: Issue }) {
  return (
    <>
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
          NONE_LABEL
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
          NONE_LABEL
        )}
      </PropertyRow>
      <PropertyRow label="Parent">
        {task.parentIdentifier ? (
          <span>
            {task.parentIdentifier} {task.parentTitle}
          </span>
        ) : (
          NONE_LABEL
        )}
      </PropertyRow>
    </>
  );
}

function DependenciesSection({ task }: { task: Issue }) {
  return (
    <>
      <PropertyRow label="Blocked by">
        {task.blockedBy.length > 0 ? task.blockedBy.join(", ") : NONE_LABEL}
      </PropertyRow>
      <PropertyRow label="Blocking">
        {task.blocking.length > 0 ? task.blocking.join(", ") : NONE_LABEL}
      </PropertyRow>
      <PropertyRow label="Sub-issues">
        {task.children.length > 0 ? <span>{task.children.length} sub-issues</span> : NONE_LABEL}
      </PropertyRow>
    </>
  );
}

function ReviewSection({ task }: { task: Issue }) {
  return (
    <>
      <PropertyRow label="Reviewers">
        {task.reviewers.length > 0 ? task.reviewers.join(", ") : NONE_LABEL}
      </PropertyRow>
      <PropertyRow label="Approvers">
        {task.approvers.length > 0 ? task.approvers.join(", ") : NONE_LABEL}
      </PropertyRow>
    </>
  );
}

function TimelineSection({ task }: { task: Issue }) {
  const dateOrDash = (d?: string | null) =>
    d ? formatDate(d) : <span className="text-muted-foreground">--</span>;
  return (
    <>
      <PropertyRow label="Created by">{task.createdBy}</PropertyRow>
      <PropertyRow label="Started">{dateOrDash(task.startedAt)}</PropertyRow>
      <PropertyRow label="Completed">{dateOrDash(task.completedAt)}</PropertyRow>
      <PropertyRow label="Created">{formatDate(task.createdAt)}</PropertyRow>
      <PropertyRow label="Updated">{formatRelativeTime(task.updatedAt)}</PropertyRow>
    </>
  );
}

export function TaskProperties({ task }: TaskPropertiesProps) {
  return (
    <div className="space-y-0">
      <IdentitySection task={task} />
      <Separator className="my-2" />
      <DependenciesSection task={task} />
      <Separator className="my-2" />
      <ReviewSection task={task} />
      <Separator className="my-2" />
      <TimelineSection task={task} />
    </div>
  );
}
