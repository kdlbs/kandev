"use client";

import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { PRFeedbackComment } from "@/lib/state/slices/comments";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { usePRCIPopover } from "@/hooks/domains/github/use-pr-ci-popover";
import {
  bucketCheck,
  bucketCheckCounts,
  groupChecksByWorkflow,
  type CheckBucket,
  type WorkflowGroup,
} from "@/lib/github/check-buckets";
import type { CheckRun, TaskPR } from "@/lib/types/github";
import { PRCIAutomationControls } from "./pr-ci-automation-controls";
import { PRDispositionRow } from "./pr-disposition-row";
import { PRMergeButton } from "./pr-merge-button";
import { PRMergeabilityRow } from "./pr-mergeability-row";
import {
  ChangeRequestCIPopoverFrame,
  ChangeRequestChecksSection,
  ChangeRequestCommentsRow,
  ChangeRequestPopoverFooter,
  ChangeRequestPopoverHeader,
  ChangeRequestReviewRow,
  CHANGE_REQUEST_CI_DESKTOP_SCROLL_CLASS,
  type ChangeRequestCheckRow,
} from "@/components/integrations/change-request-ci-anatomy";

type CountsView = {
  passed: number;
  inProgress: number;
  failed: number;
};

const CHECK_GROUP_ORDER: CheckBucket[] = ["passed", "in_progress", "failed"];

function normalizedCheckState(kind: CheckBucket): ChangeRequestCheckRow["state"] {
  if (kind === "passed") return "success";
  if (kind === "failed") return "failure";
  return "pending";
}

function normalizedReviewState(pr: TaskPR): "approved" | "changes_requested" | "pending" {
  if (pr.review_state === "approved") return "approved";
  if (pr.review_state === "changes_requested") return "changes_requested";
  return "pending";
}

export const PR_CI_DESKTOP_POPOVER_SCROLL_CLASS = CHANGE_REQUEST_CI_DESKTOP_SCROLL_CLASS;

export function deriveAggregateCounts(pr: TaskPR): CountsView {
  // Pre-load coarse split from aggregate fields; lazy PRFeedback fetch replaces it.
  const total = Math.max(0, pr.checks_total);
  const passing = Math.min(Math.max(0, pr.checks_passing), total);
  const remaining = Math.max(0, total - passing);
  if (pr.checks_state === "failure") {
    const failed = remaining > 0 ? remaining : 1;
    const passed = total > 0 ? Math.max(0, total - failed) : 0;
    return { passed, failed, inProgress: 0 };
  }
  if (pr.checks_state === "pending") {
    const inProgress = remaining > 0 ? remaining : 1;
    const passed = total > 0 ? Math.max(0, total - inProgress) : 0;
    return { passed, failed: 0, inProgress };
  }
  if (pr.checks_state === "success") {
    return { passed: total, failed: 0, inProgress: 0 };
  }
  return { passed: passing, failed: 0, inProgress: remaining };
}

export function hasNoChecksAtAll(
  pr: TaskPR,
  feedback: { checks?: CheckRun[] } | null,
  isFetching: boolean,
): boolean {
  return (
    !isFetching &&
    pr.checks_state === "" &&
    pr.checks_total === 0 &&
    (!feedback || (feedback.checks?.length ?? 0) === 0)
  );
}

function PRCIPopoverHeader({
  pr,
  onOpenDetailPanel,
}: {
  pr: TaskPR;
  onOpenDetailPanel?: () => void;
}) {
  const { t } = useTranslation();
  const title = pr.pr_title || t("github:untitledPr");
  return (
    <ChangeRequestPopoverHeader
      number={pr.pr_number}
      title={title}
      url={pr.pr_url}
      onOpenReview={onOpenDetailPanel}
      openDetailsLabel={t("github:openDetails", { title: `#${pr.pr_number} ${title}` })}
      externalLabel={t("github:viewPullRequestOnGithub")}
    />
  );
}

