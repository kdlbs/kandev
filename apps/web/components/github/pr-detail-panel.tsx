"use client";

import { useCallback, useEffect } from "react";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import {
  IconRefresh,
  IconPlus,
  IconMinus,
  IconAlertTriangle,
  IconGitMerge,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Spinner } from "@kandev/ui/spinner";
import { useAppStore } from "@/components/state-provider";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { prPanelLabel } from "@/components/github/pr-utils";
import { usePRFeedback } from "@/hooks/domains/github/use-pr-feedback";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { PRFeedbackComment } from "@/lib/state/slices/comments";
import { useToast } from "@/components/toast-provider";
import type { TaskPR, PRFeedback } from "@/lib/types/github";
import { formatTimeAgo, AuthorLink } from "./pr-shared";
import { ReviewStateBadge } from "./pr-reviews-section";
import { ChecksSection } from "./pr-checks-section";
import { ReviewsSection } from "./pr-reviews-section";
import { CommentsSection } from "./pr-comments-section";

// --- Dockview panel wrapper ---

type PRDetailPanelProps = {
  panelId: string;
};

export function PRDetailPanelComponent({ panelId }: PRDetailPanelProps) {
  const pr = useActiveTaskPR();
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);

  useEffect(() => {
    const title = pr ? prPanelLabel(pr.pr_number) : "Pull Request";
    setPanelTitle(panelId, title);
  }, [pr, panelId]);

  if (!pr || !sessionId) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        No pull request linked to this session.
      </div>
    );
  }

  return <PRDetailContent taskPR={pr} sessionId={sessionId} />;
}

// --- Add PR feedback as chat context ---

function useAddPRFeedbackAsContext(sessionId: string, prNumber: number) {
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
      toast({ description: "Added to chat context" });
    },
    [sessionId, prNumber, addComment, toast],
  );

  return { addAsContext };
}

// --- Main content ---

function PRDetailContent({ taskPR, sessionId }: { taskPR: TaskPR; sessionId: string }) {
  const { feedback, loading, refresh } = usePRFeedback(taskPR.owner, taskPR.repo, taskPR.pr_number);
  const { addAsContext } = useAddPRFeedbackAsContext(sessionId, taskPR.pr_number);

  return (
    <div className="flex flex-col h-full">
      <PRHeader taskPR={taskPR} feedback={feedback} loading={loading} onRefresh={refresh} />
      <Separator />
      <ScrollArea className="flex-1 overflow-hidden">
        <div className="p-3 space-y-1">
          {loading && !feedback && (
            <div className="flex items-center justify-center py-8">
              <Spinner className="h-6 w-6" />
            </div>
          )}
          {feedback && (
            <>
              <ReviewsSection
                reviews={feedback.reviews}
                prUrl={taskPR.pr_url}
                reviewState={taskPR.review_state}
                reviewCount={taskPR.review_count}
                pendingReviewCount={taskPR.pending_review_count}
                onAddAsContext={(msg) => addAsContext("review", msg)}
              />
              <ChecksSection
                checks={feedback.checks}
                onAddAsContext={(msg) => addAsContext("check", msg)}
              />
              <CommentsSection
                comments={feedback.comments}
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
            Last synced {formatTimeAgo(taskPR.last_synced_at)}
          </div>
        </>
      )}
    </div>
  );
}

// --- Header ---

function StateBadge({ state }: { state: string }) {
  const styles: Record<string, string> = {
    open: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
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
        <TooltipContent>Refresh</TooltipContent>
      </Tooltip>
    </div>
  );
}

function HeaderDateLine({ taskPR }: { taskPR: TaskPR }) {
  return (
    <div className="flex items-center gap-1.5 text-xs text-muted-foreground flex-wrap">
      <span className="flex items-center gap-0.5">
        by <AuthorLink author={taskPR.author_login} />
      </span>
      <span>&middot;</span>
      <span>opened {formatTimeAgo(taskPR.created_at)}</span>
      {taskPR.merged_at && (
        <>
          <span>&middot;</span>
          <span className="flex items-center gap-0.5">
            <IconGitMerge className="h-3 w-3 text-purple-500" />
            merged {formatTimeAgo(taskPR.merged_at)}
          </span>
        </>
      )}
      {taskPR.closed_at && !taskPR.merged_at && (
        <>
          <span>&middot;</span>
          <span>closed {formatTimeAgo(taskPR.closed_at)}</span>
        </>
      )}
    </div>
  );
}

function HeaderStatsLine({ taskPR }: { taskPR: TaskPR }) {
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
        {taskPR.review_count} review{taskPR.review_count !== 1 ? "s" : ""}
        {taskPR.pending_review_count > 0 && (
          <span className="text-yellow-600 dark:text-yellow-400">
            {" "}
            ({taskPR.pending_review_count} pending)
          </span>
        )}
      </span>
      <span>&middot;</span>
      <span>
        {taskPR.comment_count} comment{taskPR.comment_count !== 1 ? "s" : ""}
      </span>
      {taskPR.review_state && <ReviewStateBadge state={taskPR.review_state} />}
    </div>
  );
}

function PRHeader({
  taskPR,
  feedback,
  loading,
  onRefresh,
}: {
  taskPR: TaskPR;
  feedback: PRFeedback | null;
  loading: boolean;
  onRefresh: () => void;
}) {
  const isDraft = feedback?.pr.draft ?? false;
  const isMergeable = feedback?.pr.mergeable ?? true;
  const showWarnings = isDraft || (!isMergeable && taskPR.state === "open");

  return (
    <div className="p-3 space-y-2">
      <HeaderTitleRow taskPR={taskPR} loading={loading} onRefresh={onRefresh} />
      <div className="flex items-center gap-1.5 flex-wrap">
        <StateBadge state={taskPR.state} />
        <span className="text-xs text-muted-foreground">#{taskPR.pr_number}</span>
        <span className="text-xs text-muted-foreground">
          {taskPR.head_branch} &rarr; {taskPR.base_branch}
        </span>
      </div>
      {showWarnings && (
        <div className="flex items-center gap-1.5 flex-wrap">
          {isDraft && (
            <Badge
              variant="secondary"
              className="text-[10px] px-1.5 py-0 bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400"
            >
              Draft
            </Badge>
          )}
          {!isMergeable && taskPR.state === "open" && (
            <span className="flex items-center gap-1 text-[10px] text-yellow-600 dark:text-yellow-400">
              <IconAlertTriangle className="h-3 w-3" />
              Not mergeable
            </span>
          )}
        </div>
      )}
      <HeaderDateLine taskPR={taskPR} />
      <HeaderStatsLine taskPR={taskPR} />
    </div>
  );
}
