"use client";

import { memo, useRef, useState, useCallback, type ReactNode } from "react";
import {
  IconArrowUp,
  IconChevronsLeft,
  IconDots,
  IconFileTextSpark,
  IconPlayerPauseFilled,
  IconAt,
  IconPlugConnected,
  IconPlugConnectedX,
  IconPaperclip,
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
import { useToolbarCollapsed } from "@/hooks/use-toolbar-collapsed";
import { getWebSocketClient } from "@/lib/ws/connection";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { TokenUsageDisplay } from "@/components/task/chat/token-usage-display";
import { SessionsDropdown } from "@/components/task/sessions-dropdown";
import { ModelSelector } from "@/components/task/model-selector";
import { ModeSelector } from "@/components/task/mode-selector";
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
  /** Whether utility agent is configured for AI enhancement */
  isUtilityConfigured?: boolean;
  /** Callback to open file picker for attaching files */
  onAttachFiles?: () => void;
  /** Hide the sessions dropdown (for quick chat) */
  hideSessionsDropdown?: boolean;
  /** When true, only render the submit/cancel button — no other controls */
  minimalToolbar?: boolean;
  /** Hide the plan mode toggle button (for ephemeral/quick chat sessions) */
  hidePlanMode?: boolean;
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
          data-testid="plan-mode-toggle-button"
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

function EnhancePromptButton({
  onClick,
  isLoading,
  isConfigured = true,
}: {
  onClick: () => void;
  isLoading: boolean;
  isConfigured?: boolean;
}) {
  const isDisabled = !isConfigured || isLoading;
  const tooltipText = isConfigured
    ? "Enhance prompt with AI"
    : "Configure a utility agent in settings to enable AI enhancement";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {/* Wrap in span so tooltip works even when button is disabled */}
        <span className="inline-flex">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-7 w-7 cursor-pointer hover:bg-muted/40 text-slate-400"
            onClick={isConfigured ? onClick : undefined}
            disabled={isDisabled}
            aria-label="Enhance prompt with AI"
            aria-busy={isLoading}
          >
            {isLoading ? <GridSpinner className="h-4 w-4" /> : <IconSparkles className="h-4 w-4" />}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltipText}</TooltipContent>
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

type ToolbarItemConfig = {
  id: string;
  collapsible: boolean;
  section: "left" | "right";
  render: () => ReactNode;
  visible?: boolean;
};

function ToolbarExpandToggle({
  isExpanded,
  onToggle,
}: {
  isExpanded: boolean;
  onToggle: () => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={isExpanded ? "Collapse toolbar" : "More toolbar actions"}
          className="h-7 w-7 cursor-pointer hover:bg-muted/40"
          data-testid="toolbar-overflow-menu"
          onClick={onToggle}
        >
          {isExpanded ? <IconChevronsLeft className="h-4 w-4" /> : <IconDots className="h-4 w-4" />}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{isExpanded ? "Collapse" : "More actions"}</TooltipContent>
    </Tooltip>
  );
}

function ToolbarRightSection({
  showCollapsed,
  rightItems,
  sessionId,
  planModeEnabled,
  isAgentBusy,
  onImplementPlan,
  isDisabled,
  isSending,
  onCancel,
  onSubmit,
  submitShortcut,
}: {
  showCollapsed: boolean;
  rightItems: ToolbarItemConfig[];
  sessionId: string | null;
  planModeEnabled: boolean;
  isAgentBusy: boolean;
  onImplementPlan?: () => void;
  isDisabled: boolean;
  isSending: boolean;
  onCancel: () => void;
  onSubmit: () => void;
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
}) {
  return (
    <div className="flex items-center gap-0.5 shrink-0">
      {!showCollapsed && <CollapsibleItems items={rightItems} testIdPrefix="toolbar-item-" />}
      <TokenUsageDisplay sessionId={sessionId} />
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
  );
}

function buildCollapsibleItems(props: {
  mcpServers: string[];
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
  taskDescription: string;
  hideSessionsDropdown?: boolean;
  isAgentBusy: boolean;
  onEnhancePrompt?: () => void;
  isEnhancingPrompt: boolean;
  isUtilityConfigured: boolean;
}): ToolbarItemConfig[] {
  return [
    {
      id: "mcp",
      section: "left",
      collapsible: true,
      render: () => <McpIndicator mcpServers={props.mcpServers} />,
    },
    {
      id: "mode",
      section: "left",
      collapsible: true,
      render: () => <ModeSelector sessionId={props.sessionId} />,
    },
    {
      id: "reset-context",
      section: "right",
      collapsible: true,
      visible: !!props.sessionId && !props.isAgentBusy,
      render: () => <ResetContextButton sessionId={props.sessionId!} />,
    },
    {
      id: "sessions",
      section: "right",
      collapsible: true,
      visible: !props.hideSessionsDropdown,
      render: () => (
        <SessionsDropdown
          taskId={props.taskId}
          activeSessionId={props.sessionId}
          taskTitle={props.taskTitle}
        />
      ),
    },
    {
      id: "model",
      section: "right",
      collapsible: true,
      render: () => <ModelSelector sessionId={props.sessionId} />,
    },
    {
      id: "enhance",
      section: "right",
      collapsible: true,
      visible: !props.isAgentBusy,
      render: () => (
        <EnhancePromptButton
          onClick={props.onEnhancePrompt ?? (() => {})}
          isLoading={props.isEnhancingPrompt}
          isConfigured={props.isUtilityConfigured}
        />
      ),
    },
  ];
}

function CollapsibleItems({
  items,
  testIdPrefix,
}: {
  items: ToolbarItemConfig[];
  testIdPrefix: string;
}) {
  return items.map((i) => (
    <div key={i.id} data-testid={`${testIdPrefix}${i.id}`}>
      {i.render()}
    </div>
  ));
}

function AttachFilesButton({ onClick }: { onClick: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40"
          onClick={onClick}
        >
          <IconPaperclip className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Attach files</TooltipContent>
    </Tooltip>
  );
}

function MinimalToolbar({
  isAgentBusy,
  isDisabled,
  isSending,
  onCancel,
  onSubmit,
  submitKey = "cmd_enter",
}: Pick<
  ChatInputToolbarProps,
  "isAgentBusy" | "isDisabled" | "isSending" | "onCancel" | "onSubmit" | "submitKey"
>) {
  const submitShortcut = submitKey === "enter" ? SHORTCUTS.SUBMIT_ENTER : SHORTCUTS.SUBMIT;
  return (
    <div className="flex items-center justify-end gap-1 px-1 pt-0 pb-0.5 border-t border-border">
      <SubmitButton
        isAgentBusy={isAgentBusy}
        isDisabled={isDisabled}
        isSending={isSending}
        planModeEnabled={false}
        onCancel={onCancel}
        onSubmit={onSubmit}
        submitShortcut={submitShortcut}
      />
    </div>
  );
}

const toolbarDefaults = {
  planModeAvailable: true,
  mcpServers: [] as string[],
  submitKey: "cmd_enter" as const,
  contextCount: 0,
  contextPopoverOpen: false,
  planContextEnabled: false,
  contextFiles: [] as ContextFile[],
  isEnhancingPrompt: false,
  isUtilityConfigured: false,
  hidePlanMode: false,
};

export const ChatInputToolbar = memo(function ChatInputToolbar(rawProps: ChatInputToolbarProps) {
  const props = { ...toolbarDefaults, ...rawProps };
  const submitShortcut = props.submitKey === "enter" ? SHORTCUTS.SUBMIT_ENTER : SHORTCUTS.SUBMIT;
  const toolbarRef = useRef<HTMLDivElement>(null);
  const isCollapsed = useToolbarCollapsed(toolbarRef);
  const [isExpanded, setIsExpanded] = useState(false);
  const showCollapsed = isCollapsed && !isExpanded;

  if (props.minimalToolbar) {
    return (
      <MinimalToolbar
        isAgentBusy={props.isAgentBusy}
        isDisabled={props.isDisabled}
        isSending={props.isSending}
        onCancel={props.onCancel}
        onSubmit={props.onSubmit}
        submitKey={props.submitKey}
      />
    );
  }

  const items = buildCollapsibleItems(props);
  const leftItems = items.filter((i) => i.section === "left" && i.visible !== false);
  const rightItems = items.filter((i) => i.section === "right" && i.visible !== false);

  return (
    <div
      ref={toolbarRef}
      data-testid="chat-input-toolbar"
      className={cn(
        "flex items-center gap-1 px-1 pt-0 pb-0.5 border-t border-border",
        isCollapsed ? "overflow-x-auto scrollbar-hide" : "overflow-visible",
      )}
    >
      <div className="flex items-center gap-0.5 shrink-0">
        {!props.hidePlanMode && (
          <PlanToggleButton
            planModeEnabled={props.planModeEnabled}
            planModeAvailable={props.planModeAvailable}
            onPlanModeChange={props.onPlanModeChange}
          />
        )}
        {!showCollapsed && <CollapsibleItems items={leftItems} testIdPrefix="toolbar-item-" />}
        {props.onAttachFiles && <AttachFilesButton onClick={props.onAttachFiles} />}
        <ContextPopover
          open={props.contextPopoverOpen}
          onOpenChange={props.onContextPopoverOpenChange ?? (() => {})}
          trigger={
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 relative"
            >
              <IconAt className="h-4 w-4" />
              {props.contextCount > 0 && !isCollapsed && (
                <span className="absolute -top-1 -right-1 h-4 min-w-4 rounded-full bg-muted-foreground/80 text-[10px] text-background flex items-center justify-center px-0.5 pointer-events-none">
                  {props.contextCount}
                </span>
              )}
            </Button>
          }
          sessionId={props.sessionId}
          planContextEnabled={props.planContextEnabled}
          contextFiles={props.contextFiles}
          onToggleFile={props.onToggleFile ?? (() => {})}
        />
        {isCollapsed && (
          <ToolbarExpandToggle isExpanded={isExpanded} onToggle={() => setIsExpanded((v) => !v)} />
        )}
      </div>

      <div className="flex-1" />

      <ToolbarRightSection
        showCollapsed={showCollapsed}
        rightItems={rightItems}
        sessionId={props.sessionId}
        planModeEnabled={props.planModeEnabled}
        isAgentBusy={props.isAgentBusy}
        onImplementPlan={props.onImplementPlan}
        isDisabled={props.isDisabled}
        isSending={props.isSending}
        onCancel={props.onCancel}
        onSubmit={props.onSubmit}
        submitShortcut={submitShortcut}
      />
    </div>
  );
});
