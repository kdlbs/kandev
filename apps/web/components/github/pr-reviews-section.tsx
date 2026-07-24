import { IconCheck, IconX, IconClock } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { PRReview, RequestedReviewer } from "@/lib/types/github";
import { CollapsibleSection, FeedbackItemRow } from "./pr-shared";

function ReviewStateIcon({ state }: { state: string }) {
  if (state === "APPROVED") return <IconCheck className="h-3.5 w-3.5 text-green-500 shrink-0" />;
  if (state === "CHANGES_REQUESTED") return <IconX className="h-3.5 w-3.5 text-red-500 shrink-0" />;
  return <IconClock className="h-3.5 w-3.5 text-muted-foreground shrink-0" />;
}

function reviewStateLabel(state: string): string {
  const labels: Record<string, string> = {
    APPROVED: "Approved",
    CHANGES_REQUESTED: "Changes Requested",
    COMMENTED: "Commented",
    PENDING: "Pending",
    DISMISSED: "Dismissed",
  };
  return labels[state] ?? state;
}

export function ReviewStateBadge({ state }: { state: string }) {
  if (!state) return null;
  const styles: Record<string, string> = {
    approved: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    changes_requested: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
    pending: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  };
  const labelMap: Record<string, string> = {
    approved: "Approved",
    changes_requested: "Changes requested",
    pending: "Review pending",
  };
  return (
    <Badge variant="secondary" className={`text-[10px] px-1.5 py-0 ${styles[state] ?? ""}`}>
      {labelMap[state] ?? state}
    </Badge>
  );
}

function buildReviewMessage(review: PRReview, prUrl: string): string {
  const parts = [`Review from **${review.author}** (${review.state}):`];
  if (review.body) parts.push(review.body);
  parts.push(`PR: ${prUrl}`);
  parts.push("Please address this review feedback.");
  return parts.join("\n\n");
}

function buildAllReviewsMessage(reviews: PRReview[], prUrl: string): string {
  const parts = ["### All PR Reviews", ""];
  for (const r of reviews) {
    parts.push(`**${r.author}** — ${reviewStateLabel(r.state)}`);
    if (r.body) parts.push(r.body);
    parts.push("");
  }
  parts.push(`PR: ${prUrl}`);
  parts.push("Please address the review feedback above.");
  return parts.join("\n");
}

function normalizeLogin(login: string): string {
  return login.trim().toLowerCase();
}

export function reconcileReviews(reviews: PRReview[], requestedReviewers: RequestedReviewer[]) {
  const latestByAuthor = new Map<string, PRReview>();
  for (const review of reviews) {
    const author = normalizeLogin(review.author);
    const current = latestByAuthor.get(author);
    if (!current || new Date(review.created_at) > new Date(current.created_at)) {
      latestByAuthor.set(author, review);
    }
  }
  const requestedByLogin = new Map<string, RequestedReviewer>();
  for (const reviewer of requestedReviewers) {
    const login = normalizeLogin(reviewer.login);
    if (!requestedByLogin.has(login)) requestedByLogin.set(login, reviewer);
  }
  return {
    reviews: Array.from(latestByAuthor.entries())
      .filter(([author]) => !requestedByLogin.has(author))
      .map(([, review]) => review),
    pendingReviewers: Array.from(requestedByLogin.values()),
  };
}

function formatPendingReviewer(reviewer: RequestedReviewer): string {
  if (reviewer.type === "team") return `${reviewer.login} (team)`;
  return reviewer.login;
}

function formatReviewSummary(reviews: PRReview[]): string {
  const approved = reviews.filter((r) => r.state === "APPROVED").length;
  const changes = reviews.filter((r) => r.state === "CHANGES_REQUESTED").length;
  const parts: string[] = [];
  if (approved > 0) parts.push(`${approved} approved`);
  if (changes > 0) parts.push(`${changes} changes requested`);
  const other = reviews.length - approved - changes;
  if (other > 0) parts.push(`${other} other`);
  return parts.join(", ");
}

function computeSectionSummary(
  reviews: PRReview[],
  requestedReviewers: RequestedReviewer[],
  pendingReviewCount: number,
) {
  const submittedSummary = reviews.length > 0 ? formatReviewSummary(reviews) : "";
  const pendingCount =
    requestedReviewers.length > 0 ? requestedReviewers.length : pendingReviewCount;
  const summaryParts: string[] = [];
  if (submittedSummary) summaryParts.push(submittedSummary);
  if (pendingCount > 0) summaryParts.push(`${pendingCount} pending`);
  const summary = summaryParts.length > 0 ? ` \u2014 ${summaryParts.join(", ")}` : "";
  return { pendingCount, summary, totalCount: reviews.length + pendingCount };
}

function ReviewMetaBadge({ state }: { state: string }) {
  return (
    <>
      <ReviewStateIcon state={state} />
      <span className="text-[10px] text-muted-foreground truncate">{reviewStateLabel(state)}</span>
    </>
  );
}

