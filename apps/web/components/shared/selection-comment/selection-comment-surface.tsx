"use client";

import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import { IconPlus, IconTrash, IconGripHorizontal, IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Drawer, DrawerContent } from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { floatingBounds, placeFloatingRect } from "@/components/task/floating-selection-position";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  CommentMutationMessages,
  PopoverActions,
  SurfaceContextLabel,
  SurfaceDrawerHeader,
  isCommentMutationPending,
  useCommentDelete,
  useDrag,
  useMutationProtectedClose,
  usePopoverComposer,
  usePopoverDismiss,
  useSurfaceComment,
  useViewportResizeRender,
} from "./surface-parts";
import type { SelectionCommentSurfaceProps } from "./surface-types";

export type { SelectionCommentSurfaceProps } from "./surface-types";

/** True for events inside any open surface; hosts use it to stay open while it is in use. */
export function isInsideSelectionCommentSurface(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest("[data-selection-comment-surface]"));
}

const POPOVER_WIDTH = 340;
const POPOVER_HEIGHT = 180;
const MARGIN = 8;

/** The rendered height, so tall evidence such as a thumbnail still fits the viewport. */
function useMeasuredHeight(ref: RefObject<HTMLElement | null>) {
  const [height, setHeight] = useState(POPOVER_HEIGHT);
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () =>
      setHeight(Math.max(POPOVER_HEIGHT, element.getBoundingClientRect().height));
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [ref]);
  return height;
}

function useDesktopPlacement(
  position: SelectionCommentSurfaceProps["position"],
  portalContainer: HTMLElement | null | undefined,
  popoverRef: RefObject<HTMLElement | null>,
) {
  useViewportResizeRender();
  const height = useMeasuredHeight(popoverRef);
  const portalRect = portalContainer?.getBoundingClientRect();
  const { left, top } = placeFloatingRect({
    left: position.x,
    topCandidates: [position.y + 4, position.y - height - 4],
    width: POPOVER_WIDTH,
    height,
    bounds: floatingBounds(portalRect),
    margin: MARGIN,
  });
  return { left: left - (portalRect?.left ?? 0), top: top - (portalRect?.top ?? 0) };
}

function DesktopSurfaceHeader({
  contextLabel,
  onMouseDown,
}: {
  contextLabel?: string;
  onMouseDown: (event: ReactMouseEvent) => void;
}) {
  return (
    <div
      onMouseDown={onMouseDown}
      className={cn(
        "flex items-center gap-2 px-3 py-1.5 cursor-grab active:cursor-grabbing border-b border-border/30",
        contextLabel ? "justify-between" : "justify-center",
      )}
    >
      {contextLabel ? <SurfaceContextLabel label={contextLabel} /> : null}
      <IconGripHorizontal className="h-3.5 w-3.5 shrink-0 text-muted-foreground/50" />
    </div>
  );
}

function DesktopEvidence({ evidence, previewText }: { evidence?: ReactNode; previewText: string }) {
  if (evidence) return <div className="mb-2 min-w-0">{evidence}</div>;
  return (
    <p className="mb-2 text-xs text-muted-foreground line-clamp-2 leading-relaxed italic">
      “{previewText}”
    </p>
  );
}

function DesktopSelectionCommentSurface({
  selectedText,
  position,
  onAdd,
  onAddAndRun,
  onClose,
  editingComment,
  onDelete,
  testId,
  inputTestId,
  addButtonTestId,
  runButtonTestId,
  portalContainer,
  errorMessage,
  runDisabledReason,
  contextLabel,
  evidence,
  draftComment,
  onDraftChange,
  inputLabel,
}: SelectionCommentSurfaceProps) {
  const { t } = useTranslation();
  const [comment, setComment] = useSurfaceComment(editingComment, draftComment, onDraftChange);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);
  const { offset, onMouseDown } = useDrag();
  const placement = useDesktopPlacement(position, portalContainer, popoverRef);
  const { isDeleting, deleteComment } = useCommentDelete(onDelete, onClose);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);
  const effectiveOnAddAndRun = editingComment ? undefined : onAddAndRun;
  const { handleSubmit, handleSubmitAndRun, handleKeyDown, isDisabled, isSubmitting, previewText } =
    usePopoverComposer(comment, selectedText, onAdd, onClose, effectiveOnAddAndRun);
  const mutationPending = isCommentMutationPending(isSubmitting, isDeleting);
  usePopoverDismiss(onClose, popoverRef, mutationPending);

  return createPortal(
    <div
      ref={popoverRef}
      className={cn(
        "pointer-events-auto z-[60] max-h-[calc(100dvh-1rem)] overflow-y-auto rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm shadow-xl",
        "animate-in fade-in-0 zoom-in-95 duration-150",
        portalContainer ? "absolute" : "fixed",
      )}
      data-testid={testId}
      data-selection-comment-surface=""
      style={{
        width: `min(${POPOVER_WIDTH}px, calc(100vw - 2rem))`,
        left: placement.left + offset.dx,
        top: placement.top + offset.dy,
      }}
    >
      <DesktopSurfaceHeader contextLabel={contextLabel} onMouseDown={onMouseDown} />
      <div className="p-3">
        <DesktopEvidence evidence={evidence} previewText={previewText} />
        <Textarea
          ref={textareaRef}
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={isSubmitting || isDeleting}
          placeholder={t("task:addYourCommentOrInstruction")}
          className="min-h-[60px] resize-none text-sm border-border/50 focus:border-primary/50"
          data-testid={inputTestId}
          aria-label={inputLabel}
        />
        <CommentMutationMessages
          errorMessage={errorMessage}
          runDisabledReason={runDisabledReason}
          canRun={Boolean(effectiveOnAddAndRun)}
          className="mt-2 text-xs"
        />
        <PopoverActions
          isEditing={!!editingComment}
          isDisabled={isDisabled || isDeleting}
          onSubmit={() => void handleSubmit()}
          onSubmitAndRun={effectiveOnAddAndRun ? () => void handleSubmitAndRun() : undefined}
          onDelete={deleteComment ? () => void deleteComment() : undefined}
          addButtonTestId={addButtonTestId}
          runButtonTestId={runButtonTestId}
          runDisabledReason={runDisabledReason}
          mutationPending={mutationPending}
        />
      </div>
    </div>,
    portalContainer ?? document.body,
  );
}

