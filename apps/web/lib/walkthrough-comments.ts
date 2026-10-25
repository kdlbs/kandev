import type { Comment, WalkthroughComment } from "@/lib/state/slices/comments";
import type { TaskWalkthrough, WalkthroughStep } from "@/lib/types/http";

function matchesStep(comment: WalkthroughComment, step: WalkthroughStep): boolean {
  return (
    comment.repo === step.repo &&
    comment.filePath === step.file &&
    comment.startLine === Math.min(step.line, step.line_end ?? step.line) &&
    comment.endLine === Math.max(step.line, step.line_end ?? step.line) &&
    comment.stepText === step.text
  );
}

export function getWalkthroughComments(
  comments: Comment[],
  walkthrough: TaskWalkthrough,
  sessionId: string,
  stepIndex: number,
): WalkthroughComment[] {
  const step = walkthrough.steps[stepIndex];
  if (!step) return [];
  return comments
    .filter((comment): comment is WalkthroughComment => {
      if (comment.source !== "walkthrough") return false;
      const sameContext =
        comment.sessionId === sessionId &&
        comment.taskId === walkthrough.task_id &&
        comment.walkthroughId === walkthrough.id &&
        comment.stepIndex === stepIndex;
      const sameRevision =
        comment.walkthroughUpdatedAt === undefined ||
        comment.walkthroughUpdatedAt === walkthrough.updated_at;
      return sameContext && sameRevision && matchesStep(comment, step);
    })
    .sort((a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id));
}
