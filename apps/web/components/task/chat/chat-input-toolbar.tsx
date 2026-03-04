"use client";

import { memo, useState, useCallback } from "react";
import {
  IconArrowUp,
  IconFileTextSpark,
  IconPlayerPauseFilled,
  IconAt,
  IconPlugConnected,
  IconPlugConnectedX,
  IconRocket,
  IconRotateClockwise2,
  IconSparkles,
} from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { cn } from "@/lib/utils";
import { getWebSocketClient } from "@/lib/ws/connection";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { TokenUsageDisplay } from "@/components/task/chat/token-usage-display";
import { SessionsDropdown } from "@/components/task/sessions-dropdown";
import { ModelSelector } from "@/components/task/model-selector";
import { ContextPopover } from "./context-popover";
import type { ContextFile } from "@/lib/state/context-files-store";

export type ChatInputToolbarProps = {
  planModeEnabled: boolean;
  planModeAvailable?: boolean;
  mcpServers?: string[];
  onPlanModeChange: (enabled: boolean) => void;
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
  taskDescription: string;
  isAgentBusy: boolean;
  isDisabled: boolean;
  isSending: boolean;
  onCancel: () => void;
  onSubmit: () => void;
  submitKey?: "enter" | "cmd_enter";
  contextCount?: number;
  contextPopoverOpen?: boolean;
  onContextPopoverOpenChange?: (open: boolean) => void;
  /** Whether plan is selected as context in the popover (independent of plan panel) */
  planContextEnabled?: boolean;
  contextFiles?: ContextFile[];
  onToggleFile?: (file: ContextFile) => void;
  onImplementPlan?: () => void;
  /** Callback to enhance the current prompt with AI */
  onEnhancePrompt?: () => void;
  /** Whether prompt enhancement is in progress */
  isEnhancingPrompt?: boolean;
};

type SubmitButtonProps = {
  isAgentBusy: boolean;
  isDisabled: boolean;
  isSending: boolean;
  planModeEnabled: boolean;
  onCancel: () => void;
  onSubmit: () => void;
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
};

function SubmitButton({
  isAgentBusy,
  isDisabled,
  isSending,
  planModeEnabled,
  onCancel,
  onSubmit,
  submitShortcut,
}: SubmitButtonProps) {
  return (
    <KeyboardShortcutTooltip
      shortcut={submitShortcut}
      description={planModeEnabled ? "Request plan changes" : undefined}
      enabled={!isAgentBusy && !isDisabled}
    >
      {isAgentBusy ? (
        <Button
          type="button"
          variant="secondary"
          size="icon"
          className="h-7 w-7 rounded-full cursor-pointer bg-destructive/10 text-destructive hover:bg-destructive/20"
          onClick={onCancel}
        >
          <IconPlayerPauseFilled className="h-3.5 w-3.5" />
        </Button>
      ) : (
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
        >
          {isSending && <GridSpinner className="text-primary-foreground" />}
          {!isSending && planModeEnabled && <IconFileTextSpark className="h-4 w-4" />}
          {!isSending && !planModeEnabled && <IconArrowUp className="h-4 w-4" />}
        </Button>
      )}
    </KeyboardShortcutTooltip>
  );
}

function PlanToggleButton({
  planModeEnabled,
  planModeAvailable,
  onPlanModeChange,
}: {
  planModeEnabled: boolean;
  planModeAvailable: boolean;
  onPlanModeChange: (enabled: boolean) => void;
}) {
  const tooltip = planModeAvailable
    ? "Toggle plan mode (Shift+Tab) — Agent collaborates on the plan without implementing changes"
    : "Toggle plan layout (Shift+Tab) — View and edit the plan (agent cannot read/write it without MCP)";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
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

function ImplementPlanButton({ onClick }: { onClick: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 text-violet-400"
          onClick={onClick}
        >
          <IconRocket className="h-4 w-4" />
          <span className="text-xs">Implement</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>Implement the plan</TooltipContent>
    </Tooltip>
  );
}

function EnhancePromptButton({ onClick, isLoading }: { onClick: () => void; isLoading: boolean }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7 cursor-pointer hover:bg-muted/40 text-slate-400"
          onClick={onClick}
          disabled={isLoading}
          aria-label="Enhance prompt with AI"
          aria-busy={isLoading}
        >
          {isLoading ? <GridSpinner className="h-4 w-4" /> : <IconSparkles className="h-4 w-4" />}
        </Button>
      </TooltipTrigger>
      <TooltipContent>Enhance prompt with AI</TooltipContent>
    </Tooltip>
  );
}

