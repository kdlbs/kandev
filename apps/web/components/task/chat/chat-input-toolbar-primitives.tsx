"use client";

import { useCallback, useRef, useState } from "react";
import {
  IconArrowUp,
  IconFileTextSpark,
  IconPaperclip,
  IconPlayerPauseFilled,
  IconPlugConnected,
  IconPlugConnectedX,
} from "@tabler/icons-react";

import { GridSpinner } from "@/components/grid-spinner";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { useAppStore } from "@/components/state-provider";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { formatShortcut } from "@/lib/keyboard/utils";
import { cn } from "@/lib/utils";

type SubmitButtonProps = {
  isAgentBusy: boolean;
  hasContent: boolean;
  isDisabled: boolean;
  submitDisabledReason?: string;
  isSending: boolean;
  planModeEnabled: boolean;
  onCancel: () => void | Promise<void>;
  onSubmit: () => void;
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
};

type SendSubmitButtonProps = Pick<
  SubmitButtonProps,
  "isDisabled" | "isSending" | "planModeEnabled" | "onSubmit" | "submitShortcut"
> & {
  tooltipDescription?: string;
};

function submitTooltipDescription(
  isAgentBusy: boolean,
  planModeEnabled: boolean,
  submitDisabledReason?: string,
) {
  if (submitDisabledReason) return submitDisabledReason;
  if (isAgentBusy) return "Queue message";
  if (planModeEnabled) return "Request plan changes";
  return undefined;
}

function SendSubmitButton({
  isDisabled,
  isSending,
  planModeEnabled,
  onSubmit,
  submitShortcut,
  tooltipDescription,
}: SendSubmitButtonProps) {
  return (
    <KeyboardShortcutTooltip
      shortcut={submitShortcut}
      description={tooltipDescription}
      enabled={!isDisabled || !!tooltipDescription}
    >
      <span
        className="inline-flex"
        tabIndex={isDisabled && !!tooltipDescription ? 0 : undefined}
        aria-label={isDisabled ? (tooltipDescription ?? "Submit unavailable") : undefined}
      >
        <Button
          type="button"
          variant="default"
          size="icon"
          className={cn(
            "h-7 w-7 rounded-full cursor-pointer",
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
  hasContent,
  isDisabled,
  submitDisabledReason,
  isSending,
  planModeEnabled,
  onCancel,
  onSubmit,
  submitShortcut,
}: SubmitButtonProps) {
  const showSendButton = !isAgentBusy || hasContent;
  const [isCancelling, setIsCancelling] = useState(false);
  const tooltipDescription = submitTooltipDescription(
    isAgentBusy,
    planModeEnabled,
    submitDisabledReason,
  );
  const cancellingRef = useRef(false);
  const handleCancelClick = useCallback(async () => {
    if (cancellingRef.current) return;
    cancellingRef.current = true;
    setIsCancelling(true);
    try {
      await onCancel();
    } catch (error) {
      console.error("Failed to cancel agent turn:", error);
    } finally {
      cancellingRef.current = false;
      setIsCancelling(false);
    }
  }, [onCancel]);

  return (
    <div className="flex items-center gap-1">
      {isAgentBusy && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="secondary"
              size="icon"
              className="h-7 w-7 rounded-full cursor-pointer bg-destructive/10 text-destructive hover:bg-destructive/20 disabled:cursor-not-allowed disabled:opacity-70"
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
          <TooltipContent>{isCancelling ? "Cancelling..." : "Cancel agent"}</TooltipContent>
        </Tooltip>
      )}
      {showSendButton && (
        <SendSubmitButton
          isDisabled={isDisabled}
          isSending={isSending}
          planModeEnabled={planModeEnabled}
          onSubmit={onSubmit}
          submitShortcut={submitShortcut}
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
}: {
  planModeEnabled: boolean;
  planModeAvailable: boolean;
  onPlanModeChange: (enabled: boolean) => void;
}) {
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const planModeShortcutLabel = formatShortcut(getShortcut("TOGGLE_PLAN_MODE", keyboardShortcuts));
  const tooltip = planModeAvailable
    ? `Toggle plan mode (${planModeShortcutLabel}) — Agent collaborates on the plan without implementing changes`
    : `Toggle plan layout (${planModeShortcutLabel}) — View and edit the plan (agent cannot read/write it without MCP)`;

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
            "h-7 gap-1.5 px-2 hover:bg-muted/40 cursor-pointer",
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

export function McpIndicator({ mcpServers }: { mcpServers: string[] }) {
  const hasMcp = mcpServers.length > 0;
  const tooltipText = hasMcp
    ? `MCP Servers: ${mcpServers.join(", ")}`
    : "Agent does not support MCP";
  const Icon = hasMcp ? IconPlugConnected : IconPlugConnectedX;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          className={cn(
            "h-7 w-7 flex items-center justify-center rounded-md",
            hasMcp ? "text-foreground" : "text-muted-foreground/40",
          )}
        >
          <Icon className="h-4 w-4" />
        </div>
      </TooltipTrigger>
      <TooltipContent>{tooltipText}</TooltipContent>
    </Tooltip>
  );
}

export function AttachFilesButton({ onClick }: { onClick: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40"
          onClick={onClick}
          data-testid="chat-attachments-button"
        >
          <IconPaperclip className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Attach files</TooltipContent>
    </Tooltip>
  );
}
