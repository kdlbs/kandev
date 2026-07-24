import { useCallback, type ReactNode } from "react";
import type { DiffLineAnnotation } from "@pierre/diffs";
import type { DiffComment } from "@/lib/diff/types";
import type { TaskReviewFinding } from "@/lib/types/review";
import { CommentForm } from "./comment-form";
import { CommentDisplay } from "./comment-display";
import { HunkActionBar } from "./hunk-action-bar";
import { WalkthroughStepCard } from "./walkthrough-step-card";
import { ReviewFindingCard } from "./review-finding-card";

type AnnotationMetadata = {
  type: "comment" | "new-comment-form" | "hunk-actions" | "walkthrough-step" | "review-finding";
  comment?: DiffComment;
  isEditing?: boolean;
  changeBlockId?: string;
  /** Set when type is "review-finding". */
  finding?: TaskReviewFinding;
  /** Explains why an anchored finding is stale, when it is. */
  findingStaleReason?: string;
};

type ReviewFindingHandlers = {
  onResolve?: (finding: TaskReviewFinding) => void;
  onDismiss?: (finding: TaskReviewFinding) => void;
  onReopen?: (finding: TaskReviewFinding) => void;
  onSendToAgent?: (finding: TaskReviewFinding) => void;
};

type UseAnnotationRendererOpts = {
  handleRevertBlock: (changeBlockId: string) => Promise<void>;
  /** Actions for inline review findings; omitted when findings are read-only. */
  reviewFindingHandlers?: ReviewFindingHandlers;
  onButtonEnter: () => void;
  onButtonLeave: () => void;
  handleCommentSubmit: (content: string) => void;
  handleCommentSubmitAndRun?: (content: string) => void;
  handleCommentUpdate: (commentId: string, content: string) => void;
  handleCommentDelete: (commentId: string) => void;
  handleCommentRun?: (comment: DiffComment) => void;
  setShowCommentForm: (show: boolean) => void;
  setSelectedLines: (lines: null) => void;
  setEditingComment: (id: string | null) => void;
};

export type { AnnotationMetadata };

/** One review finding rendered inline at its anchored diff line. */
function InlineReviewFinding({
  finding,
  staleReason,
  handlers,
}: {
  finding: TaskReviewFinding;
  staleReason?: string;
  handlers?: ReviewFindingHandlers;
}) {
  return (
    <div className="my-1 px-2">
      <ReviewFindingCard
        finding={finding}
        staleReason={staleReason}
        onResolve={handlers?.onResolve}
        onDismiss={handlers?.onDismiss}
        onReopen={handlers?.onReopen}
        onSendToAgent={handlers?.onSendToAgent}
      />
    </div>
  );
}

export function useAnnotationRenderer(opts: UseAnnotationRendererOpts) {
  const {
    handleRevertBlock,
    onButtonEnter,
    onButtonLeave,
    handleCommentSubmit,
    handleCommentSubmitAndRun,
    handleCommentUpdate,
    handleCommentDelete,
    handleCommentRun,
    setShowCommentForm,
    setSelectedLines,
    setEditingComment,
    reviewFindingHandlers,
  } = opts;

  return useCallback(
    (annotation: DiffLineAnnotation<AnnotationMetadata>): ReactNode => {
      const { type, comment, isEditing, changeBlockId, finding, findingStaleReason } =
        annotation.metadata;

      if (type === "review-finding" && finding) {
        return (
          <InlineReviewFinding
            finding={finding}
            staleReason={findingStaleReason}
            handlers={reviewFindingHandlers}
          />
        );
      }

      if (type === "walkthrough-step") {
        return <WalkthroughStepCard key="walkthrough-step" />;
      }

      if (type === "hunk-actions" && changeBlockId) {
        return (
          <HunkActionBar
            key={changeBlockId}
            changeBlockId={changeBlockId}
            onRevert={() => handleRevertBlock(changeBlockId)}
            onMouseEnter={onButtonEnter}
            onMouseLeave={onButtonLeave}
          />
        );
      }

      if (type === "new-comment-form") {
        return (
          <div className="my-1 px-2">
            <CommentForm
              onSubmit={handleCommentSubmit}
              onSubmitAndRun={handleCommentSubmitAndRun}
              onCancel={() => {
                setShowCommentForm(false);
                setSelectedLines(null);
              }}
            />
          </div>
        );
      }

      if (isEditing && comment) {
        return (
          <div className="my-1 px-2">
            <CommentForm
              initialContent={comment.text}
              onSubmit={(content) => handleCommentUpdate(comment.id, content)}
              onCancel={() => setEditingComment(null)}
              isEditing
            />
          </div>
        );
      }

      if (comment) {
        return (
          <div className="my-1 px-2">
            <CommentDisplay
              comment={comment}
              onDelete={() => handleCommentDelete(comment.id)}
              onEdit={() => setEditingComment(comment.id)}
              onRun={handleCommentRun ? () => handleCommentRun(comment) : undefined}
              showCode={false}
            />
          </div>
        );
      }

      return null;
    },
    [
      setEditingComment,
      handleCommentDelete,
      handleCommentRun,
      handleCommentUpdate,
      handleCommentSubmit,
      handleCommentSubmitAndRun,
      handleRevertBlock,
      onButtonEnter,
      onButtonLeave,
      setShowCommentForm,
      setSelectedLines,
      reviewFindingHandlers,
    ],
  );
}
