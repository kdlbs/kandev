"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { Textarea } from "@kandev/ui/textarea";
import { useTranslation } from "react-i18next";
import {
  CommentMutationMessages,
  PopoverActions,
  isCommentMutationPending,
  useCommentDelete,
  usePopoverComposer,
  useSurfaceComment,
} from "./surface-parts";
import type { SelectionCommentSurfaceProps } from "./surface-types";

export type SelectionCommentComposerProps = Omit<
  SelectionCommentSurfaceProps,
  "position" | "portalContainer"
>;

/** Comment text, submit, and delete state shared by every comment surface. */
export function useCommentComposerState(props: SelectionCommentComposerProps) {
  const { editingComment, draftComment, onDraftChange, onDelete, onClose } = props;
  const [comment, setComment] = useSurfaceComment(editingComment, draftComment, onDraftChange);
  const onAddAndRun = editingComment ? undefined : props.onAddAndRun;
  const { isDeleting, deleteComment } = useCommentDelete(onDelete, onClose);
  const composer = usePopoverComposer(
    comment,
    props.selectedText,
    props.onAdd,
    onClose,
    onAddAndRun,
  );
  return {
    ...composer,
    comment,
    setComment,
    onAddAndRun,
    isDeleting,
    deleteComment,
    isDisabled: composer.isDisabled || isDeleting || Boolean(props.addDisabled),
    mutationPending: isCommentMutationPending(composer.isSubmitting, isDeleting),
  };
}

export type CommentComposerState = ReturnType<typeof useCommentComposerState>;

function ComposerEvidence({
  evidence,
  previewText,
}: {
  evidence?: ReactNode;
  previewText: string;
}) {
  if (evidence) return <div className="mb-2 min-w-0">{evidence}</div>;
  return (
    <p className="mb-2 text-xs text-muted-foreground line-clamp-2 leading-relaxed italic">
      “{previewText}”
    </p>
  );
}

/** Evidence, comment field, messages, and actions; the body of the popover and drawer. */
export function CommentComposerBody({
  props,
  state,
}: {
  props: SelectionCommentComposerProps;
  state: CommentComposerState;
}) {
  const { t } = useTranslation();
  const { deleteComment } = state;
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    textareaRef.current?.focus();
  }, []);
  return (
    <>
      <ComposerEvidence evidence={props.evidence} previewText={state.previewText} />
      <Textarea
        ref={textareaRef}
        value={state.comment}
        onChange={(event) => state.setComment(event.target.value)}
        onKeyDown={state.handleKeyDown}
        disabled={state.isSubmitting || state.isDeleting}
        placeholder={t("task:addYourCommentOrInstruction")}
        className="min-h-[60px] resize-none text-sm border-border/50 focus:border-primary/50"
        data-testid={props.inputTestId}
        aria-label={props.inputLabel}
      />
      <CommentMutationMessages
        errorMessage={props.errorMessage}
        runDisabledReason={props.runDisabledReason}
        canRun={Boolean(state.onAddAndRun)}
        className="mt-2 text-xs"
      />
      <PopoverActions
        isEditing={!!props.editingComment}
        isDisabled={state.isDisabled}
        onSubmit={() => void state.handleSubmit()}
        onSubmitAndRun={state.onAddAndRun ? () => void state.handleSubmitAndRun() : undefined}
        onDelete={deleteComment ? () => void deleteComment() : undefined}
        onCancel={props.onCancel}
        addButtonTestId={props.addButtonTestId}
        runButtonTestId={props.runButtonTestId}
        runDisabledReason={props.runDisabledReason}
        mutationPending={state.mutationPending}
      />
    </>
  );
}
