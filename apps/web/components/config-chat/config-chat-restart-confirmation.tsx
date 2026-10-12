import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";

type Props = {
  open: boolean;
  targetKey: string;
  anchorRef: RefObject<HTMLButtonElement | null>;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function ConfigChatRestartConfirmation({
  open,
  targetKey,
  anchorRef,
  onOpenChange,
  onConfirm,
}: Props) {
  const { t } = useTranslation();
  const content = {
    title: t("configChat:restartTitle"),
    description: t("configChat:restartDescription"),
    cancelLabel: t("common:cancel"),
    confirmLabel: t("configChat:restartSession"),
    confirmTestId: "config-chat-confirm-restart",
  };
  return (
    <MobileActionConfirmation
      {...content}
      open={open}
      targetKey={targetKey}
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
      focusReturnRef={anchorRef}
      testId="config-chat-restart-confirmation"
      fallback={
        <ActionConfirmPopover
          {...content}
          open={open}
          anchorRef={anchorRef}
          focusReturnRef={anchorRef}
          onOpenChange={onOpenChange}
          onConfirm={onConfirm}
          confirmationBoundary
          testId="config-chat-restart-confirmation"
        />
      }
    />
  );
}
