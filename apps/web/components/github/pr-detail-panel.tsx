"use client";

import { useCallback, useEffect, useState } from "react";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import {
  IconRefresh,
  IconPlus,
  IconMinus,
  IconGitMerge,
  IconCheck,
  IconLoader2,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useActiveTaskPR, useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { prPanelLabel, prTaskKey } from "@/components/github/pr-utils";
import { usePRFeedback } from "@/hooks/domains/github/use-pr-feedback";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { getGitHubMutationActor } from "@/lib/github-auth";
import { useCommentsStore, isPRFeedbackComment } from "@/lib/state/slices/comments";
import type { PRFeedbackComment } from "@/lib/state/slices/comments";
import { useToast } from "@/components/toast-provider";
import { submitPRReview } from "@/lib/api/domains/github-pr-api";
import type { TaskPR, PRFeedback } from "@/lib/types/github";
import {
  formatTimeAgo,
  AuthorLink,
  getTimeAgoColor,
  CollapsibleSection,
  PRMarkdownBody,
} from "./pr-shared";
import { PRMergeButton } from "./pr-merge-button";
import { PRMergeabilityNotice, buildConflictResolutionMessage } from "./pr-mergeability-notice";
import { ReviewStateBadge } from "./pr-reviews-section";
import { ChecksSection } from "./pr-checks-section";
import { ReviewsSection } from "./pr-reviews-section";
import { CommentsSection } from "./pr-comments-section";
import { usePRScopedReviewRequest } from "./use-pr-scoped-review-request";
import { Trans, useTranslation } from "react-i18next";

// --- Dockview panel wrapper ---

type PRDetailPanelProps = {
  panelId: string;
  /** Per-PR params; multi-repo panels carry prKey="<owner>/<repo>/<pr_number>". */
  params?: { prKey?: string };
};

export function PRDetailPanelComponent({ panelId, params }: PRDetailPanelProps) {
  const { t } = useTranslation();
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const { prs } = useTaskPR(activeTaskId);
  const activePR = useActiveTaskPR();
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);

  // Multi-repo: when the panel was opened with a prKey, render the matching
  // TaskPR. Falls back to the active (primary) PR for legacy single-repo
  // panels that pre-date the prKey param.
  const pr = (params?.prKey ? prs.find((p) => prTaskKey(p) === params.prKey) : null) ?? activePR;

  useEffect(() => {
    const title = pr ? prPanelLabel(pr.pr_number) : "Pull Request";
    setPanelTitle(panelId, title);
  }, [pr, panelId]);

  if (!pr || !sessionId) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        {t("github:noPullRequestLinkedToThis")}
      </div>
    );
  }

  return (
    <div data-testid="pr-detail-panel" className="h-full">
      <PRDetailContent taskPR={pr} sessionId={sessionId} />
    </div>
  );
}

// --- Add PR feedback as chat context ---

