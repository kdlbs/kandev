"use client";

import { memo } from "react";
import {
  IconArrowUp,
  IconFileTextSpark,
  IconPlayerPauseFilled,
  IconAt,
  IconPlugConnected,
  IconPlugConnectedX,
  IconRocket,
} from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
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
            planModeEnabled && "bg-slate-600 hover:bg-slate-500",
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
    : "Plan mode requires an agent with MCP support";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={cn(
            "h-7 gap-1.5 px-2 hover:bg-muted/40",
            planModeAvailable ? "cursor-pointer" : "opacity-50 cursor-not-allowed",
            planModeEnabled && planModeAvailable && "bg-slate-500/15 text-slate-400",
          )}
          onClick={() => {
            console.log("[PlanToggle] clicked", { planModeEnabled, planModeAvailable });
            if (planModeAvailable) onPlanModeChange(!planModeEnabled);
          }}
          aria-disabled={!planModeAvailable}
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
          className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 text-slate-400"
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

function McpIndicator({ mcpServers }: { mcpServers: string[] }) {
  const hasMcp = mcpServers.length > 0;
  const tooltipText = hasMcp
    ? `MCP Servers: ${mcpServers.join(", ")}`
    : "Agent does not support MCP";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          className={cn(
            "h-7 w-7 flex items-center justify-center rounded-md",
            hasMcp ? "text-foreground" : "text-muted-foreground/40",
          )}
        >
          {hasMcp ? (
            <IconPlugConnected className="h-4 w-4" />
          ) : (
            <IconPlugConnectedX className="h-4 w-4" />
          )}
        </div>
      </TooltipTrigger>
      <TooltipContent>{tooltipText}</TooltipContent>
    </Tooltip>
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

      <div className="flex items-center gap-0.5 shrink-0">
        <SessionsDropdown
          taskId={taskId}
          activeSessionId={sessionId}
          taskTitle={taskTitle}
          taskDescription={taskDescription}
        />
        <TokenUsageDisplay sessionId={sessionId} />
        <ModelSelector sessionId={sessionId} />
        {planModeEnabled && !isAgentBusy && onImplementPlan && (
          <ImplementPlanButton onClick={onImplementPlan} />
        )}
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
    </div>
  );
});
