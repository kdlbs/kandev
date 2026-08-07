import { IconCheck, IconClock, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { CollapsibleSection } from "@/components/github/pr-shared";
import { ChangeRequestActionButton, ChangeRequestPersonLink } from "./change-request-detail-header";
import {
  ChangeRequestFeedbackRow,
  normalizeChangeRequestPersonName,
} from "./change-request-detail-feedback";
import type {
  ChangeRequestDetailModel,
  ChangeRequestDetailProps,
  ChangeRequestDetailReview,
} from "./change-request-detail";

function buildReviewContext(review: ChangeRequestDetailReview, url: string) {
  return [
    `Review from **${review.author.name}** (${review.state}):`,
    review.body ?? "",
    `PR: ${url}`,
    "Please address this review feedback.",
  ]
    .filter(Boolean)
    .join("\n\n");
}

function reviewStateLabel(state: string) {
  const labels: Record<string, string> = {
    APPROVED: "Approved",
    CHANGES_REQUESTED: "Changes requested",
    COMMENTED: "Commented",
    DISMISSED: "Dismissed",
    PENDING: "Pending",
  };
  return labels[state.toUpperCase()] ?? state.toLowerCase().replaceAll("_", " ");
}

function ReviewStateIcon({ state }: { state: string }) {
  const normalized = state.toUpperCase();
  if (normalized === "APPROVED") {
    return <IconCheck className="h-3.5 w-3.5 shrink-0 text-green-500" />;
  }
  if (normalized === "CHANGES_REQUESTED") {
    return <IconX className="h-3.5 w-3.5 shrink-0 text-red-500" />;
  }
  return <IconClock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />;
}

function ReviewStateBadge({ state }: { state: string }) {
  const normalized = state.toLowerCase();
  const styles: Record<string, string> = {
    approved: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    changes_requested: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
    pending: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  };
  return (
    <Badge variant="secondary" className={`px-1.5 py-0 text-[10px] ${styles[normalized] ?? ""}`}>
      {reviewStateLabel(state)}
    </Badge>
  );
}

function reviewSummary(reviews: ChangeRequestDetailReview[]) {
  const approved = reviews.filter((review) => review.state.toUpperCase() === "APPROVED").length;
  const changes = reviews.filter(
    (review) => review.state.toUpperCase() === "CHANGES_REQUESTED",
  ).length;
  const parts: string[] = [];
  if (approved > 0) parts.push(`${approved} approved`);
  if (changes > 0) parts.push(`${changes} changes requested`);
  const other = reviews.length - approved - changes;
  if (other > 0) parts.push(`${other} other`);
  return parts;
}

function buildAllReviewsContext(reviews: ChangeRequestDetailReview[], url: string) {
  const lines = ["### All PR Reviews", ""];
  for (const review of reviews) {
    lines.push(`**${review.author.name}** — ${reviewStateLabel(review.state)}`);
    if (review.body) lines.push(review.body);
    lines.push("");
  }
  lines.push(`PR: ${url}`, "Please address the review feedback above.");
  return lines.join("\n");
}

function latestReviews(reviews: ChangeRequestDetailReview[]) {
  const latest = new Map<string, ChangeRequestDetailReview>();
  for (const review of reviews) {
    const key = normalizeChangeRequestPersonName(review.author.name);
    const current = latest.get(key);
    if (!current || (review.createdAt ?? "") >= (current.createdAt ?? "")) {
      latest.set(key, review);
    }
  }
  return [...latest.values()];
}

export function ChangeRequestReviews({
  detail,
  onAdd,
  onAction,
}: {
  detail: ChangeRequestDetailModel;
  onAdd?: ChangeRequestDetailProps["onAddContext"];
  onAction?: ChangeRequestDetailProps["onAction"];
}) {
  const requestedNames = new Set(
    detail.requestedReviewers.map((person) => normalizeChangeRequestPersonName(person.name)),
  );
  const reviews = latestReviews(detail.reviews).filter(
    (review) => !requestedNames.has(normalizeChangeRequestPersonName(review.author.name)),
  );
  const pendingCount = detail.requestedReviewers.length || detail.pendingReviewCount || 0;
  const summaryParts = reviewSummary(reviews);
  if (pendingCount > 0) summaryParts.push(`${pendingCount} pending`);
  const summary = summaryParts.length > 0 ? ` — ${summaryParts.join(", ")}` : "";
  const subtitle = detail.reviewState ? (
    <div className="px-2 pb-1 text-[10px] text-muted-foreground">
      Overall: <ReviewStateBadge state={detail.reviewState} />
      {pendingCount > 0 ? (
        <span className="text-yellow-600 dark:text-yellow-400"> ({pendingCount} pending)</span>
      ) : null}
    </div>
  ) : null;
  return (
    <CollapsibleSection
      title={`Reviews${summary}`}
      count={reviews.length + pendingCount}
      defaultOpen
      subtitle={subtitle}
      onAddAll={
        onAdd && reviews.length > 0
          ? () => onAdd("review", buildAllReviewsContext(reviews, detail.url))
          : undefined
      }
      addAllLabel="Add all reviews to chat context"
    >
      {reviews.length + pendingCount === 0 ? (
        <p className="px-2 py-2 text-xs text-muted-foreground">No reviews yet</p>
      ) : null}
      {reviews.map((review) => (
        <div
          key={review.id}
          className="space-y-1.5"
          data-testid={`change-request-submitted-review-${normalizeChangeRequestPersonName(review.author.name)}`}
        >
          <ChangeRequestFeedbackRow
            person={review.author}
            body={review.body}
            createdAt={review.createdAt}
            metadata={
              <>
                <ReviewStateIcon state={review.state} />
                <span className="truncate text-[10px] text-muted-foreground">
                  {reviewStateLabel(review.state)}
                </span>
              </>
            }
            onAdd={
              onAdd ? () => onAdd("review", buildReviewContext(review, detail.url)) : undefined
            }
          />
          {review.actions?.map((action) => (
            <ChangeRequestActionButton
              key={action.id}
              action={{ ...action, placement: "thread" }}
              busy={action.busy ?? false}
              onAction={() =>
                void onAction?.({ actionId: action.id, targetId: review.author.name })
              }
              ariaLabel={`${action.label} from ${review.author.name}`}
              testId={`change-request-review-action-${action.id}-${normalizeChangeRequestPersonName(review.author.name)}`}
            />
          ))}
        </div>
      ))}
      {detail.requestedReviewers.map((person) => (
        <div
          key={person.name}
          className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1.5"
          data-testid={`change-request-pending-reviewer-${normalizeChangeRequestPersonName(person.name)}`}
        >
          <IconClock className="h-3.5 w-3.5 shrink-0 text-yellow-500" />
          <ChangeRequestPersonLink
            person={{ ...person, name: `${person.name}${person.kind === "team" ? " (team)" : ""}` }}
          />
          <span className="text-[10px] text-muted-foreground">Pending review</span>
        </div>
      ))}
    </CollapsibleSection>
  );
}
