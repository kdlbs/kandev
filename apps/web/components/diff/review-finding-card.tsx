"use client";

import ReactMarkdown from "react-markdown";
import { IconArrowBackUp, IconCheck, IconEyeOff, IconMessagePlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  markdownComponents,
  normalizeMarkdown,
  remarkPlugins,
} from "@/components/shared/markdown-components";
import { findingLocation } from "@/lib/review/format";
import type { TaskReviewFinding } from "@/lib/types/review";
import { ReviewFindingSeverityBadge } from "./review-finding-severity";

export type ReviewFindingCardProps = {
  finding: TaskReviewFinding;
  onResolve?: (finding: TaskReviewFinding) => void;
  onDismiss?: (finding: TaskReviewFinding) => void;
  onReopen?: (finding: TaskReviewFinding) => void;
  onSendToAgent?: (finding: TaskReviewFinding) => void;
  /** Shown when the finding could not be anchored to a current line. */
  staleReason?: string;
  /** Renders the location line; off inside a diff where the line is implicit. */
  showLocation?: boolean;
};

function FindingActions({
  finding,
  onResolve,
  onDismiss,
  onReopen,
  onSendToAgent,
}: Omit<ReviewFindingCardProps, "staleReason" | "showLocation">) {
  const isOpen = finding.status === "open";
  return (
    <div className="mt-2 flex flex-wrap items-center gap-1">
      {onSendToAgent && (
        <Button
          size="sm"
          variant="ghost"
          className="h-6 cursor-pointer gap-1 px-1.5 text-xs"
          onClick={() => onSendToAgent(finding)}
          data-testid="review-finding-send-to-agent"
        >
          <IconMessagePlus className="h-3.5 w-3.5" />
          Send to agent
        </Button>
      )}
      {isOpen && onResolve && (
        <Button
          size="sm"
          variant="ghost"
          className="h-6 cursor-pointer gap-1 px-1.5 text-xs text-emerald-600 dark:text-emerald-400"
          onClick={() => onResolve(finding)}
          data-testid="review-finding-resolve"
        >
          <IconCheck className="h-3.5 w-3.5" />
          Resolve
        </Button>
      )}
      {isOpen && onDismiss && (
        <Button
          size="sm"
          variant="ghost"
          className="h-6 cursor-pointer gap-1 px-1.5 text-xs text-muted-foreground"
          onClick={() => onDismiss(finding)}
          data-testid="review-finding-dismiss"
        >
          <IconEyeOff className="h-3.5 w-3.5" />
          Dismiss
        </Button>
      )}
      {!isOpen && onReopen && (
        <Button
          size="sm"
          variant="ghost"
          className="h-6 cursor-pointer gap-1 px-1.5 text-xs"
          onClick={() => onReopen(finding)}
          data-testid="review-finding-reopen"
        >
          <IconArrowBackUp className="h-3.5 w-3.5" />
          Reopen
        </Button>
      )}
    </div>
  );
}

/**
 * A suggested change is display-only in this iteration: it is never applied,
 * staged, or committed, so the card says so next to the code rather than
 * offering an action the feature does not have.
 */
function FindingSuggestion({ suggestion }: { suggestion: string }) {
  return (
    <div className="mt-2">
      <p className="mb-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
        Suggested change — not applied automatically
      </p>
      <pre className="overflow-x-auto rounded bg-muted p-1.5 text-[11px] leading-tight">
        <code>{suggestion}</code>
      </pre>
    </div>
  );
}

/**
 * One anchored, advisory review finding.
 *
 * Resolved and dismissed findings collapse to a single muted line with an Undo,
 * so a reviewed diff does not stay cluttered with issues the human has already
 * dealt with while still letting them change their mind.
 */
export function ReviewFindingCard(props: ReviewFindingCardProps) {
  const { finding, staleReason, showLocation = false } = props;
  const isOpen = finding.status === "open";

  if (!isOpen) {
    return (
      <div
        className="flex items-center gap-2 rounded-md border border-border bg-muted/40 px-2 py-1 text-xs text-muted-foreground"
        data-testid="review-finding-card"
        data-finding-status={finding.status}
      >
        <IconCheck className="h-3.5 w-3.5 shrink-0" />
        <span className="min-w-0 flex-1 truncate line-through">{finding.title}</span>
        <span className="shrink-0 capitalize">{finding.status}</span>
        {props.onReopen && (
          <Button
            size="sm"
            variant="ghost"
            className="h-5 cursor-pointer px-1 text-xs"
            onClick={() => props.onReopen?.(finding)}
            data-testid="review-finding-reopen"
          >
            Undo
          </Button>
        )}
      </div>
    );
  }

  return (
    <div
      className="rounded-md border border-border bg-card p-2 shadow-sm"
      data-testid="review-finding-card"
      data-finding-status={finding.status}
      data-finding-severity={finding.severity}
    >
      <div className="mb-1 flex flex-wrap items-center gap-1.5">
        <ReviewFindingSeverityBadge severity={finding.severity} />
        {finding.category && (
          <Badge variant="outline" className="px-1.5 py-0 text-[10px] text-muted-foreground">
            {finding.category}
          </Badge>
        )}
        {staleReason && (
          <Badge
            variant="outline"
            className="px-1.5 py-0 text-[10px] text-muted-foreground"
            title={staleReason}
            data-testid="review-finding-stale"
          >
            Stale
          </Badge>
        )}
      </div>
      {showLocation && (
        <p className="mb-1 font-mono text-[11px] text-muted-foreground">
          {findingLocation(finding)}
        </p>
      )}
      <p className="text-xs font-semibold leading-snug" data-testid="review-finding-title">
        {finding.title}
      </p>
      {staleReason && <p className="mt-1 text-[11px] text-muted-foreground">{staleReason}</p>}
      <div
        className="prose prose-sm dark:prose-invert mt-1 max-w-none text-xs leading-relaxed [overflow-wrap:anywhere] [&_p]:my-1"
        data-testid="review-finding-body"
      >
        <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
          {normalizeMarkdown(finding.body)}
        </ReactMarkdown>
      </div>
      {finding.suggestion && <FindingSuggestion suggestion={finding.suggestion} />}
      <FindingActions {...props} />
    </div>
  );
}
