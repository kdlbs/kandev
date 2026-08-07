"use client";

import { IconChevronDown, IconGitPullRequest } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@kandev/ui/lib/utils";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import { reviewItemId } from "./review-selection";

type ReviewItemSelectorProps = {
  reviews: readonly ReviewItemSummary[];
  selectedReview: ReviewItemSummary | null;
  onSelectReview: (review: ReviewItemSummary) => void;
  presentation: "desktop" | "mobile";
  className?: string;
};

export function ReviewItemSelector({
  reviews,
  selectedReview,
  onSelectReview,
  presentation,
  className,
}: ReviewItemSelectorProps) {
  const registry = usePluginRegistry();
  if (reviews.length < 2) return null;
  const selectedId = selectedReview ? reviewItemId(selectedReview) : "";
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="outline"
          data-testid="review-item-selector-trigger"
          className={cn(
            "max-w-full cursor-pointer gap-1.5 px-2 transition-transform active:scale-[0.96]",
            presentation === "mobile" ? "min-h-11" : "min-h-8",
            className,
          )}
        >
          <IconGitPullRequest className="h-4 w-4 shrink-0" />
          <span className="truncate text-xs font-medium">
            {selectedReview ? selectedReview.title : "Choose review"}
          </span>
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        data-testid="review-item-selector-menu"
        className="max-h-[calc(100dvh-1rem)] w-80 max-w-[calc(100vw-1rem)] overflow-y-auto overscroll-contain sm:max-h-80"
      >
        <DropdownMenuLabel>Choose review</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuRadioGroup
          value={selectedId}
          onValueChange={(nextId) => {
            const nextReview = reviews.find((review) => reviewItemId(review) === nextId);
            if (nextReview) onSelectReview(nextReview);
          }}
        >
          {reviews.map((review) => (
            <DropdownMenuRadioItem
              key={reviewItemId(review)}
              value={reviewItemId(review)}
              data-testid={`review-item-selector-item-${reviewItemId(review)}`}
              className="min-h-11 cursor-pointer gap-2 py-2.5 pr-8"
            >
              <IconGitPullRequest className="h-4 w-4 shrink-0" />
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-sm font-medium">{review.title}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {providerLabel(
                    review.providerId,
                    registry.getReviewProvider(review.providerId)?.label,
                  )}{" "}
                  · {review.repositoryId}
                </span>
              </span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function providerLabel(providerId: string, registeredLabel?: string): string {
  if (providerId === "github") return "GitHub";
  if (providerId === "gitlab") return "GitLab";
  return registeredLabel ?? providerId;
}