function SelectionCommentDrawer({
  selectedText,
  onAdd,
  onAddAndRun,
  onClose,
  editingComment,
  onDelete,
  testId,
  inputTestId,
  addButtonTestId,
  runButtonTestId,
  errorMessage,
  runDisabledReason,
  contextLabel,
  evidence,
  draftComment,
  onDraftChange,
  inputLabel,
}: SelectionCommentSurfaceProps) {
  const { t } = useTranslation();
  const [comment, setComment] = useSurfaceComment(editingComment, draftComment, onDraftChange);
  const effectiveOnAddAndRun = editingComment ? undefined : onAddAndRun;
  const { isDeleting, deleteComment } = useCommentDelete(onDelete, onClose);
  const { handleSubmit, handleSubmitAndRun, handleKeyDown, isDisabled, isSubmitting } =
    usePopoverComposer(comment, selectedText, onAdd, onClose, effectiveOnAddAndRun);
  const disabled = isDisabled || isDeleting;
  const mutationPending = isCommentMutationPending(isSubmitting, isDeleting);
  const requestClose = useMutationProtectedClose(onClose, mutationPending);

  return (
    <Drawer open onOpenChange={(open) => !open && requestClose()}>
      <DrawerContent
        className="z-[60] max-h-[82dvh] pb-[calc(1rem+env(safe-area-inset-bottom))]"
        data-testid={testId ?? "plan-comment-drawer"}
        data-selection-comment-surface=""
      >
        <SurfaceDrawerHeader
          editingComment={editingComment}
          selectedText={selectedText}
          contextLabel={contextLabel}
          evidence={evidence}
        />
        <div className="min-h-0 overflow-y-auto px-4 pb-2">
          <Textarea
            value={comment}
            onChange={(event) => setComment(event.target.value)}
            onKeyDown={handleKeyDown}
            disabled={isSubmitting || isDeleting}
            placeholder={t("task:addYourCommentOrInstruction")}
            className="mb-3 min-h-20 resize-none text-sm border-border/50 focus:border-primary/50"
            data-testid={inputTestId}
            aria-label={inputLabel}
            autoFocus
          />
          <CommentMutationMessages
            errorMessage={errorMessage}
            runDisabledReason={runDisabledReason}
            canRun={Boolean(effectiveOnAddAndRun)}
            className="mb-3 text-xs"
          />
          <DrawerActions
            isEditing={Boolean(editingComment)}
            disabled={disabled}
            mutationPending={mutationPending}
            onSubmit={() => void handleSubmit()}
            onSubmitAndRun={effectiveOnAddAndRun ? () => void handleSubmitAndRun() : undefined}
            onDelete={editingComment && deleteComment ? () => void deleteComment() : undefined}
            addButtonTestId={addButtonTestId}
            runButtonTestId={runButtonTestId}
            runDisabledReason={runDisabledReason}
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function DrawerActions({
  isEditing,
  disabled,
  mutationPending,
  onSubmit,
  onSubmitAndRun,
  onDelete,
  addButtonTestId,
  runButtonTestId,
  runDisabledReason,
}: {
  isEditing: boolean;
  disabled: boolean;
  mutationPending: boolean;
  onSubmit: () => void;
  onSubmitAndRun?: () => void;
  onDelete?: () => void;
  addButtonTestId?: string;
  runButtonTestId?: string;
  runDisabledReason?: string | null;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-3">
      {onDelete ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={onDelete}
          disabled={mutationPending}
          aria-label={t("task:deleteComment")}
          className="min-h-11 min-w-11 cursor-pointer px-3 text-muted-foreground hover:text-destructive"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      ) : (
        <span />
      )}
      <div className="inline-flex">
        <Button
          type="button"
          size="sm"
          variant={onSubmitAndRun ? "outline" : "default"}
          onClick={onSubmit}
          disabled={disabled}
          data-testid={addButtonTestId}
          className={cn(
            "min-h-11 cursor-pointer gap-1.5 px-4 active:scale-[0.96]",
            onSubmitAndRun && "rounded-r-none border-r-0",
          )}
        >
          <IconPlus className="h-4 w-4" />
          {isEditing ? t("task:update") : t("task:add")}
        </Button>
        {onSubmitAndRun ? (
          <Button
            type="button"
            size="sm"
            onClick={onSubmitAndRun}
            disabled={disabled || !!runDisabledReason}
            data-testid={runButtonTestId}
            className="min-h-11 cursor-pointer gap-1.5 rounded-l-none px-4 active:scale-[0.96]"
          >
            <IconPlayerPlay className="h-4 w-4" />
            {t("task:run")}
          </Button>
        ) : null}
      </div>
    </div>
  );
}

/**
 * The shared form for writing or editing a comment on a selection, region, or
 * capture: a floating, draggable popover with a fine pointer and a bottom
 * Drawer with a coarse pointer.
 */
export function SelectionCommentSurface(props: SelectionCommentSurfaceProps) {
  const useDrawer = useTouchDrawer();
  return useDrawer ? (
    <SelectionCommentDrawer {...props} />
  ) : (
    <DesktopSelectionCommentSurface {...props} />
  );
}
