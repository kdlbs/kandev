"use client";

import { useAppStore } from "@/components/state-provider";
import { useFindingActions } from "@/hooks/domains/review/use-finding-actions";
import { useSendFindingToAgent } from "@/hooks/domains/review/use-send-finding-to-agent";
import { FINDING_DOM_ATTR } from "@/lib/review/navigation";
import type { TaskReviewFinding } from "@/lib/types/review";
import { ReviewFindingCard } from "./review-finding-card";

/**
 * One review finding rendered inline at its anchored diff line.
 *
 * This container wires its own actions from the store rather than taking them as
 * props: an inline finding annotation only exists when the active task has
 * findings, so the store is always present, and threading four callbacks through
 * FileDiffViewer → DiffViewer → the annotation renderer would add plumbing that
 * every other diff consumer would have to carry.
 */
export function InlineReviewFinding({
  finding,
  staleReason,
}: {
  finding: TaskReviewFinding;
  staleReason?: string;
}) {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const { resolveFinding, dismissFinding, reopenFinding } = useFindingActions(taskId);
  const sendToAgent = useSendFindingToAgent({ taskId, sessionId });

  return (
    <div className="my-1 scroll-mt-24 px-2 rounded-md" {...{ [FINDING_DOM_ATTR]: finding.id }}>
      <ReviewFindingCard
        finding={finding}
        staleReason={staleReason}
        onResolve={resolveFinding}
        onDismiss={dismissFinding}
        onReopen={reopenFinding}
        onSendToAgent={sessionId ? sendToAgent : undefined}
      />
    </div>
  );
}
