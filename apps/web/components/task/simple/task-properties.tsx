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
import { useTranslation } from "react-i18next";

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

// A component rather than a module-level constant: `t()` at module scope
// resolves at import, before a locale is active, and never updates on a
// locale switch. See `check-module-scope-t.mjs`.
function NoneLabel() {
  const { t } = useTranslation();
  return <span className="text-muted-foreground">{t("task:none")}</span>;
}

function IdentitySection({ task }: { task: Task }) {
  const { t } = useTranslation();
  return (
    <>
      <PropertyRow label={t("common:status")} valueClassName="ml-auto">
        <span className="flex items-center gap-2 ml-auto">
          <PendingApprovalBadge task={task} />
          <StatusPicker task={task} />
        </span>
      </PropertyRow>
      <PropertyRow label={t("task:priority")} valueClassName="ml-auto">
        <PriorityPicker task={task} />
      </PropertyRow>
      <PropertyRow label={t("task:labels")} valueClassName="ml-auto" alignStart>
        <LabelsPicker task={task} />
      </PropertyRow>
      <PropertyRow label={t("task:assignee")} valueClassName="ml-auto">
        <AssigneePicker task={task} />
      </PropertyRow>
      <PropertyRow label={t("task:project")} valueClassName="ml-auto">
        <ProjectPicker task={task} />
      </PropertyRow>
      <PropertyRow label={t("task:parent")} valueClassName="ml-auto">
        <ParentPicker task={task} />
      </PropertyRow>
    </>
  );
}

function DependenciesSection({ task }: { task: Task }) {
  const { t } = useTranslation();
  return (
    <>
      <PropertyRow label={t("task:relatedBlockedBy")} valueClassName="ml-auto" alignStart>
        <BlockersPicker task={task} />
      </PropertyRow>
      <PropertyRow label={t("task:blocking")}>
        {task.blocking.length > 0 ? task.blocking.join(", ") : <NoneLabel />}
      </PropertyRow>
      <PropertyRow label={t("task:subIssues")} alignStart>
        <SubIssuesRow task={task} />
      </PropertyRow>
    </>
  );
}

function ReviewSection({ task }: { task: Task }) {
  const { t } = useTranslation();
  return (
    <>
      <PropertyRow label={t("task:reviewers")} valueClassName="ml-auto" alignStart>
        <ReviewersPicker task={task} />
      </PropertyRow>
      <PropertyRow label={t("task:approvers")} valueClassName="ml-auto" alignStart>
        <ApproversPicker task={task} />
      </PropertyRow>
    </>
  );
}

function TimelineSection({ task }: { task: Task }) {
  const { t } = useTranslation();
  const dateOrDash = (d?: string | null) =>
    d ? formatDate(d) : <span className="text-muted-foreground">--</span>;
  return (
    <>
      <PropertyRow label={t("task:createdBy")}>{task.createdBy}</PropertyRow>
      <PropertyRow label={t("task:started")}>{dateOrDash(task.startedAt)}</PropertyRow>
      <PropertyRow label={t("task:completed")}>{dateOrDash(task.completedAt)}</PropertyRow>
      <PropertyRow label={t("task:created")}>{formatDate(task.createdAt)}</PropertyRow>
      <PropertyRow label={t("task:updated")}>{formatRelativeTime(task.updatedAt)}</PropertyRow>
    </>
  );
}

function TreeCostSection({ task }: { task: Task }) {
  const { t } = useTranslation();
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
      <PropertyRow label={t("task:treeCost")}>{formatCurrency(summary.cost_subcents)}</PropertyRow>
      <PropertyRow label={t("task:treeTasks")}>{summary.task_count}</PropertyRow>
      <PropertyRow label={t("task:inputTokens")}>{formatNumber(summary.tokens_in)}</PropertyRow>
      <PropertyRow label={t("task:cachedInput")}>
        {formatNumber(summary.tokens_cached_in)}
      </PropertyRow>
      <PropertyRow label={t("task:outputTokens")}>{formatNumber(summary.tokens_out)}</PropertyRow>
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
