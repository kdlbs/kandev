"use client";

import { memo, useRef, useState, type ReactNode } from "react";
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
} from "@tabler/icons-react";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { GridSpinner } from "@/components/grid-spinner";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useToolbarCollapsed } from "@/hooks/use-toolbar-collapsed";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { formatShortcut } from "@/lib/keyboard/utils";
import { useAppStore } from "@/components/state-provider";
import { TokenUsageDisplay } from "@/components/task/chat/token-usage-display";
import { SessionsDropdown } from "@/components/task/sessions-dropdown";
import { ModelSelector } from "@/components/task/model-selector";
import { ModeSelector } from "@/components/task/mode-selector";
import { ContextPopover } from "./context-popover";
import { ResetContextButton } from "./reset-context-button";
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
  /** Whether the input has content to send (text, comments, or context) */
  hasContent?: boolean;
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
  hasContent: boolean;
  isDisabled: boolean;
  isSending: boolean;
  planModeEnabled: boolean;
  onCancel: () => void;
  onSubmit: () => void;
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
};

function submitTooltipDescription(isAgentBusy: boolean, planModeEnabled: boolean) {
  if (isAgentBusy) return "Queue message";
  if (planModeEnabled) return "Request plan changes";
  return undefined;
}

function SubmitButton({
  isAgentBusy,
  hasContent,
  isDisabled,
  isSending,
  planModeEnabled,
  onCancel,
  onSubmit,
  submitShortcut,
}: SubmitButtonProps) {
  // When agent is busy and there's nothing to send, show only the cancel button.
  // When there's content to queue, show both cancel and send buttons side-by-side.
  const showSendButton = !isAgentBusy || hasContent;
  return (
    <div className="flex items-center gap-1">
      {isAgentBusy && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="secondary"
              size="icon"
              className="h-7 w-7 rounded-full cursor-pointer bg-destructive/10 text-destructive hover:bg-destructive/20"
              onClick={onCancel}
              data-testid="cancel-agent-button"
            >
              <IconPlayerPauseFilled className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Cancel agent</TooltipContent>
        </Tooltip>
      )}
      {showSendButton && (
        <KeyboardShortcutTooltip
          shortcut={submitShortcut}
          description={submitTooltipDescription(isAgentBusy, planModeEnabled)}
          enabled={!isDisabled}
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
        </KeyboardShortcutTooltip>
      )}
    </div>
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
  hasContent,
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
  hasContent: boolean;
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
          hasContent={hasContent}
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
  hasContent,
  isDisabled,
  isSending,
  onCancel,
  onSubmit,
  submitKey = "cmd_enter",
}: Pick<
  ChatInputToolbarProps,
  "isAgentBusy" | "hasContent" | "isDisabled" | "isSending" | "onCancel" | "onSubmit" | "submitKey"
>) {
  const submitShortcut = submitKey === "enter" ? SHORTCUTS.SUBMIT_ENTER : SHORTCUTS.SUBMIT;
  return (
    <div className="flex items-center justify-end gap-1 px-1 pt-0 pb-0.5 border-t border-border">
      <SubmitButton
        isAgentBusy={isAgentBusy}
        hasContent={hasContent ?? false}
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
        hasContent={props.hasContent}
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
        hasContent={props.hasContent ?? false}
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