function McpIndicator({ mcpServers }: { mcpServers: string[] }) {
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

function ResetContextButton({ sessionId }: { sessionId: string }) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [isResetting, setIsResetting] = useState(false);

  const handleReset = useCallback(async () => {
    setIsResetting(true);
    try {
      const client = getWebSocketClient();
      if (!client) return;
      await client.request("session.reset_context", { session_id: sessionId }, 30000);
    } catch (error) {
      console.error("Failed to reset agent context:", error);
    } finally {
      setIsResetting(false);
      setConfirmOpen(false);
    }
  }, [sessionId]);

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
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
        </TooltipTrigger>
        <TooltipContent>
          Reset agent context — clears conversation history, preserves workspace
        </TooltipContent>
      </Tooltip>
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Reset agent context?</AlertDialogTitle>
            <AlertDialogDescription>
              This will clear the agent&apos;s conversation history and start a fresh context. Your
              workspace, files, and git state will be preserved.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleReset}
              disabled={isResetting}
              className="cursor-pointer"
              data-testid="reset-context-confirm"
            >
              {isResetting ? "Resetting..." : "Reset Context"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

type ToolbarRightSectionProps = {
  taskId: string | null;
  sessionId: string | null;
  taskTitle?: string;
  taskDescription: string;
  planModeEnabled: boolean;
  isAgentBusy: boolean;
  isDisabled: boolean;
  isSending: boolean;
  onCancel: () => void;
  onSubmit: () => void;
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
  onImplementPlan?: () => void;
};

function ToolbarRightSection({
  taskId,
  sessionId,
  taskTitle,
  taskDescription,
  planModeEnabled,
  isAgentBusy,
  isDisabled,
  isSending,
  onCancel,
  onSubmit,
  submitShortcut,
  onEnhancePrompt,
  isEnhancingPrompt,
  onImplementPlan,
}: ToolbarRightSectionProps) {
  const showEnhance = onEnhancePrompt && !isAgentBusy;
  const showImplement = planModeEnabled && !isAgentBusy && onImplementPlan;
  const showResetContext = !!sessionId && !isAgentBusy;
  return (
    <div className="flex items-center gap-0.5 shrink-0">
      {showResetContext && <ResetContextButton sessionId={sessionId} />}
      <SessionsDropdown
        taskId={taskId}
        activeSessionId={sessionId}
        taskTitle={taskTitle}
        taskDescription={taskDescription}
      />
      <TokenUsageDisplay sessionId={sessionId} />
      <ModelSelector sessionId={sessionId} />
      {showEnhance && (
        <EnhancePromptButton onClick={onEnhancePrompt} isLoading={isEnhancingPrompt ?? false} />
      )}
      {showImplement && <ImplementPlanButton onClick={onImplementPlan} />}
      <div className="ml-1">
        <SubmitButton
          isAgentBusy={isAgentBusy}
          isDisabled={isDisabled}
          isSending={isSending}
          planModeEnabled={planModeEnabled}
          onCancel={onCancel}
          onSubmit={onSubmit}
          submitShortcut={submitShortcut}
        />
      </div>
    </div>
  );
}

export const ChatInputToolbar = memo(function ChatInputToolbar({
  planModeEnabled,
  planModeAvailable = true,
  mcpServers = [],
  onPlanModeChange,
  sessionId,
  taskId,
  taskTitle,
  taskDescription,
  isAgentBusy,
  isDisabled,
  isSending,
  onCancel,
  onSubmit,
  submitKey = "cmd_enter",
  contextCount = 0,
  contextPopoverOpen = false,
  onContextPopoverOpenChange,
  planContextEnabled = false,
  contextFiles = [],
  onToggleFile,
  onImplementPlan,
  onEnhancePrompt,
  isEnhancingPrompt = false,
}: ChatInputToolbarProps) {
  const submitShortcut = submitKey === "enter" ? SHORTCUTS.SUBMIT_ENTER : SHORTCUTS.SUBMIT;

  return (
    <div className="flex items-center gap-1 px-1 pt-0 pb-0.5 border-t border-border">
      <div className="flex items-center gap-0.5">
        <PlanToggleButton
          planModeEnabled={planModeEnabled}
          planModeAvailable={planModeAvailable}
          onPlanModeChange={onPlanModeChange}
        />

        <McpIndicator mcpServers={mcpServers} />

        <ContextPopover
          open={contextPopoverOpen}
          onOpenChange={onContextPopoverOpenChange ?? (() => {})}
          trigger={
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 relative"
            >
              <IconAt className="h-4 w-4" />
              {contextCount > 0 && (
                <span className="absolute -top-1 -right-1 h-4 min-w-4 rounded-full bg-muted-foreground/80 text-[10px] text-background flex items-center justify-center px-0.5">
                  {contextCount}
                </span>
              )}
            </Button>
          }
          sessionId={sessionId}
          planContextEnabled={planContextEnabled}
          contextFiles={contextFiles}
          onToggleFile={onToggleFile ?? (() => {})}
        />
      </div>

      <div className="flex-1" />

      <ToolbarRightSection
        taskId={taskId}
        sessionId={sessionId}
        taskTitle={taskTitle}
        taskDescription={taskDescription}
        planModeEnabled={planModeEnabled}
        isAgentBusy={isAgentBusy}
        isDisabled={isDisabled}
        isSending={isSending}
        onCancel={onCancel}
        onSubmit={onSubmit}
        submitShortcut={submitShortcut}
        onEnhancePrompt={onEnhancePrompt}
        isEnhancingPrompt={isEnhancingPrompt}
        onImplementPlan={onImplementPlan}
      />
    </div>
  );
});
