"use client";

import { useCallback, useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { openFindingCount } from "@/lib/review/findings";
import { navigateToFinding } from "@/lib/review/navigation";
import type { TaskReviewFinding } from "@/lib/types/review";
import { ReviewFindingsOverview } from "./review-findings-overview";

export type ReviewFindingsButtonProps = {
  findings: TaskReviewFinding[];
  /** Selects a file in the review diff so its section expands and scrolls in. */
  onSelectFile: (fileKey: string) => void;
};

/**
 * The findings-count control on the review top bar. Instead of a passive badge,
 * it opens a popover listing every open finding grouped by file; clicking one
 * jumps to and flashes that finding's card in the diff. Renders nothing when
 * there is nothing to navigate to.
 */
export function ReviewFindingsButton({ findings, onSelectFile }: ReviewFindingsButtonProps) {
  const [open, setOpen] = useState(false);
  const count = openFindingCount(findings);

  const handleNavigate = useCallback(
    (finding: TaskReviewFinding) => {
      setOpen(false);
      // Fire-and-forget: the popover is already closing. If the card never
      // renders within the retry budget, warn rather than fail silently.
      navigateToFinding(finding, onSelectFile).then((reached) => {
        if (!reached) {
          console.warn("[review] could not scroll to finding:", finding.id);
        }
      });
    },
    [onSelectFile],
  );

  if (count === 0) return null;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className="h-6 cursor-pointer gap-1 px-1.5 text-[10px]"
          aria-label={`Go to ${count} review finding${count === 1 ? "" : "s"}`}
          data-testid="review-open-count"
        >
          {count} finding{count === 1 ? "" : "s"}
          <IconChevronDown className="h-3 w-3" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-0" data-testid="review-findings-popover">
        <ReviewFindingsOverview findings={findings} onNavigate={handleNavigate} />
      </PopoverContent>
    </Popover>
  );
}
