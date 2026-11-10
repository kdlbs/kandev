"use client";

import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { TaskCreateRecoveryView } from "@/components/task-create-dialog-recovery";
import type { TaskCreationReceipt } from "@/lib/task-creation-receipt";

const STATUS_KEYS = {
  unknown: {
    title: "task:taskCreationUnconfirmedTitle",
    description: "task:taskCreationUnconfirmedDescription",
  },
  submitting: {
    title: "task:taskCreationUnconfirmedTitle",
    description: "task:taskCreationUnconfirmedDescription",
  },
  incomplete: {
    title: "task:taskCreationIncompleteTitle",
    description: "task:taskCreationIncompleteDescription",
  },
  accepted: {
    title: "task:taskCreationAcceptedTitle",
    description: "task:taskCreationAcceptedDescription",
  },
  unavailable: {
    title: "task:taskCreationUnavailableTitle",
    description: "task:taskCreationUnavailableDescription",
  },
} as const;

function statusKeys(receipt: TaskCreationReceipt) {
  return STATUS_KEYS[receipt.state];
}

function RecoveryNotice({
  receipt,
  storageUnavailable,
}: {
  receipt: TaskCreationReceipt;
  storageUnavailable: boolean;
}) {
  const { t } = useTranslation();
  const keys = statusKeys(receipt);
  return (
    <div
      role="status"
      aria-live="polite"
      data-testid="task-create-recovery-status"
      className="rounded-md border border-border bg-muted/40 px-3 py-2 text-sm"
    >
      <p className="font-medium text-foreground">{t(keys.title)}</p>
      <p className="text-muted-foreground">{t(keys.description)}</p>
      {receipt.state === "unknown" && !receipt.replayable ? (
        <p className="text-muted-foreground">{t("task:taskCreationRetryUnavailable")}</p>
      ) : null}
      {storageUnavailable ? (
        <p className="text-muted-foreground">{t("task:taskCreationStorageUnavailable")}</p>
      ) : null}
    </div>
  );
}

/**
 * Inline status above the fixed footer. State and action order come from the
 * system design's recovery-surface table; the full draft stays editable.
 */
export function TaskCreateDialogRecoveryPanel({ recovery }: { recovery: TaskCreateRecoveryView }) {
  if (!recovery.receipt) return null;
  return (
    <RecoveryNotice receipt={recovery.receipt} storageUnavailable={recovery.storageUnavailable} />
  );
}

const ACTION_CLASS =
  "h-11 w-full cursor-pointer sm:[@media(pointer:fine)]:h-7 sm:[@media(pointer:fine)]:w-auto";

/**
 * Replaces the ordinary submit footer while a native submission is
 * unresolved. Phone actions stack full-width with 44px targets; desktop keeps
 * the normal 28px control size.
 */
function resolvePrimaryAction(hasOpen: boolean): "check" | "open" {
  return hasOpen ? "open" : "check";
}

export function TaskCreateDialogRecoveryFooter({ recovery }: { recovery: TaskCreateRecoveryView }) {
  const { t } = useTranslation();
  if (!recovery.receipt) return null;
  const showOpen = recovery.canOpen;
  const showRetry = recovery.canRetry;
  const showCheck = recovery.canCheck;
  const primary = resolvePrimaryAction(showOpen);

  return (
    <div className="flex w-full flex-col-reverse gap-2 sm:flex-row sm:justify-end sm:gap-2">
      <Button
        type="button"
        variant="outline"
        className={ACTION_CLASS}
        onClick={recovery.close}
        data-testid="task-create-recovery-close"
      >
        {t("common:close")}
      </Button>
      {showRetry ? (
        <Button
          type="button"
          variant="outline"
          className={ACTION_CLASS}
          onClick={() => void recovery.retry()}
          disabled={recovery.busy}
          data-testid="task-create-recovery-retry"
        >
          {t("task:taskCreationRetrySubmission")}
        </Button>
      ) : null}
      {showCheck ? (
        <Button
          type="button"
          variant={primary === "check" ? "default" : "outline"}
          className={ACTION_CLASS}
          onClick={() => void recovery.checkStatus()}
          disabled={recovery.busy}
          data-testid="task-create-recovery-check"
        >
          {t("task:taskCreationCheckStatus")}
        </Button>
      ) : null}
      {showOpen ? (
        <Button
          type="button"
          variant={primary === "open" ? "default" : "outline"}
          className={ACTION_CLASS}
          onClick={() => void recovery.openTask()}
          disabled={recovery.busy}
          data-testid="task-create-recovery-open"
        >
          {t("task:taskCreationOpenTask")}
        </Button>
      ) : null}
      {recovery.busy ? (
        <span className="sr-only" role="status" aria-live="polite">
          {t("task:taskCreationBusy")}
        </span>
      ) : null}
    </div>
  );
}
