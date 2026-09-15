"use client";

import { useRef, type RefObject } from "react";
import { IconRestore } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import type { TaskPlanRevision } from "@/lib/types/http";

type RevisionRestoreProps = {
  revision: TaskPlanRevision;
  isCurrent: boolean;
  isSaving: boolean;
  rowConfirmTarget: TaskPlanRevision | null;
  isFinePointer: boolean;
  onRevertCancel: () => void;
  onRevert: (revision: TaskPlanRevision) => Promise<void>;
};

type RevisionRestoreActionProps = RevisionRestoreProps & {
  onRevertRequest: (revision: TaskPlanRevision) => void;
};

export function MobileRevisionRestoreConfirmation({
  taskId,
  target,
  isSaving,
  anchorRef,
  onClose,
  onRevert,
}: {
  taskId: string;
  target: TaskPlanRevision | null;
  isSaving: boolean;
  anchorRef: RefObject<HTMLButtonElement | null>;
  onClose: () => void;
  onRevert: (revision: TaskPlanRevision) => Promise<void>;
}) {
  const { t } = useTranslation();
  const version = target?.revision_number;
  return (
    <MobileActionConfirmation
      open={target !== null}
      targetKey={`${taskId}:${target?.id}`}
      title={t("task:restoreToVersionConfirm", { version })}
      subject={target?.title}
      description={t("task:restoreToVersionDescription", { version })}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("task:restoreVersion", { version })}
      confirmTestId="plan-revision-restore-confirm"
      focusReturnRef={anchorRef}
      disabled={isSaving}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      onConfirm={() => (target ? onRevert(target) : undefined)}
    />
  );
}

export function RevisionRestoreAction({
  revision,
  isCurrent,
  isSaving,
  rowConfirmTarget,
  isFinePointer,
  onRevertRequest,
  onRevertCancel,
  onRevert,
}: RevisionRestoreActionProps) {
  const { t } = useTranslation();
  const restoreButtonRef = useRef<HTMLButtonElement>(null);
  const isConfirming = rowConfirmTarget?.id === revision.id;
  const version = revision.revision_number;
  const restoreLabel = t("task:restoreVersion", { version });

  if (isCurrent) return null;

  return (
    <>
      {(!isConfirming || isFinePointer) && (
        <Button
          ref={restoreButtonRef}
          size="sm"
          variant="ghost"
          disabled={isSaving}
          className="h-11 md:h-7 px-2 text-xs cursor-pointer shrink-0 gap-1"
          onClick={(event) => {
            event.stopPropagation();
            onRevertRequest(revision);
          }}
          data-testid="plan-revision-revert-button"
        >
          <IconRestore className="h-3.5 w-3.5" />
          {t("task:restore")}
        </Button>
      )}
      {isFinePointer && (
        <ActionConfirmPopover
          open={isConfirming}
          disabled={isSaving}
          anchorRef={restoreButtonRef}
          title={t("task:restoreToVersionConfirm", { version })}
          description={t("task:restoreToVersionDescription", { version })}
          cancelLabel={t("common:cancel")}
          confirmLabel={restoreLabel}
          confirmAriaLabel={restoreLabel}
          confirmTestId="plan-revision-restore-confirm"
          testId="plan-revision-restore-confirm-popover"
          onOpenChange={(nextOpen) => {
            if (!nextOpen) onRevertCancel();
          }}
          onCancel={onRevertCancel}
          onConfirm={() => onRevert(revision)}
        />
      )}
    </>
  );
}

export function RevisionInlineConfirmation({
  revision,
  isCurrent,
  isSaving,
  isFinePointer,
  rowConfirmTarget,
  onRevertCancel,
  onRevert,
}: RevisionRestoreProps) {
  const { t } = useTranslation();
  if (isCurrent || isFinePointer || rowConfirmTarget?.id !== revision.id) return null;

  const version = revision.revision_number;
  const restoreLabel = t("task:restoreVersion", { version });
  return (
    <div className="mt-2 min-w-0">
      <InlineConfirmActions
        density="touch"
        disabled={isSaving}
        testId="plan-revision-restore-inline-confirmation"
        ariaLabel={t("task:restoreToVersionConfirm", { version })}
        description={t("task:restoreToVersionDescription", { version })}
        cancelLabel={t("common:cancel")}
        confirmLabel={restoreLabel}
        confirmAriaLabel={restoreLabel}
        confirmTestId="plan-revision-restore-confirm"
        onCancel={onRevertCancel}
        onClose={onRevertCancel}
        onConfirm={() => onRevert(revision)}
      />
    </div>
  );
}
