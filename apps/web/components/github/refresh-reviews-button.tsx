"use client";

import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useRefreshReviews } from "@/hooks/domains/github/use-refresh-reviews";

export function RefreshReviewsButton() {
  const { available, loading, trigger } = useRefreshReviews();

  if (!available) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="outline"
          size="icon"
          className="cursor-pointer"
          onClick={trigger}
          disabled={loading}
        >
          <IconRefresh className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Check for PRs to review</TooltipContent>
    </Tooltip>
  );
}
