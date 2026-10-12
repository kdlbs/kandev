import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { TaskWalkthrough } from "@/lib/types/http";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";

export function WalkthroughDiscardConfirmation({
  taskId,
  walkthrough,
  open,
  disabled,
  anchorRef,
  onOpenChange,
  onConfirm,
}: {
  taskId: string;
  walkthrough: TaskWalkthrough;
  open: boolean;
  disabled: boolean;
  anchorRef: RefObject<HTMLButtonElement | null>;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const actions = {
    open,
    onOpenChange,
    onConfirm,
    title: t("review:discardWalkthroughTitle"),
    description: t("review:discardWalkthroughDescription"),
    cancelLabel: t("common:cancel"),
    confirmLabel: t("review:discardWalkthrough"),
    confirmTestId: "walkthrough-discard-confirm",
    testId: "walkthrough-discard-confirmation",
  };
  return (
    <MobileActionConfirmation
      {...actions}
      targetKey={`${taskId}:${walkthrough.id}:${walkthrough.updated_at}`}
      subject={walkthrough.title}
      focusReturnRef={anchorRef}
      disabled={disabled}
      fallback={isFinePointer ? <ActionConfirmPopover {...actions} anchorRef={anchorRef} /> : null}
    />
  );
}
