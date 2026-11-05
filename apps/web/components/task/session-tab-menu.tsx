"use client";

import { useRef, type RefObject } from "react";
import { ContextMenuContent, ContextMenuItem, ContextMenuSeparator } from "@kandev/ui/context-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import {
  isSessionStoppable as isStoppable,
  isSessionDeletable as isDeletable,
  isSessionResumable as isResumable,
} from "@/hooks/domains/session/use-session-actions";
import { ShareDialog } from "@/components/task/share/share-dialog";
import { HandoffContextMenuSub } from "@/components/task/handoff-profile-menu-items";
import { NewSessionDialog, type HandoffPreset } from "@/components/task/new-session-dialog";
import type { TaskSessionState } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { SessionDeleteDescription } from "./session-delete-description";

/** Lifecycle callbacks the context menu needs from the owning tab.
 * `handleCloseOthers` is dockview-specific (closing sibling panels) and has
 * no equivalent where tabs are a plain session switcher, so it's optional —
 * omitting it hides the Close Others item entirely. */
export type SessionTabMenuActions = {
  handleSetPrimary: () => void;
  handleStop: () => void;
  handleResume: () => void;
  handleRestart: () => void;
  isRestarting?: boolean;
  handleCloseOthers?: () => void;
};

export function DeleteSessionDialog({
  open,
  onOpenChange,
  isPrimary,
  sessionCount,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isPrimary: boolean;
  sessionCount: number;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("task:deleteSession")}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="min-w-0 space-y-2 text-left">
              <SessionDeleteDescription
                isPrimary={isPrimary}
                isOnlySession={sessionCount === 1}
                structured
              />
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              onOpenChange(false);
              onConfirm();
            }}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {t("task:delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function DeleteSessionPopover({
  open,
  anchorRef,
  focusBoundaryRef,
  onOpenChange,
  isPrimary,
  sessionCount,
  targetName,
  onConfirm,
}: {
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  isPrimary: boolean;
  sessionCount: number;
  targetName?: string | null;
  onConfirm: () => void | Promise<void>;
}) {
  const { t } = useTranslation();
  const name = targetName || t("task:session");
  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      focusBoundaryRef={focusBoundaryRef}
      title={t("task:deleteSession")}
      description={
        <SessionDeleteDescription isPrimary={isPrimary} isOnlySession={sessionCount === 1} />
      }
      cancelLabel={t("common:cancel")}
      confirmLabel={t("task:delete")}
      confirmAriaLabel={t("task:delete2", { name })}
      confirmTestId="session-delete-confirm"
      testId="session-delete-confirm-popover"
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
    />
  );
}

/** Keeps restart visible while pending and disables competing actions for the same target. */
function SessionLifecycleMenuItems({
  sessionState,
  actions,
  onDelete,
}: {
  sessionState: TaskSessionState | null;
  actions: SessionTabMenuActions;
  onDelete: (event: Event) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {sessionState && isStoppable(sessionState) && (
        <ContextMenuItem
          className="cursor-pointer"
          onSelect={actions.handleStop}
          disabled={actions.isRestarting}
        >
          {t("task:stop")}
        </ContextMenuItem>
      )}
      {(actions.isRestarting || (sessionState && isStoppable(sessionState))) && (
        <ContextMenuItem
          className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onSelect={actions.handleRestart}
          disabled={actions.isRestarting}
        >
          {t("task:restartSession")}
        </ContextMenuItem>
      )}
      {sessionState && isResumable(sessionState) && (
        <ContextMenuItem
          className="cursor-pointer"
          onSelect={actions.handleResume}
          disabled={actions.isRestarting}
        >
          {t("task:resume")}
        </ContextMenuItem>
      )}
      {sessionState && isDeletable(sessionState) && (
        <ContextMenuItem
          className="cursor-pointer text-destructive"
          disabled={actions.isRestarting}
          onSelect={(event) => {
            // Keep the context menu mounted so the confirmation popover can stay anchored here.
            event.preventDefault();
            onDelete(event);
          }}
        >
          {t("task:delete")}
        </ContextMenuItem>
      )}
    </>
  );
}

