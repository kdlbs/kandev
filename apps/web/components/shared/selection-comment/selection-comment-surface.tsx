"use client";

import {
  useLayoutEffect,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import { IconPlus, IconTrash, IconGripHorizontal, IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Drawer, DrawerContent } from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";
import { useDrag } from "@/hooks/use-drag-offset";
import { cn } from "@/lib/utils";
import { floatingBounds, placeFloatingRect } from "@/components/task/floating-selection-position";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  CommentMutationMessages,
  SurfaceContextLabel,
  SurfaceDrawerHeader,
  useMutationProtectedClose,
  usePopoverDismiss,
  useViewportResizeRender,
} from "./surface-parts";
import {
  CommentComposerBody,
  useCommentComposerState,
  type CommentComposerState,
} from "./selection-comment-composer";
import { CommentAttachmentControl } from "./comment-attachment-control";
import { useCommentAttachments } from "./use-comment-attachments";
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
      data-selection-comment-drag-handle=""
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

type SurfaceProps = SelectionCommentSurfaceProps & { state: CommentComposerState };

function DesktopSelectionCommentSurface(props: SurfaceProps) {
  const { position, portalContainer, onClose, testId, contextLabel, animated = true } = props;
  const { state } = props;
  const popoverRef = useRef<HTMLDivElement>(null);
  const { offset, onMouseDown } = useDrag();
  const placement = useDesktopPlacement(position, portalContainer, popoverRef);
  usePopoverDismiss(onClose, popoverRef, state.mutationPending);

  return createPortal(
    <div
      ref={popoverRef}
      className={cn(
        "pointer-events-auto z-[60] max-h-[calc(100dvh-1rem)] overflow-y-auto rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm shadow-xl",
        animated && "animate-in fade-in-0 zoom-in-95 duration-150",
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
        <CommentComposerBody props={props} state={state} />
      </div>
    </div>,
    portalContainer ?? document.body,
  );
}

function SelectionCommentDrawer(props: SurfaceProps) {
  const { t } = useTranslation();
  const { selectedText, onClose, editingComment, testId, contextLabel, evidence } = props;
  const { state } = props;
  const { deleteComment } = state;
  const requestClose = useMutationProtectedClose(onClose, state.mutationPending);

  return (
    <Drawer open onOpenChange={(open) => !open && requestClose()}>
      <DrawerContent
        className="z-[60] max-h-[82dvh] pb-[calc(1rem+env(safe-area-inset-bottom))]"
        data-testid={props.drawerTestId ?? testId ?? "plan-comment-drawer"}
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
            value={state.comment}
            onChange={(event) => state.setComment(event.target.value)}
            onPaste={props.attachmentController?.handlePaste}
            onKeyDown={state.handleKeyDown}
            disabled={state.isSubmitting || state.isDeleting}
            placeholder={t("task:addYourCommentOrInstruction")}
            className="mb-3 min-h-20 resize-none text-sm border-border/50 focus:border-primary/50"
            data-testid={props.inputTestId}
            aria-label={props.inputLabel}
            autoFocus
          />
          <CommentMutationMessages
            errorMessage={props.errorMessage}
            runDisabledReason={props.runDisabledReason}
            canRun={Boolean(state.onAddAndRun)}
            className="mb-3 text-xs"
          />
          {wrapAttachmentActions(
            props,
            state.mutationPending,
            <DrawerActions
              isEditing={Boolean(editingComment)}
              disabled={state.isDisabled}
              mutationPending={state.mutationPending}
              onSubmit={() => void state.handleSubmit()}
              onSubmitAndRun={state.onAddAndRun ? () => void state.handleSubmitAndRun() : undefined}
              onDelete={editingComment && deleteComment ? () => void deleteComment() : undefined}
              onCancel={props.onCancel}
              addButtonTestId={props.addButtonTestId}
              runButtonTestId={props.runButtonTestId}
              runDisabledReason={props.runDisabledReason}
            />,
          )}
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function wrapAttachmentActions(
  props: SelectionCommentSurfaceProps,
  pending: boolean,
  actions: import("react").ReactNode,
) {
  if (!props.attachmentController) return actions;
  return (
    <CommentAttachmentControl controller={props.attachmentController} disabled={pending}>
      {actions}
    </CommentAttachmentControl>
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
  onCancel,
}: {
  isEditing: boolean;
  disabled: boolean;
  mutationPending: boolean;
  onSubmit: () => void;
  onSubmitAndRun?: () => void;
  onDelete?: () => void;
  onCancel?: () => void;
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
      <div className="inline-flex items-center gap-2">
        {onCancel && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={onCancel}
            disabled={mutationPending}
            className="min-h-11 cursor-pointer px-4"
          >
            {t("task:close")}
          </Button>
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
    </div>
  );
}

/**
 * The shared form for writing or editing a comment on a selection, region, or
 * capture: a floating, draggable popover with a fine pointer and a bottom
 * Drawer with a coarse pointer.
 */
export function SelectionCommentSurface(props: SelectionCommentSurfaceProps) {
  const touch = useTouchDrawer();
  const { isMobile } = useResponsiveBreakpoint();
  const useDrawer = touch || (isMobile && !!props.attachmentOptions);
  const controller = useCommentAttachments(props.attachmentOptions);
  const attachmentController =
    props.attachmentController ?? (props.attachmentOptions ? controller : undefined);
  const state = useCommentComposerState({ ...props, attachmentController });
  const surface = { ...props, attachmentController, state };
  return useDrawer ? (
    <SelectionCommentDrawer {...surface} />
  ) : (
    <DesktopSelectionCommentSurface {...surface} />
  );
}