function buildWorkflowMessage(group: WorkflowGroup): string {
  const failed = group.jobs.filter((j) => bucketCheck(j) === "failed");
  const lines: string[] = [
    `### Workflow **${group.workflow}** has ${failed.length} failing job${failed.length !== 1 ? "s" : ""}.`,
    "",
  ];
  for (const job of failed) {
    lines.push(`- **${job.name}** - ${job.conclusion || job.status}`);
    if (job.output) lines.push(`  ${job.output}`);
    if (job.html_url) lines.push(`  ${job.html_url}`);
  }
  lines.push("", "Please investigate and fix.");
  return lines.join("\n");
}

function PRChecksSection({
  pr,
  feedback,
  isFetching,
  onAddAsContext,
}: {
  pr: TaskPR;
  feedback: { checks?: CheckRun[] } | null;
  isFetching: boolean;
  onAddAsContext: ((message: string) => void) | null;
}) {
  const { t } = useTranslation();
  const aggregateCounts = useMemo(() => deriveAggregateCounts(pr), [pr]);

  const { precise, byBucket } = useMemo(() => {
    // Treat empty `feedback.checks` the same as "feedback not loaded yet" so
    // we keep showing the aggregate counts. Some mock paths return empty
    // arrays without errors, and we don't want the popover to flash 0/0/0.
    if (!feedback?.checks || feedback.checks.length === 0) {
      return { precise: null as CountsView | null, byBucket: null };
    }
    const counts = bucketCheckCounts(feedback.checks);
    const precise: CountsView = {
      passed: counts.passed,
      inProgress: counts.inProgress,
      failed: counts.failed,
    };
    const groups = groupChecksByWorkflow(feedback.checks);
    const byBucket: Record<CheckBucket, WorkflowGroup[]> = {
      passed: groups.filter((g) => g.bucket === "passed"),
      in_progress: groups.filter((g) => g.bucket === "in_progress"),
      failed: groups.filter((g) => g.bucket === "failed"),
    };
    return { precise, byBucket };
  }, [feedback]);

  const counts = precise ?? aggregateCounts;
  const rows: ChangeRequestCheckRow[] = byBucket
    ? CHECK_GROUP_ORDER.flatMap((kind) =>
        byBucket[kind].map((group) => ({
          id: `${kind}:${group.workflow}`,
          label: group.workflow,
          state: normalizedCheckState(kind),
          detail:
            kind === "in_progress"
              ? `${group.inProgress} running`
              : `${group.passed}/${group.total} passed`,
          ...(group.htmlUrl ? { url: group.htmlUrl } : {}),
          ...(kind === "failed" && onAddAsContext
            ? { onAddAsContext: () => onAddAsContext(buildWorkflowMessage(group)) }
            : {}),
        })),
      )
    : [];
  return (
    <ChangeRequestChecksSection
      counts={{ passed: counts.passed, pending: counts.inProgress, failed: counts.failed }}
      rows={rows}
      loading={isFetching && !byBucket}
      emptyLabel={t("github:noChecksHaveStarted")}
      passRateLabel={t("github:passRate")}
      groupLabels={{
        success: t("github:checkBucketPassed"),
        pending: t("github:checkBucketInProgress"),
        failure: t("github:checkBucketFailed"),
      }}
    />
  );
}

function PRReviewRow({ pr }: { pr: TaskPR }) {
  const required = pr.required_reviews ?? null;
  const approved = pr.review_count;
  const requested = pr.pending_review_count;

  return (
    <ChangeRequestReviewRow
      state={normalizedReviewState(pr)}
      approved={approved}
      required={required ?? undefined}
      requested={requested}
    />
  );
}

function PRCommentsRow({ pr }: { pr: TaskPR }) {
  const { t } = useTranslation();
  const count = pr.unresolved_review_threads ?? 0;
  return (
    <ChangeRequestCommentsRow count={count} label={t("github:unresolvedComments", { count })} />
  );
}

