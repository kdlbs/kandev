"use client";

import { useCallback } from "react";
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
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { formatShortcut } from "@/lib/keyboard/utils";
import { cn } from "@/lib/utils";
import type { MCPAttachmentHistory } from "@/lib/state/slices/session-runtime/types";

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
}: SubmitButtonProps) {
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

const mcpStatusColor: Record<string, string> = {
  active: "bg-emerald-500",
  connected: "bg-amber-500",
  delivered: "bg-amber-500",
  failed: "bg-destructive",
  filtered: "bg-muted-foreground/50",
  unavailable: "bg-muted-foreground/50",
  unknown: "bg-muted-foreground/50",
};

const mcpStatusLabel: Record<string, string> = {
  active: "Active",
  connected: "Connected",
  delivered: "Delivered — connection unverified",
  failed: "Failed",
  filtered: "Filtered",
  unavailable: "Unavailable",
  unknown: "Unknown",
};

export function McpIndicator({
  mcpServers,
  attachmentHistory,
}: {
  mcpServers: string[];
  attachmentHistory?: MCPAttachmentHistory;
}) {
  const usesTouchDrawer = useTouchDrawer();
  const hasMcp = mcpServers.length > 0;
  const Icon = hasMcp ? IconPlugConnected : IconPlugConnectedX;
  const observedServers = attachmentHistory?.current.servers;
  const servers =
    observedServers && observedServers.length > 0
      ? observedServers
      : mcpServers.map((name) => ({ name, status: "unknown" as const, summary: undefined }));
  const statusList = hasMcp ? (
    <div className="space-y-1">
      <div className="font-medium">MCP servers</div>
      {servers.map((server) => (
        <div key={server.name} className="flex items-center gap-2">
          <span
            className={cn(
              "h-2 w-2 shrink-0 rounded-full",
              mcpStatusColor[server.status] ?? mcpStatusColor.unknown,
            )}
            aria-hidden="true"
          />
          <span className="truncate">{server.name}</span>
          <span className="text-muted-foreground">
            {mcpStatusLabel[server.status] ?? "Unknown"}
          </span>
          {server.summary && <span className="text-muted-foreground">{server.summary}</span>}
        </div>
      ))}
    </div>
  ) : (
    "Agent does not support MCP"
  );
  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label="Show MCP connection status"
      className={cn(
        "h-7 w-7 cursor-pointer rounded-md hover:bg-muted/40",
        hasMcp ? "text-foreground" : "text-muted-foreground/40",
      )}
      data-testid="mcp-status-trigger"
    >
      <Icon className="h-4 w-4" />
    </Button>
  );

  if (usesTouchDrawer) {
    return (
      <Drawer>
        <DrawerTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label="Show MCP connection status"
            className={cn(
              "h-11 w-11 cursor-pointer rounded-md active:scale-95",
              hasMcp ? "text-foreground" : "text-muted-foreground/40",
            )}
            data-testid="mcp-status-trigger"
          >
            <Icon className="h-4 w-4" />
          </Button>
        </DrawerTrigger>
        <DrawerContent data-testid="mcp-status-drawer" className="max-h-[80dvh]">
          <DrawerHeader>
            <DrawerTitle>MCP connection status</DrawerTitle>
          </DrawerHeader>
          <div className="min-h-0 overflow-y-auto px-4 pb-[calc(1rem+env(safe-area-inset-bottom))] text-sm">
            {statusList}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent data-testid="mcp-status-popover">{statusList}</TooltipContent>
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
