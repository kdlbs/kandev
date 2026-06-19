"use client";

import { useCallback } from "react";
import { IconAlertTriangle, IconClock } from "@tabler/icons-react";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import {
  useCommentsStore,
  isPRFeedbackComment,
  type PRFeedbackComment,
} from "@/lib/state/slices/comments";
import type { TaskPR } from "@/lib/types/github";
import { isPRAwaitingReview, isPRWaitingOnBranchProtection } from "./pr-task-icon";
import { PRMergeabilityNotice, buildConflictResolutionMessage } from "./pr-mergeability-notice";

/**
 * Wires the "Resolve conflicts" CTA to chat context, mirroring the PR detail
 * panel (`useAddPRFeedbackAsContext` + `conflictQueued`). Returns a null handler
 * when there is no active session so the button is suppressed rather than no-op.
 */
function useResolveConflicts(pr: TaskPR): {
  onResolveConflicts: (() => void) | null;
  conflictQueued: boolean;
} {
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const addComment = useCommentsStore((s) => s.addComment);
  const { toast } = useToast();
  const prNumber = pr.pr_number;
  const headBranch = pr.head_branch;
  const baseBranch = pr.base_branch;

  // True once a conflict prompt for this PR is already queued — avoids piling
  // up identical instructions if the user clicks "Resolve conflicts" again.
  const conflictQueued = useCommentsStore((s) =>
    s.pendingForChat.some((id) => {
      const c = s.byId[id];
      return (
        !!c &&
        isPRFeedbackComment(c) &&
        c.feedbackType === "conflict" &&
        c.sessionId === sessionId &&
        c.prNumber === prNumber
      );
    }),
  );

  const handler = useCallback(() => {
    if (!sessionId || conflictQueued) return;
    const content = buildConflictResolutionMessage({ prNumber, headBranch, baseBranch });
    const comment: PRFeedbackComment = {
      id: `pr-feedback-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      sessionId,
      text: content,
      createdAt: new Date().toISOString(),
      status: "pending",
      source: "pr-feedback",
      prNumber,
      feedbackType: "conflict",
      content,
    };
    addComment(comment);
    toast({ description: "Added to chat context" });
  }, [sessionId, conflictQueued, prNumber, headBranch, baseBranch, addComment, toast]);

  return { onResolveConflicts: sessionId ? handler : null, conflictQueued };
}

/**
 * Mergeability cue for the CI hover popover. Surfaces the conflict banner
 * ("dirty") and the Blocked / Behind-base chips so an unmergeable PR no longer
 * silently drops its merge button with no explanation. Renders nothing for
 * clean / unstable / has_hooks and for transient unknown/empty states (we pass
 * `mergeable: true` so the popover stays quiet instead of flapping "Not
 * mergeable" while GitHub recomputes).
 */
export function PRMergeabilityRow({ pr }: { pr: TaskPR }) {
  const { onResolveConflicts, conflictQueued } = useResolveConflicts(pr);
  // "blocked" gets a richer note than the bare chip: it explains *why* the
  // merge is gated. But when the block is only an outstanding requested review,
  // stay silent — the review row above already conveys that and the
  // pill/chip/badge deliberately read it as the calm "awaiting review" state,
  // so a "Blocked by branch protection" note (or even the bare chip) here would
  // contradict them.
  if (pr.state === "open" && pr.mergeable_state === "blocked") {
    if (isPRAwaitingReview(pr)) return null;
    if (isPRWaitingOnBranchProtection(pr)) {
      return <BranchProtectionWaitNote reason={blockedReason(pr)} />;
    }
    return <BlockedNote reason={blockedReason(pr)} />;
  }
  return (
    <PRMergeabilityNotice
      popover
      state={pr.mergeable_state}
      mergeable
      isDraft={pr.mergeable_state === "draft"}
      prState={pr.state}
      baseBranch={pr.base_branch}
      onResolveConflicts={onResolveConflicts ?? undefined}
      resolveDisabled={conflictQueued}
    />
  );
}

/**
 * Best-effort explanation for a `blocked` mergeable state. GitHub doesn't tell
 * us which branch-protection rule failed, so we infer the common cases from the
 * fields we do have (required reviews, CI state) and fall back to a generic
 * note for the rest (code owners, required conversations, etc.).
 */
export function blockedReason(pr: TaskPR): string {
  const required = pr.required_reviews ?? null;
  if (required != null && pr.review_count < required) {
    const missing = required - pr.review_count;
    return `Needs ${missing} more approval${missing === 1 ? "" : "s"} before it can merge.`;
  }
  // Only "failure"/"pending" indicate an actual check problem; "" means no CI
  // is configured (or it hasn't loaded), which isn't a status-check block.
  if (pr.checks_state === "failure" || pr.checks_state === "pending") {
    return "A required status check hasn't passed yet.";
  }
  return "Required reviews, code owners, or repository rules still need to clear.";
}

function BlockedNote({ reason }: { reason: string }) {
  return (
    <div data-testid="pr-blocked-note" className="flex items-start gap-1.5 px-1 py-1 text-xs">
      <IconAlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-600 dark:text-amber-400" />
      <div className="min-w-0">
        <div className="font-medium text-amber-600 dark:text-amber-400">
          Blocked by branch protection
        </div>
        <p className="text-[11px] leading-snug text-muted-foreground">{reason}</p>
      </div>
    </div>
  );
}

function BranchProtectionWaitNote({ reason }: { reason: string }) {
  return (
    <div
      data-testid="pr-branch-protection-wait-note"
      className="flex items-start gap-1.5 px-1 py-1 text-xs"
    >
      <IconClock className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      <div className="min-w-0">
        <div className="font-medium text-muted-foreground">Waiting on branch protection</div>
        <p className="text-[11px] leading-snug text-muted-foreground">{reason}</p>
      </div>
    </div>
  );
}
