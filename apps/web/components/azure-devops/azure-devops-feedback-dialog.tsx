"use client";

import { Badge } from "@kandev/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Separator } from "@kandev/ui/separator";
import type { AzureDevOpsPullRequestFeedback } from "@/lib/types/azure-devops";

function voteLabel(vote: number): string {
  if (vote >= 10) return "Approved";
  if (vote >= 5) return "Approved with suggestions";
  if (vote <= -10) return "Rejected";
  if (vote <= -5) return "Waiting for author";
  return "No vote";
}

function Summary({ feedback }: { feedback: AzureDevOpsPullRequestFeedback }) {
  return (
    <div className="flex flex-wrap gap-2">
      <Badge variant="outline">Review: {feedback.reviewState || "pending"}</Badge>
      <Badge variant="outline">Policies: {feedback.policyState || "none"}</Badge>
      <Badge variant="secondary">{feedback.linkedWorkItems.length} linked items</Badge>
    </div>
  );
}

function Reviewers({ feedback }: { feedback: AzureDevOpsPullRequestFeedback }) {
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-semibold">Reviewers</h3>
      {feedback.reviewers.length === 0 ? (
        <p className="text-sm text-muted-foreground">No reviewers.</p>
      ) : (
        <div className="space-y-2">
          {feedback.reviewers.map((reviewer) => (
            <div key={reviewer.id} className="flex items-center justify-between gap-3 text-sm">
              <span className="min-w-0 truncate">{reviewer.displayName}</span>
              <Badge variant="outline" className="shrink-0">
                {voteLabel(reviewer.vote)}
              </Badge>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function Policies({ feedback }: { feedback: AzureDevOpsPullRequestFeedback }) {
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-semibold">Branch policies</h3>
      {feedback.policies.length === 0 ? (
        <p className="text-sm text-muted-foreground">No policy evaluations.</p>
      ) : (
        <div className="space-y-2">
          {feedback.policies.map((policy) => (
            <div key={policy.id} className="flex items-center justify-between gap-3 text-sm">
              <span className="min-w-0 break-words">{policy.name}</span>
              <Badge variant="outline" className="shrink-0">
                {policy.status}
              </Badge>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function Threads({ feedback }: { feedback: AzureDevOpsPullRequestFeedback }) {
  const comments = feedback.threads.flatMap((thread) =>
    thread.comments.map((comment) => ({ ...comment, threadId: thread.id })),
  );
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-semibold">Discussion</h3>
      {comments.length === 0 ? (
        <p className="text-sm text-muted-foreground">No comments.</p>
      ) : (
        <div className="space-y-3">
          {comments.map((comment) => (
            <div key={`${comment.threadId}:${comment.id}`} className="space-y-1 border-l-2 pl-3">
              <div className="text-xs font-medium">{comment.author.displayName}</div>
              <p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">
                {comment.content}
              </p>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

export function AzureDevOpsFeedbackDialog({
  open,
  loading,
  error,
  feedback,
  onOpenChange,
}: {
  open: boolean;
  loading: boolean;
  error: string | null;
  feedback: AzureDevOpsPullRequestFeedback | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85dvh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="break-words">
            {feedback?.pullRequest.title ?? "Pull request feedback"}
          </DialogTitle>
          <DialogDescription>
            {feedback
              ? `${feedback.pullRequest.repositoryName} · PR ${feedback.pullRequest.id}`
              : "Azure DevOps review and policy state"}
          </DialogDescription>
        </DialogHeader>
        {loading && <p className="text-sm text-muted-foreground">Loading feedback...</p>}
        {error && (
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
        )}
        {feedback && (
          <div className="space-y-4" data-testid="azure-devops-feedback-detail">
            <Summary feedback={feedback} />
            <Separator />
            <Reviewers feedback={feedback} />
            <Separator />
            <Policies feedback={feedback} />
            <Separator />
            <Threads feedback={feedback} />
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
