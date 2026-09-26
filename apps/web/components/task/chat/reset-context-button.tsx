"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { IconRotateClockwise2 } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useTranslation } from "react-i18next";

type ResetContextButtonProps = {
  sessionId: string;
  presentation?: "desktop" | "mobile";
  onConfirmationOpenChange?: (open: boolean) => void;
};

type ResetContextTriggerProps = {
  actionRef: RefObject<HTMLButtonElement | null>;
  isResetting: boolean;
  ariaLabel: string;
  tooltip: string;
  onOpen: () => void;
};

function ResetContextTrigger({
  actionRef,
  isResetting,
  ariaLabel,
  tooltip,
  onOpen,
}: ResetContextTriggerProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const [tooltipOpen, setTooltipOpen] = useState(false);
  return (
    <Tooltip open={!isMobile && tooltipOpen} onOpenChange={setTooltipOpen}>
      <TooltipTrigger asChild>
        <Button
          ref={actionRef}
          type="button"
          variant="ghost"
          size="icon"
          aria-label={ariaLabel}
          className="h-7 w-7 cursor-pointer text-muted-foreground hover:bg-muted/40 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
          onClick={onOpen}
          disabled={isResetting}
          data-testid="reset-context-button"
        >
          {isResetting ? (
            <GridSpinner className="h-4 w-4" />
          ) : (
            <IconRotateClockwise2 className="h-4 w-4" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

export function ResetContextButton({
  sessionId,
  presentation = "desktop",
  onConfirmationOpenChange,
}: ResetContextButtonProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const { isResetting, handleReset } = useContextResetRequest(sessionId);
  const actionRef = useRef<HTMLButtonElement>(null);
  const restoreFocusAfterCancelRef = useRef(false);
  const resetContextLabel = t("task:resetContext");
  const resetContextAriaLabel = t("task:resetAgentContext");
  const resetContextTooltip = t("task:resetAgentContextClearsConversationHistory");

  const updateConfirmationOpen = useCallback(
    (open: boolean) => {
      setConfirmOpen(open);
      onConfirmationOpenChange?.(open);
    },
    [onConfirmationOpenChange],
  );

  useEffect(
    () => () => {
      onConfirmationOpenChange?.(false);
    },
    [onConfirmationOpenChange],
  );

  useLayoutEffect(() => {
    if (confirmOpen || !restoreFocusAfterCancelRef.current) return;
    restoreFocusAfterCancelRef.current = false;
    actionRef.current?.focus();
  }, [confirmOpen]);

  const handleMobileCancel = () => {
    restoreFocusAfterCancelRef.current = true;
    updateConfirmationOpen(false);
  };

  const resetContextTrigger = (
    <ResetContextTrigger
      actionRef={actionRef}
      isResetting={isResetting}
      ariaLabel={resetContextAriaLabel}
      tooltip={resetContextTooltip}
      onOpen={() => updateConfirmationOpen(true)}
    />
  );

  const confirmationProps = {
    description: t("task:thisWillClearTheAgentS"),
    cancelLabel: t("common:cancel"),
    confirmLabel: resetContextLabel,
    confirmAriaLabel: resetContextLabel,
    confirmTestId: "reset-context-confirm",
    onConfirm: handleReset,
  };
  const fallback =
    presentation === "mobile" ? (
      <InlineConfirmActions
        {...confirmationProps}
        density="touch"
        testId="reset-context-inline-confirm"
        ariaLabel={resetContextAriaLabel}
        onCancel={handleMobileCancel}
        onClose={() => updateConfirmationOpen(false)}
      />
    ) : (
      <ActionConfirmPopover
        {...confirmationProps}
        open={confirmOpen}
        anchorRef={actionRef}
        title={resetContextAriaLabel}
        testId="reset-context-confirm-popover"
        onOpenChange={updateConfirmationOpen}
      />
    );

  return (
    <>
      {isMobile || presentation !== "mobile" || !confirmOpen ? resetContextTrigger : null}
      <MobileActionConfirmation
        {...confirmationProps}
        open={confirmOpen}
        targetKey={sessionId}
        title={resetContextAriaLabel}
        focusReturnRef={actionRef}
        onOpenChange={updateConfirmationOpen}
        fallback={fallback}
      />
    </>
  );
}

function useContextResetRequest(sessionId: string) {
  const [isResetting, setIsResetting] = useState(false);
  const clearContextWindow = useAppStore((state) => state.clearContextWindow);
  const handleReset = useCallback(async () => {
    setIsResetting(true);
    try {
      const client = getWebSocketClient();
      if (!client) return;
      await client.request("session.reset_context", { session_id: sessionId }, 30000);
      clearContextWindow(sessionId);
    } catch (error) {
      console.error("Failed to reset agent context:", error);
    } finally {
      setIsResetting(false);
    }
  }, [clearContextWindow, sessionId]);
  return { isResetting, handleReset };
}
