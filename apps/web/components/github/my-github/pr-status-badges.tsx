"use client";

import {
  IconCheck,
  IconX,
  IconClockHour4,
  IconGitMerge,
  IconAlertTriangle,
  IconGitPullRequestDraft,
} from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import type { GitHubPR, GitHubPRStatus } from "@/lib/types/github";
import { cn } from "@/lib/utils";

type StatusChipProps = {
  Icon: Icon;
  label: string;
  tone: "success" | "failure" | "pending" | "neutral";
  title?: string;
};

function StatusChip({ Icon, label, tone, title }: StatusChipProps) {
  const toneClass = {
    success: "text-emerald-600 dark:text-emerald-400",
    failure: "text-red-600 dark:text-red-400",
    pending: "text-amber-600 dark:text-amber-400",
    neutral: "text-muted-foreground",
  }[tone];
  return (
    <span
      className={cn("inline-flex items-center gap-1 text-xs", toneClass)}
      title={title ?? label}
    >
      <Icon className="h-3.5 w-3.5 shrink-0" />
      <span className="whitespace-nowrap tabular-nums">{label}</span>
    </span>
  );
}

function ChecksChip({ status }: { status: GitHubPRStatus }) {
  const { checks_state: state, checks_total: total, checks_passing: passing } = status;
  if (!state) return null;
  const label = total > 0 ? `${passing}/${total}` : "";
  if (state === "success")
    return <StatusChip Icon={IconCheck} label={label || "Checks passed"} tone="success" />;
  if (state === "failure")
    return <StatusChip Icon={IconX} label={label || "Checks failed"} tone="failure" />;
  return <StatusChip Icon={IconClockHour4} label={label || "Checks running"} tone="pending" />;
}

function ReviewChip({
  state,
  pending,
}: {
  state: GitHubPRStatus["review_state"];
  pending: number;
}) {
  if (state === "approved") return <StatusChip Icon={IconCheck} label="Approved" tone="success" />;
  if (state === "changes_requested")
    return <StatusChip Icon={IconAlertTriangle} label="Changes requested" tone="failure" />;
  if (pending > 0)
    return (
      <StatusChip
        Icon={IconClockHour4}
        label={`${pending} pending`}
        tone="pending"
        title={`${pending} pending review(s)`}
      />
    );
  return null;
}

function MergeableChip({
  state,
  prState,
}: {
  state: GitHubPRStatus["mergeable_state"];
  prState: GitHubPR["state"];
}) {
  if (prState === "merged") return <StatusChip Icon={IconGitMerge} label="Merged" tone="success" />;
  if (state === "draft")
    return <StatusChip Icon={IconGitPullRequestDraft} label="Draft" tone="neutral" />;
  if (state === "dirty" || state === "blocked")
    return <StatusChip Icon={IconAlertTriangle} label="Conflicts" tone="failure" />;
  if (state === "behind")
    return <StatusChip Icon={IconAlertTriangle} label="Behind base" tone="pending" />;
  return null;
}

export function PRStatusBadges({
  pr,
  status,
}: {
  pr: GitHubPR;
  status: GitHubPRStatus | null | undefined;
}) {
  if (!status) return null;
  return (
    <>
      <ChecksChip status={status} />
      <ReviewChip state={status.review_state} pending={status.pending_review_count} />
      <MergeableChip state={status.mergeable_state} prState={pr.state} />
    </>
  );
}
