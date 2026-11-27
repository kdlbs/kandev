"use client";

import type { Dispatch, RefObject, SetStateAction } from "react";
import { createPortal } from "react-dom";
import { IconMessage } from "@tabler/icons-react";
import { SelectionCommentSurface } from "@/components/shared/selection-comment/selection-comment-surface";
import { floatingBounds, placeFloatingRect } from "@/components/task/floating-selection-position";
import type { MessageAttachment } from "@/lib/services/session-launch-service";
import type { AgentMessageComment } from "@/lib/state/slices/comments";
import type { MessageSelection } from "@/lib/chat/agent-message-comments";
import { cn } from "@/lib/utils";
import {
  SelectionActionDock,
  SelectionActionDockButton,
} from "@/components/shared/selection-action-dock";
import { useSelectionActionSlot } from "@/components/shared/selection-action-slot";
import { useTranslation } from "react-i18next";

export type CommentTarget = {
  selection: MessageSelection;
  anchor: AgentMessageComment["anchor"];
  position: { x: number; y: number };
  editingCommentId?: string;
  editingText?: string;
  attachments?: MessageAttachment[];
};

type SaveMessageComment = (feedback: string, attachments?: MessageAttachment[]) => boolean | void;

export type MessageCommentActions = {
  handleAdd: SaveMessageComment;
  handleRun: SaveMessageComment;
  handleDelete?: () => void;
  saveError: string | null;
};

function SelectionCommentTrigger({
  selection,
  onOpen,
  portalContainer,
}: {
  selection: MessageSelection;
  onOpen: () => void;
  portalContainer?: HTMLElement | null;
}) {
  const { t } = useTranslation();
  const size = 28;
  const portalRect = portalContainer?.getBoundingClientRect();
  const outerSize = size + 8;
  const position = placeFloatingRect({
    left: selection.rect.right - outerSize,
    topCandidates: [selection.rect.top - outerSize - 8, selection.rect.bottom + 8],
    width: outerSize,
    height: outerSize,
    bounds: floatingBounds(portalRect),
  });
  return createPortal(
    <div
      className={cn(
        "pointer-events-auto z-40 rounded-lg border border-border/50 bg-popover p-1 shadow-lg",
        portalContainer ? "absolute" : "fixed",
      )}
      style={{
        left: position.left - (portalRect?.left ?? 0),
        top: position.top - (portalRect?.top ?? 0),
      }}
    >
      <button
        type="button"
        title={t("task:commentCmdShiftC")}
        aria-label={t("task:commentOnSelection")}
        data-testid="agent-message-comment-trigger"
        className="flex cursor-pointer items-center justify-center rounded bg-primary text-primary-foreground transition-transform duration-150 ease-out hover:bg-primary/90 active:scale-[0.96]"
        style={{ width: size, height: size }}
        onMouseDown={(event) => event.preventDefault()}
        onClick={onOpen}
      >
        <IconMessage className="h-3.5 w-3.5" />
      </button>
    </div>,
    portalContainer ?? document.body,
  );
}

function DockedCommentTrigger({
  onOpen,
  hostRef,
  portalContainer,
}: {
  onOpen: () => void;
  hostRef?: RefObject<HTMLElement | null>;
  portalContainer?: HTMLElement | null;
}) {
  const { t } = useTranslation();
  const slot = useSelectionActionSlot();
  const button = (
    <SelectionActionDockButton
      icon={IconMessage}
      label={t("task:commentOnSelection")}
      onActivate={onOpen}
      testId="agent-message-comment-trigger"
    />
  );
  if (slot) return createPortal(button, slot);
  return (
    <SelectionActionDock
      hostRef={hostRef}
      portalContainer={portalContainer}
      label={t("task:selectionActions")}
      testId="agent-message-comment-dock"
    >
      {button}
    </SelectionActionDock>
  );
}

export function MessageCommentControls({
  target,
  taskId,
  composerOpen,
  useDrawer,
  setComposerOpen,
  close,
  actions,
  handleRun,
  portalContainer,
  dockHostRef,
}: {
  target: CommentTarget | null;
  taskId: string | null;
  composerOpen: boolean;
  useDrawer: boolean;
  setComposerOpen: Dispatch<SetStateAction<boolean>>;
  close: () => void;
  actions: MessageCommentActions;
  handleRun: SaveMessageComment | undefined;
  portalContainer: HTMLElement | null;
  dockHostRef?: RefObject<HTMLElement | null>;
}) {
  const { t } = useTranslation();
  return (
    <>
      {target && !composerOpen && useDrawer ? (
        <DockedCommentTrigger
          onOpen={() => setComposerOpen(true)}
          hostRef={dockHostRef}
          portalContainer={portalContainer}
        />
      ) : null}
      {target && !composerOpen && !useDrawer ? (
        <SelectionCommentTrigger
          selection={target.selection}
          onOpen={() => setComposerOpen(true)}
          portalContainer={portalContainer}
        />
      ) : null}
      {target && composerOpen ? (
        <SelectionCommentSurface
          key={target.editingCommentId ?? `${target.selection.start}:${target.selection.end}`}
          selectedText={target.selection.selectedText}
          position={target.position}
          attachmentOptions={taskId ? { taskId, initial: target.attachments } : undefined}
          onAdd={(text, _, attachments) => actions.handleAdd(text, attachments)}
          onAddAndRun={
            target.editingCommentId || !handleRun
              ? undefined
              : (text, _, attachments) => handleRun(text, attachments)
          }
          onClose={close}
          editingComment={target.editingText}
          onDelete={actions.handleDelete}
          testId="agent-message-comment-popover"
          drawerTestId="agent-message-comment-drawer"
          inputLabel={t("task:commentOnAgentResponse")}
          inputTestId="agent-message-comment-input"
          addButtonTestId="agent-message-comment-add"
          runButtonTestId="agent-message-comment-run"
          portalContainer={portalContainer}
          errorMessage={actions.saveError}
        />
      ) : null}
    </>
  );
}
