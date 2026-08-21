"use client";

import { useEffect, useRef, type RefObject } from "react";
import { Trans, useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";

type PromptDeleteConfirmationProps = {
  promptName: string;
  open: boolean;
  isFinePointer: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  isBusy: boolean;
  onClose: () => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

function PromptDeleteDescription({ promptName }: { promptName: string }) {
  return (
    <Trans i18nKey="settings:promptDeleteDescription" values={{ name: `@${promptName}` }}>
      This will permanently remove{" "}
      <span className="font-medium text-foreground">{`@${promptName}`}</span>. This action cannot be
      undone.
    </Trans>
  );
}

export function PromptDeleteConfirmation({
  promptName,
  open,
  isFinePointer,
  anchorRef,
  isBusy,
  onClose,
  onCancel,
  onConfirm,
}: PromptDeleteConfirmationProps) {
  const { t } = useTranslation();
  const restoreFocusRef = useRef(false);
  const promptDeleteLabel = t("settings:promptDelete");
  const cancelLabel = t("settings:cancel");
  const description = <PromptDeleteDescription promptName={promptName} />;
  const handleCancel = () => {
    restoreFocusRef.current = !isFinePointer;
    onCancel();
  };

  useEffect(() => {
    if (isFinePointer || open || !restoreFocusRef.current) return;
    restoreFocusRef.current = false;
    anchorRef.current?.focus();
  }, [anchorRef, isFinePointer, open]);

  if (!isFinePointer) {
    return open ? (
      <InlineConfirmActions
        density="touch"
        testId="prompt-delete-inline-confirmation"
        ariaLabel={promptDeleteLabel}
        description={description}
        cancelLabel={cancelLabel}
        confirmLabel={promptDeleteLabel}
        confirmAriaLabel={promptDeleteLabel}
        confirmTestId="prompt-delete-confirm"
        confirmDisabled={isBusy}
        onCancel={handleCancel}
        onClose={onClose}
        onConfirm={onConfirm}
      />
    ) : null;
  }

  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      title={promptDeleteLabel}
      description={description}
      cancelLabel={cancelLabel}
      confirmLabel={promptDeleteLabel}
      confirmAriaLabel={promptDeleteLabel}
      confirmTestId="prompt-delete-confirm"
      confirmDisabled={isBusy}
      testId="prompt-delete-confirm-popover"
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onClose();
      }}
      onCancel={handleCancel}
      onConfirm={onConfirm}
    />
  );
}