function PRPopoverFooter({
  lastUpdatedAt,
  isRefreshing,
}: {
  lastUpdatedAt: number | null;
  isRefreshing: boolean;
}) {
  const { t } = useTranslation();
  return (
    <ChangeRequestPopoverFooter
      updatedAt={lastUpdatedAt ?? undefined}
      isRefreshing={isRefreshing}
      updatingLabel={t("github:updatingStatus")}
      formatElapsed={(seconds) =>
        seconds === 0
          ? t("github:updatedJustNow")
          : t("github:updatedAgo", { elapsed: formatElapsedShort(seconds) })
      }
    />
  );
}

function formatElapsedShort(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  return `${Math.floor(minutes / 60)}h`;
}

function ReconnectGitHubBlock() {
  const { t } = useTranslation();
  return (
    <div
      data-testid="pr-popover-auth-error"
      className="flex flex-col items-start gap-1 px-1 py-2 text-xs"
    >
      <span className="text-foreground">{t("github:githubAuthenticationLost")}</span>
      <a
        data-testid="pr-popover-reconnect-link"
        href="/settings#github"
        className="cursor-pointer text-primary hover:underline"
      >
        {t("github:reconnectGithub")}
      </a>
    </div>
  );
}

export function PRCIPopover({
  pr,
  enabled,
  onOpenDetailPanel,
  refreshTaskPR,
}: {
  pr: TaskPR;
  enabled: boolean;
  onOpenDetailPanel?: () => void;
  refreshTaskPR?: () => void | Promise<void>;
}) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { status: ghStatus } = useGitHubStatus(workspaceId);
  const authLost = ghStatus !== null && !ghStatus.authenticated;
  const { feedback, isFetching, isRefreshing, lastUpdatedAt, refetch } = usePRCIPopover(
    workspaceId,
    pr,
    enabled && !authLost,
    refreshTaskPR,
  );
  const onAddAsContext = useAddCheckToContext(pr);

  return (
    <ChangeRequestCIPopoverFrame>
      <PRCIPopoverHeader pr={pr} onOpenDetailPanel={onOpenDetailPanel} />
      {authLost ? (
        <ReconnectGitHubBlock />
      ) : (
        <>
          <PRChecksSection
            pr={pr}
            feedback={feedback}
            isFetching={isFetching}
            onAddAsContext={onAddAsContext}
          />
          <div className="flex flex-col gap-0">
            <PRReviewRow pr={pr} />
            <PRCommentsRow pr={pr} />
          </div>
          <PRMergeabilityRow pr={pr} />
          <PRDispositionRow pr={pr} />
          <PRCIAutomationControls pr={pr} />
          <PRMergeButton taskPR={pr} onMerged={refetch} compact />
        </>
      )}
      <PRPopoverFooter lastUpdatedAt={lastUpdatedAt} isRefreshing={isRefreshing} />
    </ChangeRequestCIPopoverFrame>
  );
}

// --- Add-to-context wiring (mirrors pr-detail-panel.tsx for failed checks) ---
function useAddCheckToContext(pr: TaskPR): ((message: string) => void) | null {
  const { t } = useTranslation();
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const addComment = useCommentsStore((s) => s.addComment);
  const { toast } = useToast();
  const prNumber = pr.pr_number;
  // Always call useCallback (rules-of-hooks) before bailing out, so the
  // returned callback identity is stable across renders unless its inputs
  // change. Without memoization the popover would create a new function
  // every parent render, defeating PRWorkflowRow's cheap reference equality.
  const handler = useCallback(
    (message: string) => {
      if (!sessionId) return;
      const comment: PRFeedbackComment = {
        id: `pr-feedback-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        sessionId,
        text: message,
        createdAt: new Date().toISOString(),
        status: "pending",
        source: "pr-feedback",
        prNumber,
        feedbackType: "check",
        content: message,
      };
      addComment(comment);
      toast({ description: t("github:addedToChatContext") });
    },
    [sessionId, prNumber, addComment, toast, t],
  );
  return sessionId ? handler : null;
}
