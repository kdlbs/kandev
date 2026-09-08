"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import { IconCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import {
  ChangeRequestDetail,
  type ChangeRequestDetailModel,
} from "@/components/integrations/change-request-detail";
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
import { PRMergeButton } from "./pr-merge-button";
import { PRMergeabilityNotice, buildConflictResolutionMessage } from "./pr-mergeability-notice";
import { hasActiveMergeQueueEntry, PRMergeQueueStatus } from "./pr-merge-queue-status";
import { usePRScopedReviewRequest } from "./use-pr-scoped-review-request";

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
    const title = pr ? prPanelLabel(pr.pr_number) : t("task:pullRequest2");
    setPanelTitle(panelId, title);
  }, [pr, panelId, t]);

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
    [sessionId, prNumber, addComment, toast, t],
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

function githubPerson(login: string, avatarUrl?: string, isBot?: boolean) {
  return {
    name: login,
    url: login ? `https://github.com/${login}` : undefined,
    avatarUrl: avatarUrl || undefined,
    isBot,
  };
}

function mapGitHubIdentity(taskPR: TaskPR, feedback: PRFeedback | null) {
  const live = feedback?.pr;
  return {
    title: live?.title ?? taskPR.pr_title,
    url: live?.html_url || taskPR.pr_url,
    state: live?.state ?? taskPR.state,
    draft: live?.draft,
    author: githubPerson(live?.author_login ?? taskPR.author_login),
    description: live?.body,
  };
}

function mapGitHubDates(taskPR: TaskPR, feedback: PRFeedback | null) {
  const live = feedback?.pr;
  return {
    createdAt: live?.created_at ?? taskPR.created_at,
    mergedAt: live?.merged_at ?? taskPR.merged_at ?? undefined,
    closedAt: live?.closed_at ?? taskPR.closed_at ?? undefined,
  };
}

function mapGitHubBranches(taskPR: TaskPR, feedback: PRFeedback | null) {
  const live = feedback?.pr;
  return {
    sourceBranch: live?.head_branch ?? taskPR.head_branch,
    targetBranch: live?.base_branch ?? taskPR.base_branch,
    additions: live?.additions ?? taskPR.additions,
    deletions: live?.deletions ?? taskPR.deletions,
  };
}

function mapGitHubReviews(
  taskPR: TaskPR,
  feedback: PRFeedback | null,
  requestingReviewers: string[],
  labels: { reRequestReview: string; requesting: string },
) {
  const requesting = new Set(requestingReviewers.map((reviewer) => reviewer.toLowerCase()));
  const state = feedback?.pr.state ?? taskPR.state;
  return (feedback?.reviews ?? []).map((review) => ({
    id: String(review.id),
    author: githubPerson(review.author, review.author_avatar),
    state: review.state,
    body: review.body || undefined,
    createdAt: review.created_at,
    actions: shouldShowReRequestReviewAction(state, review.state)
      ? [
          {
            id: "rerequest-review",
            label: labels.reRequestReview,
            pendingLabel: labels.requesting,
            tone: "secondary" as const,
            disabled: requesting.has(review.author.toLowerCase()),
            busy: requesting.has(review.author.toLowerCase()),
          },
        ]
      : undefined,
  }));
}

function mapGitHubChecks(feedback: PRFeedback | null) {
  return (feedback?.checks ?? []).map((check) => ({
    id: `${check.name}-${check.html_url}-${check.source}-${check.started_at ?? ""}`,
    name: check.name,
    state: check.status,
    conclusion: check.conclusion,
    url: check.html_url || undefined,
    output: check.output,
    startedAt: check.started_at ?? undefined,
    completedAt: check.completed_at ?? undefined,
  }));
}

export function mapGitHubComments(feedback: PRFeedback | null) {
  return (feedback?.comments ?? []).map((comment) => {
    const url = comment.html_url?.trim();
    return {
      id: String(comment.id),
      parentId: comment.in_reply_to ? String(comment.in_reply_to) : undefined,
      author: githubPerson(comment.author, comment.author_avatar, comment.author_is_bot),
      body: comment.body,
      createdAt: comment.created_at,
      ...(url ? { url } : {}),
      path: comment.path || undefined,
      line: comment.line || undefined,
    };
  });
}

function mapGitHubDetail(
  taskPR: TaskPR,
  feedback: PRFeedback | null,
  metrics: PRPanelMetrics,
  review: {
    requestedReviewers: { login: string; type: "user" | "team" }[];
    requestingReviewers: string[];
    labels: { reRequestReview: string; requesting: string };
  },
): ChangeRequestDetailModel {
  return {
    providerId: "github",
    reviewKey: `${taskPR.owner}/${taskPR.repo}#${taskPR.pr_number}`,
    number: taskPR.pr_number,
    ...mapGitHubIdentity(taskPR, feedback),
    ...mapGitHubDates(taskPR, feedback),
    ...mapGitHubBranches(taskPR, feedback),
    reviewState: metrics.reviewState,
    pendingReviewCount: metrics.pendingReviewCount,
    reviews: mapGitHubReviews(taskPR, feedback, review.requestingReviewers, review.labels),
    requestedReviewers: review.requestedReviewers.map((reviewer) => ({
      ...githubPerson(reviewer.login),
      kind: reviewer.type,
    })),
    checks: mapGitHubChecks(feedback),
    comments: mapGitHubComments(feedback),
    lastSyncedAt: taskPR.last_synced_at ?? undefined,
  };
}

function useConflictQueued(sessionId: string, prNumber: number): boolean {
  return useCommentsStore((s) =>
    s.pendingForChat.some((id) => {
      const comment = s.byId[id];
      return (
        !!comment &&
        isPRFeedbackComment(comment) &&
        comment.feedbackType === "conflict" &&
        comment.sessionId === sessionId &&
        comment.prNumber === prNumber
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

  const detail = mapGitHubDetail(taskPR, feedback, metrics, {
    requestedReviewers: reviewRequest.requestedReviewers,
    requestingReviewers: reviewRequest.requestingReviewers,
    labels: {
      reRequestReview: t("github:reRequestReview"),
      requesting: t("github:requesting"),
    },
  });
  const liveState = feedback?.pr.state ?? taskPR.state;
  const isDraft = feedback?.pr.draft ?? false;
  const isMergeable = feedback?.pr.mergeable ?? true;
  const mergeableState = feedback?.pr.mergeable_state ?? taskPR.mergeable_state;
  const displayPR = { ...taskPR, state: liveState };

  return (
    <ChangeRequestDetail
      detail={detail}
      loading={loading}
      contentLoading={loading && !feedback}
      onRefresh={refresh}
      onAddContext={addAsContext}
      onAction={({ actionId, targetId }) => {
        if (actionId === "rerequest-review" && targetId) void reviewRequest.reRequest(targetId);
      }}
      headerActions={
        <>
          <ApproveButton
            workspaceId={workspaceId}
            taskPR={taskPR}
            feedback={feedback}
            onRefresh={refresh}
          />
          <PRMergeButton taskPR={taskPR} onMerged={refresh} />
        </>
      }
      notice={
        hasActiveMergeQueueEntry(displayPR) ? (
          <PRMergeQueueStatus pr={displayPR} />
        ) : (
          <PRMergeabilityNotice
            state={mergeableState}
            mergeable={isMergeable}
            isDraft={isDraft}
            prState={liveState}
            baseBranch={taskPR.base_branch}
            onResolveConflicts={onResolveConflicts}
            resolveDisabled={conflictQueued}
          />
        )
      }
    />
  );
}