function useAddPRFeedbackAsContext(sessionId: string, prNumber: number) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const addComment = useCommentsStore((s) => s.addComment);

  const addAsContext = useCallback(
    (feedbackType: PRFeedbackComment["feedbackType"], content: string) => {
      const comment: PRFeedbackComment = {
        id: `pr-feedback-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        sessionId,
        text: content,
        createdAt: new Date().toISOString(),
        status: "pending",
        source: "pr-feedback",
        prNumber,
        feedbackType,
        content,
      };
      addComment(comment);
      toast({ description: t("github:addedToChatContext") });
    },
    [sessionId, prNumber, addComment, toast],
  );

  return { addAsContext };
}

// The panel used to patch state / merged_at / closed_at / additions / deletions /
// mergeable_state from the feedback response straight into the Zustand taskPRs
// row (useSyncLivePRState). That write never reached github_task_prs, so opening
// this panel turned its own tab purple while the kanban card and boot payload
// kept reading state=open, and a reload snapped it back. The backend now writes
// the same live PR through SyncTaskPR during the feedback fetch and publishes
// github.task_pr.updated, which lib/ws/handlers/github.ts applies to the store —
// one writer, and it survives a reload. Do not reintroduce a client-side patch.

type PRPanelMetrics = {
  reviewCount: number;
  pendingReviewCount: number;
  commentCount: number;
  reviewState: TaskPR["review_state"];
};

function computeLiveReviewState(feedback: PRFeedback, fallbackState: TaskPR["review_state"]) {
  const requestedReviewers = feedback.pr.requested_reviewers ?? [];
  const reviews = feedback.reviews ?? [];
  if (reviews.length === 0) {
    return requestedReviewers.length > 0 ? "pending" : fallbackState || "";
  }
  const latestByAuthor = new Map<string, { state: string; createdAt: number }>();
  for (const review of reviews) {
    const current = latestByAuthor.get(review.author);
    const createdAt = new Date(review.created_at).getTime();
    if (!current || createdAt > current.createdAt) {
      latestByAuthor.set(review.author, { state: review.state, createdAt });
    }
  }
  let hasChangesRequested = false;
  let allApproved = true;
  for (const review of latestByAuthor.values()) {
    if (review.state === "CHANGES_REQUESTED") hasChangesRequested = true;
    if (review.state !== "APPROVED") allApproved = false;
  }
  if (hasChangesRequested) return "changes_requested";
  if (allApproved) return "approved";
  return "pending";
}

function derivePanelMetrics(taskPR: TaskPR, feedback: PRFeedback | null): PRPanelMetrics {
  if (!feedback) {
    return {
      reviewCount: taskPR.review_count,
      pendingReviewCount: taskPR.pending_review_count,
      commentCount: taskPR.comment_count,
      reviewState: taskPR.review_state,
    };
  }
  const pendingReviewCount = feedback.pr.requested_reviewers?.length ?? taskPR.pending_review_count;
  return {
    reviewCount: (feedback.reviews ?? []).length,
    pendingReviewCount,
    commentCount: (feedback.comments ?? []).length,
    reviewState: computeLiveReviewState(feedback, taskPR.review_state),
  };
}

function DescriptionSection({ body }: { body: string }) {
  const { t } = useTranslation();
  if (!body) return null;
  return (
    <CollapsibleSection title={t("github:description")} count={1} defaultOpen={false}>
      <div className="px-2">
        <PRMarkdownBody body={body} />
      </div>
    </CollapsibleSection>
  );
}

// GitHub logins are case-insensitive; normalize before comparing.
// Fails closed when the current user is unknown — without that identity we
// can't tell whether the viewer is the PR author, and GitHub rejects
// self-approval, so the button would only ever produce a failed request.
// Exported for unit testing.
export function shouldHideApproveButton(
  taskPR: TaskPR,
  feedback: PRFeedback | null,
  currentUser: string | null,
  hasMutationActor = !!currentUser?.trim(),
): boolean {
  const liveState = feedback?.pr.state ?? taskPR.state;
  if (liveState !== "open") return true;
  const normalizedUser = currentUser?.trim().toLowerCase();
  if (!normalizedUser) return !hasMutationActor;
  const prAuthor = feedback?.pr.author_login ?? taskPR.author_login;
  if (prAuthor?.trim().toLowerCase() === normalizedUser) return true;
  return (
    feedback?.reviews?.some(
      (r) => r.state === "APPROVED" && r.author?.trim().toLowerCase() === normalizedUser,
    ) ?? false
  );
}

export function shouldShowReRequestReviewAction(prState: string, reviewState: string): boolean {
  return prState === "open" && reviewState === "DISMISSED";
}

function ApproveButton({
  workspaceId,
  taskPR,
  feedback,
  onRefresh,
}: {
  workspaceId: string | null;
  taskPR: TaskPR;
  feedback: PRFeedback | null;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [submitting, setSubmitting] = useState(false);
  // Ensures status (and thus the authenticated username) is fetched even when
  // the PR panel is the first GitHub-aware surface the user opens; without
  // this, currentUser is null on first render and shouldHideApproveButton has
  // no identity to compare against the PR author.
  const { status } = useGitHubStatus();
  const currentUser = status?.username ?? null;
  const mutationActor = getGitHubMutationActor(status);

  if (shouldHideApproveButton(taskPR, feedback, currentUser, !!mutationActor)) return null;

  const handleApprove = async () => {
    if (!workspaceId) return;
    setSubmitting(true);
    try {
      await submitPRReview(
        workspaceId,
        { owner: taskPR.owner, repo: taskPR.repo, number: taskPR.pr_number },
        "APPROVE",
      );
      toast({ description: t("github:prApproved"), variant: "success" });
      onRefresh();
    } catch (e) {
      toast({
        title: t("github:failedToApprove"),
        description: e instanceof Error ? e.message : t("github:anErrorOccurred"),
        variant: "error",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Button
      data-testid="pr-approve-button"
      size="sm"
      className="cursor-pointer gap-1.5 border-0 bg-green-600 text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500"
      onClick={handleApprove}
      disabled={submitting}
    >
      <IconCheck className="h-3.5 w-3.5" />
      {submitting ? t("github:approving") : t("github:approveAs", { mutationActor })}
    </Button>
  );
}

/**
 * True once a conflict prompt for this PR is already queued — avoids piling up
 * identical instructions if the user clicks "Resolve conflicts" again. Extracted
 * from PRDetailContent, which is at the 100-line function cap.
 */
function useConflictQueued(sessionId: string, prNumber: number): boolean {
  return useCommentsStore((s) =>
    s.pendingForChat.some((id) => {
      const c = s.byId[id];
      return (
        !!c &&
        isPRFeedbackComment(c) &&
        c.feedbackType === "conflict" &&
        c.sessionId === sessionId &&
        c.prNumber === prNumber
      );
    }),
  );
}

export function PRDetailContent({ taskPR, sessionId }: { taskPR: TaskPR; sessionId: string }) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const { feedback, loading, refresh } = usePRFeedback(
    workspaceId,
    taskPR.owner,
    taskPR.repo,
    taskPR.pr_number,
  );
  const { addAsContext } = useAddPRFeedbackAsContext(sessionId, taskPR.pr_number);
  const { toast } = useToast();
  const reviewRequest = usePRScopedReviewRequest(taskPR, {
    workspaceId,
    requestedReviewers: feedback?.pr.requested_reviewers ?? [],
    reviews: feedback?.reviews ?? [],
    refresh,
    toast,
  });

  const metrics = derivePanelMetrics(taskPR, feedback);

  const conflictQueued = useConflictQueued(sessionId, taskPR.pr_number);

  const onResolveConflicts = useCallback(() => {
    if (conflictQueued) return;
    addAsContext(
      "conflict",
      buildConflictResolutionMessage({
        prNumber: taskPR.pr_number,
        headBranch: taskPR.head_branch,
        baseBranch: taskPR.base_branch,
      }),
    );
  }, [addAsContext, conflictQueued, taskPR.pr_number, taskPR.head_branch, taskPR.base_branch]);

  return (
    <div className="flex flex-col h-full">
      <PRHeader
        workspaceId={workspaceId}
        taskPR={taskPR}
        feedback={feedback}
        metrics={metrics}
        loading={loading}
        onRefresh={refresh}
        onResolveConflicts={onResolveConflicts}
        conflictQueued={conflictQueued}
      />
      <Separator />
      <ScrollArea className="flex-1 overflow-hidden">
        <div className="box-border w-0 min-w-full max-w-full overflow-x-hidden p-3 space-y-1">
          {loading && !feedback && (
            <div className="flex items-center justify-center py-8">
              <IconLoader2 className="h-6 w-6 text-blue-500 animate-spin" />
            </div>
          )}
          {feedback && (
            <>
              <DescriptionSection body={feedback.pr.body ?? ""} />
              <ReviewsSection
                reviews={feedback.reviews ?? []}
                requestedReviewers={reviewRequest.requestedReviewers}
                prUrl={taskPR.pr_url}
                reviewState={metrics.reviewState}
                pendingReviewCount={metrics.pendingReviewCount}
                onAddAsContext={(msg) => addAsContext("review", msg)}
                canReRequest={shouldShowReRequestReviewAction(feedback.pr.state, "DISMISSED")}
                requestingReviewers={reviewRequest.requestingReviewers}
                onReRequest={reviewRequest.reRequest}
              />
              <ChecksSection
                checks={feedback.checks ?? []}
                onAddAsContext={(msg) => addAsContext("check", msg)}
              />
              <CommentsSection
                comments={feedback.comments ?? []}
                prUrl={taskPR.pr_url}
                onAddAsContext={(msg) => addAsContext("comment", msg)}
              />
            </>
          )}
        </div>
      </ScrollArea>
      {taskPR.last_synced_at && (
        <>
          <Separator />
          <div className="px-3 py-2 text-[10px] text-muted-foreground text-center">
            {t("github:lastSynced")} {formatTimeAgo(taskPR.last_synced_at)}
          </div>
        </>
      )}
    </div>
  );
}

function StateBadge({ state }: { state: string }) {
  const styles: Record<string, string> = {
    open: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    draft: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
    merged: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
    closed: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  };
  return (
    <Badge variant="secondary" className={`text-[10px] px-1.5 py-0 ${styles[state] ?? ""}`}>
      {state}
    </Badge>
  );
}

function HeaderTitleRow({
  taskPR,
  loading,
  onRefresh,
}: {
  taskPR: TaskPR;
  loading: boolean;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-start justify-between gap-2">
      <a
        href={taskPR.pr_url}
        target="_blank"
        rel="noopener noreferrer"
        className="text-sm font-medium hover:underline truncate cursor-pointer min-w-0 flex-1"
      >
        {taskPR.pr_title}
      </a>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 w-6 p-0 cursor-pointer shrink-0 text-muted-foreground hover:text-foreground"
            onClick={onRefresh}
            disabled={loading}
          >
            <IconRefresh className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("github:refresh")}</TooltipContent>
      </Tooltip>
    </div>
  );
}

function HeaderDateLine({ taskPR }: { taskPR: TaskPR }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1.5 text-xs text-muted-foreground flex-wrap">
      <span className="flex items-center gap-0.5">
        <Trans i18nKey="github:byAuthor">
          by <AuthorLink author={taskPR.author_login} />
        </Trans>
      </span>
      <span>&middot;</span>
      <span className={getTimeAgoColor(taskPR.created_at)}>
        {t("github:openedAgo", { time: formatTimeAgo(taskPR.created_at) })}
      </span>
      {taskPR.merged_at && (
        <>
          <span>&middot;</span>
          <span className="flex items-center gap-0.5">
            <IconGitMerge className="h-3 w-3 text-purple-500" />
            {t("github:mergedAgo", { time: formatTimeAgo(taskPR.merged_at) })}
          </span>
        </>
      )}
      {taskPR.closed_at && !taskPR.merged_at && (
        <>
          <span>&middot;</span>
          <span>{t("github:closedAgo", { time: formatTimeAgo(taskPR.closed_at) })}</span>
        </>
      )}
    </div>
  );
}

function HeaderStatsLine({ taskPR, metrics }: { taskPR: TaskPR; metrics: PRPanelMetrics }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-3 text-xs text-muted-foreground flex-wrap">
      <span className="flex items-center gap-1">
        <IconPlus className="h-3 w-3 text-green-500" />
        {taskPR.additions}
      </span>
      <span className="flex items-center gap-1">
        <IconMinus className="h-3 w-3 text-red-500" />
        {taskPR.deletions}
      </span>
      <span>&middot;</span>
      <span>
        {t("github:reviewCount", { count: metrics.reviewCount })}
        {metrics.pendingReviewCount > 0 && (
          <span className="text-yellow-600 dark:text-yellow-400">
            {" "}
            {t("github:pendingReviewCount", { count: metrics.pendingReviewCount })}
          </span>
        )}
      </span>
      <span>&middot;</span>
      <span>{t("github:commentCount", { count: metrics.commentCount })}</span>
      {metrics.reviewState && <ReviewStateBadge state={metrics.reviewState} />}
    </div>
  );
}

function PRHeader({
  workspaceId,
  taskPR,
  feedback,
  metrics,
  loading,
  onRefresh,
  onResolveConflicts,
  conflictQueued,
}: {
  workspaceId: string | null;
  taskPR: TaskPR;
  feedback: PRFeedback | null;
  metrics: PRPanelMetrics;
  loading: boolean;
  onRefresh: () => void;
  onResolveConflicts: () => void;
  conflictQueued: boolean;
}) {
  const liveState = feedback?.pr.state ?? taskPR.state;
  const isDraft = feedback?.pr.draft ?? false;
  const isMergeable = feedback?.pr.mergeable ?? true;
  // Prefer the live feedback state (refreshed by the panel's Refresh button);
  // fall back to the polled store value before feedback loads.
  const mergeableState = feedback?.pr.mergeable_state ?? taskPR.mergeable_state;

  return (
    <div className="p-3 space-y-2">
      <div className="flex items-center gap-2">
        <div className="flex-1 min-w-0">
          <HeaderTitleRow taskPR={taskPR} loading={loading} onRefresh={onRefresh} />
        </div>
        <ApproveButton
          workspaceId={workspaceId}
          taskPR={taskPR}
          feedback={feedback}
          onRefresh={onRefresh}
        />
        <PRMergeButton taskPR={taskPR} onMerged={onRefresh} />
      </div>
      <div className="flex items-center gap-1.5 flex-wrap">
        <StateBadge state={isDraft && liveState === "open" ? "draft" : liveState} />
        <span className="text-xs text-muted-foreground">#{taskPR.pr_number}</span>
        <code className="text-[10px] px-1 py-0.5 bg-muted rounded font-mono">
          {taskPR.head_branch}
        </code>
        <span className="text-muted-foreground mx-0.5">&rarr;</span>
        <code className="text-[10px] px-1 py-0.5 bg-muted rounded font-mono">
          {taskPR.base_branch}
        </code>
      </div>
      <PRMergeabilityNotice
        state={mergeableState}
        mergeable={isMergeable}
        isDraft={isDraft}
        prState={liveState}
        baseBranch={taskPR.base_branch}
        onResolveConflicts={onResolveConflicts}
        resolveDisabled={conflictQueued}
      />
      <HeaderDateLine taskPR={taskPR} />
      <HeaderStatsLine taskPR={taskPR} metrics={metrics} />
    </div>
  );
}
