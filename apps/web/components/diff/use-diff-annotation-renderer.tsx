import type { MessageAttachment } from "@/lib/services/session-launch-service";
import { useCallback, type ReactNode } from "react";
import type { DiffLineAnnotation } from "@pierre/diffs";
import type { DiffComment } from "@/lib/diff/types";
import type { TaskReviewFinding } from "@/lib/types/review";
import { useOptionalAppStore } from "@/components/state-provider";
import { CommentForm } from "./comment-form";
import { CommentDisplay } from "./comment-display";
import { HunkActionBar } from "./hunk-action-bar";
import { WalkthroughStepCard } from "./walkthrough-step-card";
import { InlineReviewFinding } from "./inline-review-finding";

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

type UseAnnotationRendererOpts = {
  handleRevertBlock: (changeBlockId: string) => Promise<void>;
  onButtonEnter: () => void;
  onButtonLeave: () => void;
  handleCommentSubmit: (content: string, attachments?: MessageAttachment[]) => void;
  handleCommentSubmitAndRun?: (content: string, attachments?: MessageAttachment[]) => void;
  submitDisabled: boolean;
  sessionId?: string;
  draftKey?: string;
  handleCommentUpdate: (
    commentId: string,
    content: string,
    attachments?: MessageAttachment[],
  ) => void;
  handleCommentDelete: (commentId: string) => void;
  handleCommentRun?: (comment: DiffComment) => void;
  setShowCommentForm: (show: boolean) => void;
  setSelectedLines: (lines: null) => void;
  setEditingComment: (id: string | null) => void;
};

export type { AnnotationMetadata };

function EditAnnotationForm({
  taskId,
  comment,
  onUpdate,
  onCancel,
}: {
  taskId: string | null;
  comment: DiffComment;
  onUpdate: UseAnnotationRendererOpts["handleCommentUpdate"];
  onCancel: () => void;
}) {
  return (
    <div className="my-1 px-2">
      <CommentForm
        initialContent={comment.text}
        attachmentOptions={taskId ? { taskId, initial: comment.attachments } : undefined}
        onSubmit={(text, attachments) => onUpdate(comment.id, text, attachments)}
        onCancel={onCancel}
        isEditing
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
    submitDisabled,
    sessionId,
    draftKey,
    handleCommentUpdate,
    handleCommentDelete,
    handleCommentRun,
    setShowCommentForm,
    setSelectedLines,
    setEditingComment,
  } = opts;

  const taskId = useOptionalAppStore((state) => state.tasks.activeTaskId, null);
  const attachmentTask = sessionId ? taskId : null;
  return useCallback(
    (annotation: DiffLineAnnotation<AnnotationMetadata>): ReactNode => {
      const { type, comment, isEditing, changeBlockId, finding, findingStaleReason } =
        annotation.metadata;

      if (type === "review-finding" && finding) {
        return <InlineReviewFinding finding={finding} staleReason={findingStaleReason} />;
      }

      if (type === "walkthrough-step") return <WalkthroughStepCard key="walkthrough-step" />;

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
              key={`${attachmentTask}:${draftKey}`}
              attachmentOptions={attachmentTask ? { taskId: attachmentTask } : undefined}
              onSubmit={handleCommentSubmit}
              onSubmitAndRun={handleCommentSubmitAndRun}
              submitDisabled={submitDisabled}
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
          <EditAnnotationForm
            key={`${attachmentTask}:${comment.id}`}
            taskId={attachmentTask}
            comment={comment}
            onUpdate={handleCommentUpdate}
            onCancel={() => setEditingComment(null)}
          />
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
      attachmentTask,
      draftKey,
      setEditingComment,
      handleCommentDelete,
      handleCommentRun,
      handleCommentUpdate,
      handleCommentSubmit,
      handleCommentSubmitAndRun,
      submitDisabled,
      handleRevertBlock,
      onButtonEnter,
      onButtonLeave,
      setShowCommentForm,
      setSelectedLines,
    ],
  );
}
