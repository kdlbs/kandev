"use client";

import { useCallback, useRef, useState } from "react";
import { IconRotateClockwise2 } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useTranslation } from "react-i18next";

type ResetContextButtonProps = {
  sessionId: string;
  presentation?: "desktop" | "mobile";
};

export function ResetContextButton({
  sessionId,
  presentation = "desktop",
}: ResetContextButtonProps) {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [isResetting, setIsResetting] = useState(false);
  const actionRef = useRef<HTMLButtonElement>(null);
  const clearContextWindow = useAppStore((state) => state.clearContextWindow);
  const resetContextLabel = t("task:resetContext");

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
      setConfirmOpen(false);
    }
  }, [clearContextWindow, sessionId]);

  const resetButton = (
    <Button
      ref={actionRef}
      type="button"
      variant="ghost"
      size="icon"
      aria-label={t("task:resetAgentContext")}
      className="h-7 w-7 cursor-pointer hover:bg-muted/40 text-muted-foreground"
      onClick={() => setConfirmOpen(true)}
      disabled={isResetting}
      data-testid="reset-context-button"
    >
      {isResetting ? (
        <GridSpinner className="h-4 w-4" />
      ) : (
        <IconRotateClockwise2 className="h-4 w-4" />
      )}
    </Button>
  );

  if (presentation === "mobile") {
    return confirmOpen ? (
      <InlineConfirmActions
        density="touch"
        testId="reset-context-inline-confirm"
        ariaLabel={t("task:resetAgentContext")}
        cancelLabel={t("common:cancel")}
        confirmLabel={resetContextLabel}
        confirmAriaLabel={resetContextLabel}
        confirmTestId="reset-context-confirm"
        onCancel={() => setConfirmOpen(false)}
        onClose={() => setConfirmOpen(false)}
        onConfirm={handleReset}
      />
    ) : (
      <Tooltip>
        <TooltipTrigger asChild>{resetButton}</TooltipTrigger>
        <TooltipContent>{t("task:resetAgentContextClearsConversationHistory")}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>{resetButton}</TooltipTrigger>
        <TooltipContent>{t("task:resetAgentContextClearsConversationHistory")}</TooltipContent>
      </Tooltip>
      <ActionConfirmPopover
        open={confirmOpen}
        anchorRef={actionRef}
        title={t("task:resetAgentContext")}
        description={t("task:thisWillClearTheAgentS")}
        cancelLabel={t("common:cancel")}
        confirmLabel={isResetting ? t("task:resetting") : resetContextLabel}
        confirmAriaLabel={resetContextLabel}
        confirmTestId="reset-context-confirm"
        testId="reset-context-confirm-popover"
        onOpenChange={setConfirmOpen}
        onConfirm={handleReset}
      />
    </>
  );
}
