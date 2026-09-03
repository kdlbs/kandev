import { useCallback, useEffect, useRef, useState } from "react";
import type { TextSelection } from "@/components/editors/tiptap/tiptap-plan-editor";
import type { usePlanComments } from "@/hooks/domains/comments/use-plan-comments";

type PlanSelectionCommentState = Pick<
  ReturnType<typeof usePlanComments>,
  "comments" | "setEditingCommentId"
>;

/** Own transient plan-comment selection for the currently open task. */
export function usePlanSelection(
  taskId: string | null | undefined,
  commentState: PlanSelectionCommentState,
) {
  const [textSelection, setTextSelection] = useState<TextSelection | null>(null);
  const previousTaskIdRef = useRef(taskId);

  useEffect(() => {
    if (previousTaskIdRef.current === taskId) return;
    previousTaskIdRef.current = taskId;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- task identity owns this transient editor state
    setTextSelection(null);
    commentState.setEditingCommentId(null);
    window.getSelection()?.removeAllRanges();
  }, [taskId, commentState.setEditingCommentId]);

  const handleCommentHighlightClick = useCallback(
    (id: string, position: { x: number; y: number }) => {
      const comment = commentState.comments.find((candidate) => candidate.id === id);
      if (!comment) return;
      commentState.setEditingCommentId(id);
      setTextSelection({
        text: comment.selectedText,
        from: comment.from,
        to: comment.to,
        position,
      });
    },
    [commentState],
  );

  const handleSelectionClose = useCallback(() => {
    setTextSelection(null);
    commentState.setEditingCommentId(null);
    window.getSelection()?.removeAllRanges();
  }, [commentState]);

  return { textSelection, setTextSelection, handleCommentHighlightClick, handleSelectionClose };
}
