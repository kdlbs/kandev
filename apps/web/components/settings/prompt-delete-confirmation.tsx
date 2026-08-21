"use client";

import type { RefObject } from "react";
import { Trans, useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";

type PromptDeleteConfirmationProps = {
  promptName: string;
  open: boolean;
  isFinePointer: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
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
  onOpenChange,
  onCancel,
  onConfirm,
}: PromptDeleteConfirmationProps) {
  const { t } = useTranslation();
  const promptDeleteLabel = t("settings:promptDelete");
  const cancelLabel = t("settings:cancel");
  const description = <PromptDeleteDescription promptName={promptName} />;

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
        onCancel={onCancel}
        onClose={onCancel}
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
      testId="prompt-delete-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}
