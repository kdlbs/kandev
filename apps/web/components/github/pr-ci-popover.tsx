"use client";

import { useCallback, useMemo } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { useTaskSessionId, useTaskWorkspaceId } from "@/components/task/task-workspace-context";
import { formatCompactDuration } from "@/lib/i18n/formats";
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
import { PRMergeButton } from "./pr-merge-button";
import { PRMergeabilityRow } from "./pr-mergeability-row";
import { PRWorkflowAttentionNotice } from "./pr-workflow-attention-notice";
import { isAllSkippedCheckSummary } from "./pr-ci-state";
import {
  ChangeRequestCIPopoverFrame,
  ChangeRequestChecksSection,
  ChangeRequestCommentsRow,
  ChangeRequestPopoverFooter,
  ChangeRequestPopoverHeader,
  ChangeRequestReviewRow,
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

export const PR_CI_DESKTOP_POPOVER_CLASS =
  "h-[min(28rem,calc(100dvh-2rem))] max-h-[var(--radix-popover-content-available-height)] max-w-[calc(100vw-1rem)] overflow-hidden motion-reduce:animate-none";

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
      author={pr.author_login}
      url={pr.pr_url}
      onOpenReview={onOpenDetailPanel}
      openDetailsLabel={t("github:openDetails", { title: `#${pr.pr_number} ${title}` })}
      externalLabel={t("github:openPr")}
      externalActionLabel={t("github:openPr")}
      className="h-14 shrink-0 [@media(pointer:coarse)]:h-16 [@media(max-width:767px)]:h-16"
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

function emptyChecksLabel({
  loadedAllSkipped,
  persistedAllSkipped,
  checks,
  t,
}: {
  loadedAllSkipped: boolean;
  persistedAllSkipped: boolean;
  checks: CheckRun[] | undefined;
  t: TFunction;
}): string {
  if (loadedAllSkipped || persistedAllSkipped) return t("github:allChecksSkipped");
  if (checks?.some((check) => check.conclusion === "cancelled")) {
    return t("github:checksNotSuccessful");
  }
  return t("github:noChecksHaveStarted");
}

type PreciseCheckView = {
  precise: CountsView | null;
  byBucket: Record<CheckBucket, WorkflowGroup[]> | null;
};

function preciseCheckView(feedback: { checks?: CheckRun[] } | null): PreciseCheckView {
  // A loaded empty list is authoritative. The selected workflow may be active
  // before its dependent jobs exist, so aggregate counts would invent checks.
  if (!feedback?.checks) return { precise: null, byBucket: null };
  const counts = bucketCheckCounts(feedback.checks);
  const groups = groupChecksByWorkflow(feedback.checks);
  return {
    precise: { passed: counts.passed, inProgress: counts.inProgress, failed: counts.failed },
    byBucket: {
      passed: groups.filter((g) => g.bucket === "passed"),
      in_progress: groups.filter((g) => g.bucket === "in_progress"),
      failed: groups.filter((g) => g.bucket === "failed"),
    },
  };
}

function checkRows(
  byBucket: Record<CheckBucket, WorkflowGroup[]> | null,
  onAddAsContext: ((message: string) => void) | null,
): ChangeRequestCheckRow[] {
  if (!byBucket) return [];
  return CHECK_GROUP_ORDER.flatMap((kind) =>
    byBucket[kind].map((group) => ({
      id: `${kind}:${group.id}`,
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
  );
}

function skippedCheckView(pr: TaskPR, feedback: { checks?: CheckRun[] } | null) {
  const loadedSkipped = feedback?.checks
    ? feedback.checks.filter(
        (check) => check.status === "completed" && check.conclusion === "skipped",
      ).length
    : 0;
  return {
    skippedCount: feedback?.checks ? loadedSkipped : 0,
    loadedAllSkipped:
      feedback?.checks != null &&
      feedback.checks.length > 0 &&
      loadedSkipped === feedback.checks.length,
    persistedAllSkipped: feedback?.checks == null && isAllSkippedCheckSummary(pr),
  };
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
  const { precise, byBucket } = useMemo(() => preciseCheckView(feedback), [feedback]);
  const counts = precise ?? aggregateCounts;
  const rows = checkRows(byBucket, onAddAsContext);
  const { skippedCount, loadedAllSkipped, persistedAllSkipped } = skippedCheckView(pr, feedback);
  if (isFetching && !feedback) {
    return (
      <div data-testid="pr-checks-loading" aria-hidden="true" className="space-y-3 p-1">
        {["w-3/4", "w-full", "w-2/3"].map((width) => (
          <div key={width} className={`h-4 rounded bg-muted motion-safe:animate-pulse ${width}`} />
        ))}
      </div>
    );
  }
  return (
    <ChangeRequestChecksSection
      counts={{ passed: counts.passed, pending: counts.inProgress, failed: counts.failed }}
      rows={rows}
      loading={isFetching && !byBucket}
      emptyLabel={emptyChecksLabel({
        loadedAllSkipped,
        persistedAllSkipped,
        checks: feedback?.checks,
        t,
      })}
      skippedCount={skippedCount}
      skippedLabel={
        skippedCount > 0 ? t("github:checksSkippedCount", { count: skippedCount }) : undefined
      }
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
  if (seconds < 60) return formatCompactDuration(seconds, "second");
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return formatCompactDuration(minutes, "minute");
  return formatCompactDuration(Math.floor(minutes / 60), "hour");
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
  const { t } = useTranslation();
  const workspaceId = useTaskWorkspaceId(pr.task_id);
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
    <ChangeRequestCIPopoverFrame className="min-h-0 flex-1">
      <PRCIPopoverHeader pr={pr} onOpenDetailPanel={onOpenDetailPanel} />
      <div
        key={pr.id}
        data-testid="pr-popover-scroll-body"
        role="region"
        aria-label={t("github:pullRequestCiStatusReviewsAnd")}
        tabIndex={0}
        className="-mx-2 min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 [scrollbar-gutter:stable] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
      >
        <div className="flex flex-col gap-2">
          <PRWorkflowAttentionNotice pr={pr} attention={feedback?.workflow_attention} />
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
              <PRCIAutomationControls pr={pr} />
              <PRMergeButton taskPR={pr} onMerged={refetch} compact />
            </>
          )}
        </div>
      </div>
      <div className="h-6 shrink-0 overflow-hidden">
        <PRPopoverFooter lastUpdatedAt={lastUpdatedAt} isRefreshing={isRefreshing} />
      </div>
    </ChangeRequestCIPopoverFrame>
  );
}

// --- Add-to-context wiring (mirrors pr-detail-panel.tsx for failed checks) ---
function useAddCheckToContext(pr: TaskPR): ((message: string) => void) | null {
  const { t } = useTranslation();
  const sessionId = useTaskSessionId(pr.task_id);
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
