"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";
import type { PendingQuickChatCreation, QuickChatCreationState } from "./quick-chat-creation";
import type { DeleteTaskParams } from "@/lib/api/domains/kanban-api";

const CREATION_STATUS_MESSAGE: Record<QuickChatCreationState, string> = {
  submitting: "chat:quickChatCreationUnknown",
  unknown: "chat:quickChatCreationUnknown",
  incomplete: "chat:quickChatCreationIncomplete",
  accepted: "chat:quickChatCreationAccepted",
};

export function QuickChatCreationStatus({
  pending,
  checking,
  onCheck,
  onRetry,
  onDelete,
}: {
  pending: PendingQuickChatCreation | null;
  checking: boolean;
  onCheck: () => void;
  onRetry: () => void;
  onDelete: (options: DeleteTaskParams) => void;
}) {
  const { t } = useTranslation();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const usesTouchDrawer = useTouchDrawer();
  const isMobile = useResponsiveBreakpoint().isMobile;
  const isTouch = usesTouchDrawer || isMobile;
  if (!pending) return null;
  const accepted = pending.state === "accepted";
  const actionsDisabled = checking || pending.state === "submitting";

  return (
    <>
      <div
        className="space-y-2 rounded-md border border-muted bg-muted/30 p-3 text-sm"
        data-testid="quick-chat-creation-status"
        role="status"
        aria-live="polite"
      >
        <p>{t(CREATION_STATUS_MESSAGE[pending.state])}</p>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={actionsDisabled}
            className={isTouch ? "min-h-11" : undefined}
            data-testid="quick-chat-creation-check"
            onClick={onCheck}
          >
            {checking ? <IconLoader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden /> : null}
            {t(accepted ? "chat:openQuickChatCreation" : "chat:checkQuickChatCreation")}
          </Button>
          {!accepted ? (
            <Button
              type="button"
              variant="outline"
              disabled={actionsDisabled}
              className={isTouch ? "min-h-11" : undefined}
              data-testid="quick-chat-creation-retry"
              onClick={onRetry}
            >
              {t("chat:retryQuickChatCreation")}
            </Button>
          ) : null}
          {pending.taskId ? (
            <Button
              type="button"
              variant="ghost"
              disabled={actionsDisabled}
              className={isTouch ? "min-h-11" : undefined}
              data-testid="quick-chat-creation-delete"
              onClick={() => setDeleteOpen(true)}
            >
              {t("chat:deleteQuickChatCreation")}
            </Button>
          ) : null}
        </div>
      </div>
      <QuickChatDeleteDialog
        sessionToDelete={deleteOpen ? (pending.taskId ?? null) : null}
        taskId={pending.taskId ?? undefined}
        onOpenChange={setDeleteOpen}
        onConfirm={(options) => {
          setDeleteOpen(false);
          onDelete(options);
        }}
      />
    </>
  );
}
