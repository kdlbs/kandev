"use client";

import { useEffect, useState } from "react";
import { IconAlertTriangle, IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { sortFindings } from "@/lib/review/findings";
import { NAVIGATE_FINDING_EVENT, type NavigateFindingEventDetail } from "@/lib/review/navigation";
import type { TaskReviewFinding } from "@/lib/types/review";
import { InlineReviewFinding } from "./inline-review-finding";

const STALE_REASON =
  "The diff changed after this review and the code this finding describes is no longer in it.";

/**
 * Findings for this file that can no longer be anchored to a line.
 *
 * Rendered above the diff rather than dropped or placed at their stored line: a
 * stale finding may still be valid, and putting it against whatever now occupies
 * that line number would be actively misleading. Collapsed by default so a file
 * with old findings still reads as a diff first.
 */
export function UnanchoredFindingsBanner({ findings }: { findings: TaskReviewFinding[] }) {
  const [expanded, setExpanded] = useState(false);
  const open = findings.filter((f) => f.status === "open");

  // A stale finding lives inside this collapsed banner, so a "go to finding"
  // from the overview can't scroll to a card that isn't in the DOM. Expand when
  // the navigation target is one of ours so its card renders and can be found.
  useEffect(() => {
    const onNavigate = (event: Event) => {
      const { findingId } = (event as CustomEvent<NavigateFindingEventDetail>).detail ?? {};
      if (findingId && findings.some((f) => f.status === "open" && f.id === findingId)) {
        setExpanded(true);
      }
    };
    window.addEventListener(NAVIGATE_FINDING_EVENT, onNavigate);
    return () => window.removeEventListener(NAVIGATE_FINDING_EVENT, onNavigate);
  }, [findings]);

  if (open.length === 0) return null;

  return (
    <div
      className="border-b border-border bg-muted/30 px-2 py-1.5"
      data-testid="unanchored-findings-banner"
    >
      <Button
        size="sm"
        variant="ghost"
        className="h-6 cursor-pointer gap-1 px-1 text-xs text-muted-foreground"
        onClick={() => setExpanded((value) => !value)}
        aria-expanded={expanded}
        data-testid="unanchored-findings-toggle"
      >
        {expanded ? (
          <IconChevronDown className="h-3.5 w-3.5" />
        ) : (
          <IconChevronRight className="h-3.5 w-3.5" />
        )}
        <IconAlertTriangle className="h-3.5 w-3.5" />
        {open.length} stale finding{open.length === 1 ? "" : "s"} for this file
      </Button>
      {expanded && (
        <div className="mt-1 space-y-1">
          {sortFindings(open).map((finding) => (
            <InlineReviewFinding key={finding.id} finding={finding} staleReason={STALE_REASON} />
          ))}
        </div>
      )}
    </div>
  );
}
