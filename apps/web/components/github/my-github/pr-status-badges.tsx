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
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  const { checks_state: state, checks_total: total, checks_passing: passing } = status;
  if (!state) return null;
  const label = total > 0 ? `${passing}/${total}` : "";
  if (state === "success")
    return <StatusChip Icon={IconCheck} label={label || t("github:checksPassed")} tone="success" />;
  if (state === "failure")
    return <StatusChip Icon={IconX} label={label || t("github:checksFailed")} tone="failure" />;
  return (
    <StatusChip Icon={IconClockHour4} label={label || t("github:checksRunning")} tone="pending" />
  );
}

function ReviewChip({
  state,
  pending,
}: {
  state: GitHubPRStatus["review_state"];
  pending: number;
}) {
  const { t } = useTranslation();
  if (state === "approved")
    return <StatusChip Icon={IconCheck} label={t("github:approved")} tone="success" />;
  if (state === "changes_requested")
    return (
      <StatusChip Icon={IconAlertTriangle} label={t("github:changesRequested")} tone="failure" />
    );
  if (pending > 0)
    return (
      <StatusChip
        Icon={IconClockHour4}
        label={t("github:pending", { pending })}
        tone="pending"
        title={t("github:pendingReviewS", { pending })}
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
  const { t } = useTranslation();
  if (prState === "merged")
    return <StatusChip Icon={IconGitMerge} label={t("github:merged")} tone="success" />;
  if (state === "draft")
    return <StatusChip Icon={IconGitPullRequestDraft} label={t("github:draft")} tone="neutral" />;
  if (state === "dirty")
    return <StatusChip Icon={IconAlertTriangle} label={t("github:conflicts")} tone="failure" />;
  if (state === "blocked")
    return <StatusChip Icon={IconAlertTriangle} label={t("github:blocked")} tone="pending" />;
  if (state === "behind")
    return <StatusChip Icon={IconAlertTriangle} label={t("github:behindBase")} tone="pending" />;
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