function SubmittedReviewRow({
  review,
  prUrl,
  onAddAsContext,
  canReRequest,
  isRequesting,
  onReRequest,
}: {
  review: PRReview;
  prUrl: string;
  onAddAsContext: (message: string) => void;
  canReRequest: boolean;
  isRequesting: boolean;
  onReRequest?: (reviewer: string) => void;
}) {
  const hasActionable = review.state === "CHANGES_REQUESTED" || !!review.body;
  return (
    <div data-testid={`pr-submitted-review-${normalizeLogin(review.author)}`}>
      <FeedbackItemRow
        author={review.author}
        authorAvatar={review.author_avatar}
        body={review.body || undefined}
        createdAt={review.created_at}
        metaBadge={<ReviewMetaBadge state={review.state} />}
        trailingAction={
          canReRequest && review.state === "DISMISSED" && onReRequest ? (
            <div className="basis-full w-full sm:basis-auto sm:w-auto">
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="h-8 min-h-11 w-full max-w-full sm:w-auto sm:min-h-0 shrink-0 cursor-pointer px-2 text-[10px] [@media(pointer:coarse)]:min-h-11"
                data-testid={`pr-rerequest-review-${normalizeLogin(review.author)}`}
                aria-label={`Re-request review from ${review.author}`}
                onClick={() => onReRequest(review.author)}
                disabled={isRequesting}
              >
                {isRequesting ? "Requesting..." : "Re-request review"}
              </Button>
            </div>
          ) : undefined
        }
        onAddAsContext={
          hasActionable ? () => onAddAsContext(buildReviewMessage(review, prUrl)) : undefined
        }
      />
    </div>
  );
}

function PendingReviewRow({ reviewer }: { reviewer: RequestedReviewer }) {
  return (
    <div
      className="px-2.5 py-1.5 rounded-md border border-border bg-muted/30"
      data-testid={`pr-pending-reviewer-${normalizeLogin(reviewer.login)}`}
    >
      <div className="flex items-center gap-2">
        <IconClock className="h-3.5 w-3.5 text-yellow-500 shrink-0" />
        <span className="text-xs font-medium">{formatPendingReviewer(reviewer)}</span>
        <span className="text-[10px] text-muted-foreground truncate">Pending review</span>
      </div>
    </div>
  );
}

export function ReviewsSection({
  reviews,
  requestedReviewers,
  prUrl,
  reviewState,
  pendingReviewCount,
  onAddAsContext,
  canReRequest = false,
  requestingReviewers = [],
  onReRequest,
}: {
  reviews: PRReview[];
  requestedReviewers: RequestedReviewer[];
  prUrl: string;
  reviewState: string;
  pendingReviewCount: number;
  onAddAsContext: (message: string) => void;
  canReRequest?: boolean;
  requestingReviewers?: string[];
  onReRequest?: (reviewer: string) => void;
}) {
  const { reviews: reconciledReviews, pendingReviewers } = reconcileReviews(
    reviews,
    requestedReviewers,
  );
  const requestingReviewerLogins = new Set(requestingReviewers.map(normalizeLogin));

  const { pendingCount, summary, totalCount } = computeSectionSummary(
    reconciledReviews,
    pendingReviewers,
    pendingReviewCount,
  );
  const pendingText = pendingCount > 0 ? ` (${pendingCount} pending)` : "";

  const subtitle = reviewState ? (
    <div className="text-[10px] text-muted-foreground px-2 pb-1">
      Overall: <ReviewStateBadge state={reviewState} />
      {pendingText && <span className="text-yellow-600 dark:text-yellow-400">{pendingText}</span>}
    </div>
  ) : null;

  return (
    <CollapsibleSection
      title={`Reviews${summary}`}
      count={totalCount}
      defaultOpen
      subtitle={subtitle}
      onAddAll={
        reconciledReviews.length > 0
          ? () => onAddAsContext(buildAllReviewsMessage(reconciledReviews, prUrl))
          : undefined
      }
      addAllLabel="Add all reviews to chat context"
    >
      {reconciledReviews.length === 0 && pendingCount === 0 && (
        <p className="text-xs text-muted-foreground px-2 py-2">No reviews yet</p>
      )}
      {reconciledReviews.map((review) => (
        <SubmittedReviewRow
          key={review.id}
          review={review}
          prUrl={prUrl}
          onAddAsContext={onAddAsContext}
          canReRequest={canReRequest}
          isRequesting={requestingReviewerLogins.has(normalizeLogin(review.author))}
          onReRequest={onReRequest}
        />
      ))}
      {pendingReviewers.map((reviewer) => (
        <PendingReviewRow key={`pending-${reviewer.type}-${reviewer.login}`} reviewer={reviewer} />
      ))}
    </CollapsibleSection>
  );
}
