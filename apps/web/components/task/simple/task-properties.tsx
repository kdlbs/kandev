"use client";

import { useEffect, useState } from "react";
import { Separator } from "@kandev/ui/separator";
import { formatRelativeTime } from "@/lib/utils";
import { getTreeCostSummary, type TreeCostSummary } from "@/lib/api/domains/tree-api";
import type { Task } from "@/app/office/tasks/[id]/types";
import { PropertyRow } from "./components/property-row";
import { StatusPicker } from "./components/status-picker";
import { PriorityPicker } from "./components/priority-picker";
import { LabelsPicker } from "./components/labels-picker";
import { AssigneePicker } from "./components/assignee-picker";
import { ProjectPicker } from "./components/project-picker";
import { ParentPicker } from "./components/parent-picker";
import { BlockersPicker } from "./components/blockers-picker";
import { SubIssuesRow } from "./components/sub-issues-row";
import { ReviewersPicker } from "./components/reviewers-picker";
import { ApproversPicker } from "./components/approvers-picker";
import { PendingApprovalBadge } from "./components/pending-approval-badge";

type TaskPropertiesProps = {
  task: Task;
};

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

// formatCurrency converts subcents (hundredths of a cent — the office
// cost storage unit per docs/specs/office-costs/spec.md) to a USD string.
function formatCurrency(subcents: number): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "USD",
  }).format(subcents / 10000);
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value);
}

const NONE_LABEL = <span className="text-muted-foreground">None</span>;

function IdentitySection({ task }: { task: Task }) {
  return (
    <>
      <PropertyRow label="Status" valueClassName="ml-auto">
        <span className="flex items-center gap-2 ml-auto">
          <PendingApprovalBadge task={task} />
          <StatusPicker task={task} />
        </span>
      </PropertyRow>
      <PropertyRow label="Priority" valueClassName="ml-auto">
        <PriorityPicker task={task} />
      </PropertyRow>
      <PropertyRow label="Labels" valueClassName="ml-auto" alignStart>
        <LabelsPicker task={task} />
      </PropertyRow>
      <PropertyRow label="Assignee" valueClassName="ml-auto">
        <AssigneePicker task={task} />
      </PropertyRow>
      <PropertyRow label="Project" valueClassName="ml-auto">
        <ProjectPicker task={task} />
      </PropertyRow>
      <PropertyRow label="Parent" valueClassName="ml-auto">
        <ParentPicker task={task} />
      </PropertyRow>
    </>
  );
}

function DependenciesSection({ task }: { task: Task }) {
  return (
    <>
      <PropertyRow label="Blocked by" valueClassName="ml-auto" alignStart>
        <BlockersPicker task={task} />
      </PropertyRow>
      <PropertyRow label="Blocking">
        {task.blocking.length > 0 ? task.blocking.join(", ") : NONE_LABEL}
      </PropertyRow>
      <PropertyRow label="Sub-issues" alignStart>
        <SubIssuesRow task={task} />
      </PropertyRow>
    </>
  );
}

function ReviewSection({ task }: { task: Task }) {
  return (
    <>
      <PropertyRow label="Reviewers" valueClassName="ml-auto" alignStart>
        <ReviewersPicker task={task} />
      </PropertyRow>
      <PropertyRow label="Approvers" valueClassName="ml-auto" alignStart>
        <ApproversPicker task={task} />
      </PropertyRow>
    </>
  );
}

function TimelineSection({ task }: { task: Task }) {
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

function TreeCostSection({ task }: { task: Task }) {
  const [summary, setSummary] = useState<TreeCostSummary | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (task.children.length === 0) {
      return;
    }
    getTreeCostSummary(task.id)
      .then((res) => {
        if (!cancelled) setSummary(res);
      })
      .catch(() => {
        if (!cancelled) setSummary(null);
      });
    return () => {
      cancelled = true;
    };
  }, [task.id, task.children.length]);

  if (task.children.length === 0 || !summary) return null;

  return (
    <>
      <Separator className="my-2" />
      <PropertyRow label="Tree cost">{formatCurrency(summary.cost_subcents)}</PropertyRow>
      <PropertyRow label="Tree tasks">{summary.task_count}</PropertyRow>
      <PropertyRow label="Input tokens">{formatNumber(summary.tokens_in)}</PropertyRow>
      <PropertyRow label="Cached input">{formatNumber(summary.tokens_cached_in)}</PropertyRow>
      <PropertyRow label="Output tokens">{formatNumber(summary.tokens_out)}</PropertyRow>
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
      <TreeCostSection task={task} />
    </div>
  );
}