/** Composes session actions while deferring rename focus until the context menu closes. */
export function SessionContextMenuItems({
  sessionState,
  isPrimary,
  canShare,
  taskId,
  sessionId,
  actions,
  onDelete,
  onShare,
  onHandoffProfile,
  onStartRename,
  onExecution,
}: {
  sessionState: TaskSessionState | null;
  isPrimary: boolean;
  canShare: boolean;
  taskId: string | null;
  sessionId: string | undefined;
  actions: SessionTabMenuActions;
  /** Passes the Radix selection event so the owner can capture its currentTarget as an anchor. */
  onDelete: (event: Event) => void;
  onShare: () => void;
  onHandoffProfile: (profileId: string) => void;
  onStartRename: () => void;
  onExecution?: () => void;
}) {
  const { t } = useTranslation();
  const renameAfterCloseRef = useRef(false);
  const executionAfterCloseRef = useRef(false);
  return (
    <ContextMenuContent
      onCloseAutoFocus={(event) => {
        if (executionAfterCloseRef.current && onExecution) {
          executionAfterCloseRef.current = false;
          event.preventDefault();
          queueMicrotask(onExecution);
          return;
        }
        if (!renameAfterCloseRef.current) return;
        renameAfterCloseRef.current = false;
        event.preventDefault();
        queueMicrotask(onStartRename);
      }}
    >
      {onExecution && (
        <ContextMenuItem
          onSelect={() => {
            executionAfterCloseRef.current = true;
          }}
        >
          {t("agents:executionDetails")}
        </ContextMenuItem>
      )}
      <ContextMenuItem
        className="cursor-pointer"
        onSelect={() => {
          renameAfterCloseRef.current = true;
        }}
      >
        {t("task:rename2")}
      </ContextMenuItem>
      <ContextMenuItem
        className="cursor-pointer"
        onSelect={actions.handleSetPrimary}
        disabled={isPrimary || !sessionState || !isStoppable(sessionState)}
      >
        {t("task:setAsPrimary")}
      </ContextMenuItem>
      <ContextMenuSeparator />
      <SessionLifecycleMenuItems
        sessionState={sessionState}
        actions={actions}
        onDelete={onDelete}
      />
      {canShare && (
        <>
          <ContextMenuSeparator />
          <ContextMenuItem className="cursor-pointer" onSelect={onShare}>
            {t("task:share")}
          </ContextMenuItem>
        </>
      )}
      {taskId && sessionId && (
        <>
          <ContextMenuSeparator />
          <HandoffContextMenuSub taskId={taskId} onSelectProfile={onHandoffProfile} />
        </>
      )}
      {actions.handleCloseOthers && (
        <>
          <ContextMenuSeparator />
          <ContextMenuItem className="cursor-pointer" onSelect={actions.handleCloseOthers}>
            {t("task:closeOthers")}
          </ContextMenuItem>
        </>
      )}
    </ContextMenuContent>
  );
}

/** Delete / share / handoff dialogs rendered alongside the tab. */
export function SessionTabDialogs({
  confirmDelete,
  menuDeleteOpen,
  menuDeleteAnchorRef,
  menuDeleteFocusBoundaryRef,
  setConfirmDelete,
  isPrimary,
  sessionCount,
  onConfirmDelete,
  targetName,
  taskId,
  sessionId,
  shareOpen,
  setShareOpen,
  handoffOpen,
  setHandoffOpen,
  handoffPreset,
  setHandoffPreset,
  groupId,
}: {
  confirmDelete: boolean;
  menuDeleteOpen: boolean;
  menuDeleteAnchorRef: RefObject<HTMLElement | null>;
  menuDeleteFocusBoundaryRef?: RefObject<HTMLElement | null>;
  setConfirmDelete: (open: boolean) => void;
  isPrimary: boolean;
  sessionCount: number;
  onConfirmDelete: () => void;
  targetName?: string | null;
  taskId: string | null;
  sessionId: string | undefined;
  shareOpen: boolean;
  setShareOpen: (open: boolean) => void;
  handoffOpen: boolean;
  setHandoffOpen: (open: boolean) => void;
  handoffPreset: HandoffPreset | null;
  setHandoffPreset: (preset: HandoffPreset | null) => void;
  groupId: string | undefined;
}) {
  return (
    <>
      <DeleteSessionDialog
        open={confirmDelete && !menuDeleteOpen}
        onOpenChange={setConfirmDelete}
        isPrimary={isPrimary}
        sessionCount={sessionCount}
        onConfirm={onConfirmDelete}
      />
      <DeleteSessionPopover
        open={menuDeleteOpen}
        anchorRef={menuDeleteAnchorRef}
        focusBoundaryRef={menuDeleteFocusBoundaryRef}
        onOpenChange={setConfirmDelete}
        isPrimary={isPrimary}
        sessionCount={sessionCount}
        targetName={targetName}
        onConfirm={onConfirmDelete}
      />
      {taskId && sessionId && (
        <ShareDialog
          open={shareOpen}
          onOpenChange={setShareOpen}
          taskId={taskId}
          sessionId={sessionId}
        />
      )}
      {taskId && handoffPreset && (
        <NewSessionDialog
          open={handoffOpen}
          onOpenChange={(open) => {
            setHandoffOpen(open);
            if (!open) setHandoffPreset(null);
          }}
          taskId={taskId}
          groupId={groupId}
          handoff={handoffPreset}
        />
      )}
    </>
  );
}
