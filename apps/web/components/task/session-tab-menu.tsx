"use client";

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
import {
  isSessionStoppable as isStoppable,
  isSessionDeletable as isDeletable,
  isSessionResumable as isResumable,
} from "@/hooks/domains/session/use-session-actions";
import { ShareDialog } from "@/components/task/share/share-dialog";
import { HandoffContextMenuSub } from "@/components/task/handoff-profile-menu-items";
import { NewSessionDialog, type HandoffPreset } from "@/components/task/new-session-dialog";
import { SessionDeleteWarningLines } from "@/components/task/session-delete-warning";
import { useSessionDeleteWarning } from "@/hooks/use-session-delete-warning";
import type { TaskSessionState } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

/** Lifecycle callbacks the context menu needs from the owning tab. */
export type SessionTabMenuActions = {
  handleSetPrimary: () => void;
  handleStop: () => void;
  handleResume: () => void;
  handleCloseOthers: () => void;
};

export function DeleteSessionDialog({
  open,
  onOpenChange,
  isPrimary,
  sessionCount,
  sessionId,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isPrimary: boolean;
  sessionCount: number;
  sessionId?: string;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  const warning = useSessionDeleteWarning(open, sessionId);
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("task:deleteSession")}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>{t("task:thisWillPermanentlyDeleteTheConversation")}</p>
              <SessionDeleteWarningLines warning={warning} />
              {isPrimary && sessionCount > 1 && (
                <p className="mt-2 font-medium">{t("task:thisIsThePrimarySessionAnother")}</p>
              )}
              {sessionCount === 1 && (
                <p className="mt-2 font-medium">{t("task:thisIsTheOnlySessionFor")}</p>
              )}
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
}: {
  sessionState: TaskSessionState | null;
  isPrimary: boolean;
  canShare: boolean;
  taskId: string | null;
  sessionId: string | undefined;
  actions: SessionTabMenuActions;
  onDelete: () => void;
  onShare: () => void;
  onHandoffProfile: (profileId: string) => void;
  onStartRename: () => void;
}) {
  const { t } = useTranslation();
  return (
    <ContextMenuContent>
      <ContextMenuItem className="cursor-pointer" onSelect={onStartRename}>
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
      {sessionState && isStoppable(sessionState) && (
        <ContextMenuItem className="cursor-pointer" onSelect={actions.handleStop}>
          {t("task:stop")}
        </ContextMenuItem>
      )}
      {sessionState && isResumable(sessionState) && (
        <ContextMenuItem className="cursor-pointer" onSelect={actions.handleResume}>
          {t("task:resume")}
        </ContextMenuItem>
      )}
      {sessionState && isDeletable(sessionState) && (
        <ContextMenuItem className="cursor-pointer text-destructive" onSelect={onDelete}>
          {t("task:delete")}
        </ContextMenuItem>
      )}
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
      <ContextMenuSeparator />
      <ContextMenuItem className="cursor-pointer" onSelect={actions.handleCloseOthers}>
        {t("task:closeOthers")}
      </ContextMenuItem>
    </ContextMenuContent>
  );
}

/** Delete / share / handoff dialogs rendered alongside the tab. */
export function SessionTabDialogs({
  confirmDelete,
  setConfirmDelete,
  isPrimary,
  sessionCount,
  onConfirmDelete,
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
  setConfirmDelete: (open: boolean) => void;
  isPrimary: boolean;
  sessionCount: number;
  onConfirmDelete: () => void;
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
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        isPrimary={isPrimary}
        sessionCount={sessionCount}
        sessionId={sessionId}
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
