"use client";

import { useCallback } from "react";
import {
  IconArrowUp,
  IconFileTextSpark,
  IconPaperclip,
  IconPlayerPauseFilled,
} from "@tabler/icons-react";

import { GridSpinner } from "@/components/grid-spinner";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { formatShortcut } from "@/lib/keyboard/utils";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

type SubmitButtonProps = {
  isAgentBusy: boolean;
  canCancelAgent?: boolean;
  sessionId: string | null;
  hasContent: boolean;
  isDisabled: boolean;
  submitDisabledReason?: string;
  isSending: boolean;
  planModeEnabled: boolean;
  onCancel: () => void | Promise<void>;
  onSubmit: () => void;
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
  presentation?: "desktop" | "mobile";
};

type SendSubmitButtonProps = Pick<
  SubmitButtonProps,
  "isDisabled" | "isSending" | "planModeEnabled" | "onSubmit" | "submitShortcut" | "presentation"
> & {
  tooltipDescription?: string;
};

function submitTooltipDescription(
  isAgentBusy: boolean,
  planModeEnabled: boolean,
  submitDisabledReason?: string,
) {
  if (submitDisabledReason) return submitDisabledReason;
  if (isAgentBusy) return t("task:submitTooltipQueueMessage");
  if (planModeEnabled) return t("task:submitTooltipRequestPlanChanges");
  return undefined;
}

function SendSubmitButton({
  isDisabled,
  isSending,
  planModeEnabled,
  onSubmit,
  submitShortcut,
  presentation = "desktop",
  tooltipDescription,
}: SendSubmitButtonProps) {
  const { t } = useTranslation();
  return (
    <KeyboardShortcutTooltip
      shortcut={submitShortcut}
      description={tooltipDescription}
      enabled={!isDisabled || !!tooltipDescription}
    >
      <span
        className="inline-flex"
        tabIndex={isDisabled && !!tooltipDescription ? 0 : undefined}
        aria-label={isDisabled ? (tooltipDescription ?? t("task:submitUnavailable")) : undefined}
      >
        <Button
          type="button"
          variant="default"
          size="icon"
          className={cn(
            presentation === "mobile"
              ? "min-h-11 min-w-11 rounded-full cursor-pointer"
              : "h-7 w-7 rounded-full cursor-pointer",
            planModeEnabled && "bg-violet-600 hover:bg-violet-500",
          )}
          disabled={isDisabled}
          onClick={onSubmit}
          data-testid="submit-message-button"
        >
          {isSending && <GridSpinner className="text-primary-foreground" />}
          {!isSending && planModeEnabled && <IconFileTextSpark className="h-4 w-4" />}
          {!isSending && !planModeEnabled && <IconArrowUp className="h-4 w-4" />}
        </Button>
      </span>
    </KeyboardShortcutTooltip>
  );
}

export function SubmitButton({
  isAgentBusy,
  canCancelAgent = isAgentBusy,
  sessionId,
  hasContent,
  isDisabled,
  submitDisabledReason,
  isSending,
  planModeEnabled,
  onCancel,
  onSubmit,
  submitShortcut,
  presentation = "desktop",
}: SubmitButtonProps) {
  const { t } = useTranslation();
  const showSendButton = !isAgentBusy || hasContent;
  const storeApi = useAppStoreApi();
  const isCancelling = useAppStore((state) => {
    if (!sessionId) return false;
    return (
      state.taskSessions.items[sessionId]?.cancellation_pending === true ||
      state.chatInput.cancellingBySessionId[sessionId] === true
    );
  });
  const tooltipDescription = submitTooltipDescription(
    isAgentBusy,
    planModeEnabled,
    submitDisabledReason,
  );
  const handleCancelClick = useCallback(async () => {
    if (!sessionId) return;
    if (storeApi.getState().chatInput.cancellingBySessionId[sessionId]) return;
    storeApi.getState().setCancelTurnPending(sessionId, true);
    try {
      await onCancel();
    } catch (error) {
      console.error("Failed to cancel agent turn:", error);
    } finally {
      storeApi.getState().setCancelTurnPending(sessionId, false);
    }
  }, [onCancel, sessionId, storeApi]);

  return (
    <div className="flex items-center gap-1">
      {canCancelAgent && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="secondary"
              size="icon"
              className={cn(
                presentation === "mobile" ? "min-h-11 min-w-11" : "h-7 w-7",
                "rounded-full cursor-pointer bg-destructive/10 text-destructive hover:bg-destructive/20 disabled:cursor-not-allowed disabled:opacity-70",
              )}
              onClick={handleCancelClick}
              disabled={isCancelling}
              data-testid="cancel-agent-button"
            >
              {isCancelling ? (
                <GridSpinner className="text-destructive" />
              ) : (
                <IconPlayerPauseFilled className="h-3.5 w-3.5" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {isCancelling ? t("task:cancelling") : t("task:cancelAgent")}
          </TooltipContent>
        </Tooltip>
      )}
      {showSendButton && (
        <SendSubmitButton
          isDisabled={isDisabled}
          isSending={isSending}
          planModeEnabled={planModeEnabled}
          onSubmit={onSubmit}
          submitShortcut={submitShortcut}
          presentation={presentation}
          tooltipDescription={tooltipDescription}
        />
      )}
    </div>
  );
}

export function PlanToggleButton({
  planModeEnabled,
  planModeAvailable,
  onPlanModeChange,
  presentation = "desktop",
}: {
  planModeEnabled: boolean;
  planModeAvailable: boolean;
  onPlanModeChange: (enabled: boolean) => void;
  presentation?: "desktop" | "mobile";
}) {
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const planModeShortcutLabel = formatShortcut(getShortcut("TOGGLE_PLAN_MODE", keyboardShortcuts));
  const tooltip = planModeAvailable
    ? `Toggle plan mode (${planModeShortcutLabel}) - Agent collaborates on the plan without implementing changes`
    : `Toggle plan layout (${planModeShortcutLabel}) - View and edit the plan (agent cannot read/write it without MCP)`;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          data-testid="plan-mode-toggle-button"
          data-plan-available={planModeAvailable}
          data-plan-enabled={planModeEnabled}
          className={cn(
            presentation === "mobile"
              ? "min-h-11 min-w-11 gap-1.5 px-2 hover:bg-muted/40 cursor-pointer"
              : "h-7 gap-1.5 px-2 hover:bg-muted/40 cursor-pointer",
            planModeEnabled && planModeAvailable && "bg-violet-500/15 text-violet-400",
          )}
          onClick={() => onPlanModeChange(!planModeEnabled)}
        >
          <IconFileTextSpark className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">{tooltip}</TooltipContent>
    </Tooltip>
  );
}

export function AttachFilesButton({
  onClick,
  presentation = "desktop",
}: {
  onClick: () => void;
  presentation?: "desktop" | "mobile";
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={cn(
            presentation === "mobile"
              ? "min-h-11 min-w-11 gap-1.5 px-2 cursor-pointer hover:bg-muted/40"
              : "h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40",
          )}
          onClick={onClick}
          data-testid="chat-attachments-button"
        >
          <IconPaperclip className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("task:attachFiles")}</TooltipContent>
    </Tooltip>
  );
}
