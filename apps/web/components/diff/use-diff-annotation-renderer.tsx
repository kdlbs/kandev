import { useCallback, type ReactNode } from "react";
import type { DiffLineAnnotation } from "@pierre/diffs";
import type { DiffComment } from "@/lib/diff/types";
import { CommentForm } from "./comment-form";
import { CommentDisplay } from "./comment-display";
import { HunkActionBar } from "./hunk-action-bar";

type AnnotationMetadata = {
  type: "comment" | "new-comment-form" | "hunk-actions";
  comment?: DiffComment;
  isEditing?: boolean;
  changeBlockId?: string;
};

type UseAnnotationRendererOpts = {
  handleRevertBlock: (changeBlockId: string) => Promise<void>;
  onButtonEnter: () => void;
  onButtonLeave: () => void;
  handleCommentSubmit: (content: string) => void;
  handleCommentUpdate: (commentId: string, content: string) => void;
  handleCommentDelete: (commentId: string) => void;
  setShowCommentForm: (show: boolean) => void;
  setSelectedLines: (lines: null) => void;
  setEditingComment: (id: string | null) => void;
};

export type { AnnotationMetadata };

export function useAnnotationRenderer(opts: UseAnnotationRendererOpts) {
  const {
    handleRevertBlock,
    onButtonEnter,
    onButtonLeave,
    handleCommentSubmit,
    handleCommentUpdate,
    handleCommentDelete,
    setShowCommentForm,
    setSelectedLines,
    setEditingComment,
  } = opts;

  return useCallback(
    (annotation: DiffLineAnnotation<AnnotationMetadata>): ReactNode => {
      const { type, comment, isEditing, changeBlockId } = annotation.metadata;

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
      handleCommentUpdate,
      handleCommentSubmit,
      handleRevertBlock,
      onButtonEnter,
      onButtonLeave,
      setShowCommentForm,
      setSelectedLines,
    ],
  );
}
