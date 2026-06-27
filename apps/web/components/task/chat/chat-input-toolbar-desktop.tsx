"use client";

import { useRef, useState, type ReactNode } from "react";
import { IconAt, IconChevronsLeft, IconDots } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { SessionsDropdown } from "@/components/task/sessions-dropdown";
import { ModelSelector } from "@/components/task/model-selector";
import { ModeSelector } from "@/components/task/mode-selector";
import { TokenUsageDisplay } from "@/components/task/chat/token-usage-display";
import { useToolbarCollapsed } from "@/hooks/use-toolbar-collapsed";
import { cn } from "@/lib/utils";
import { ResetContextButton } from "./reset-context-button";
import { ImplementPlanButton } from "./implement-plan-button";
import { VoiceInputButton } from "./voice-input-button";
import { ContextPopover } from "./context-popover";
import {
  AttachFilesButton,
  McpIndicator,
  PlanToggleButton,
  SubmitButton,
} from "./chat-input-toolbar-primitives";
import { type ChatInputToolbarProps } from "./chat-input-toolbar";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { SHORTCUTS } from "@/lib/keyboard/constants";

type ToolbarItemConfig = {
  id: string;
  section: "left" | "right";
  render: () => ReactNode;
  visible?: boolean;
};

type DesktopToolbarProps = ChatInputToolbarProps & {
  planModeAvailable: boolean;
  mcpServers: string[];
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
  contextCount: number;
  contextPopoverOpen: boolean;
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  isEnhancingPrompt: boolean;
  isUtilityConfigured: boolean;
  hideAgentControls: boolean;
  hidePlanMode: boolean;
};

function ToolbarExpandToggle(props: { isExpanded: boolean; onToggle: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={props.isExpanded ? "Collapse toolbar" : "More toolbar actions"}
          aria-expanded={props.isExpanded}
          className="h-7 w-7 cursor-pointer hover:bg-muted/40"
          data-testid="toolbar-overflow-menu"
          onClick={props.onToggle}
        >
          {props.isExpanded ? (
            <IconChevronsLeft className="h-4 w-4" />
          ) : (
            <IconDots className="h-4 w-4" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{props.isExpanded ? "Collapse" : "More actions"}</TooltipContent>
    </Tooltip>
  );
}

function CollapsibleItems(props: { items: ToolbarItemConfig[]; testIdPrefix: string }) {
  return props.items.map((i) => (
    <div key={i.id} data-testid={`${props.testIdPrefix}${i.id}`}>
      {i.render()}
    </div>
  ));
}

function buildCollapsibleItems(props: DesktopToolbarProps): ToolbarItemConfig[] {
  if (props.hideAgentControls) return [];
  return [
    {
      id: "mcp",
      section: "left",
      render: () => <McpIndicator mcpServers={props.mcpServers} />,
    },
    {
      id: "mode",
      section: "left",
      render: () => <ModeSelector sessionId={props.sessionId} />,
    },
    {
      id: "reset-context",
      section: "right",
      visible: !!props.sessionId && !props.isAgentBusy,
      render: () => <ResetContextButton sessionId={props.sessionId!} />,
    },
    {
      id: "sessions",
      section: "right",
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
      render: () => <ModelSelector sessionId={props.sessionId} />,
    },
    {
      id: "enhance",
      section: "right",
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

function DesktopRightSection(props: {
  showCollapsed: boolean;
  rightItems: ToolbarItemConfig[];
  sessionId: string | null;
  planModeEnabled: boolean;
  isAgentBusy: boolean;
  hasContent: boolean;
  onImplementPlan?: (fresh: boolean) => void;
  isDisabled: boolean;
  submitDisabledReason?: string;
  isSending: boolean;
  onCancel: () => void | Promise<void>;
  onSubmit: () => void;
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
  onVoiceTranscript?: (text: string) => void;
  onVoiceAutoSend?: () => void;
}) {
  return (
    <div className="flex items-center gap-0.5 shrink-0">
      {!props.showCollapsed && (
        <CollapsibleItems items={props.rightItems} testIdPrefix="toolbar-item-" />
      )}
      <TokenUsageDisplay sessionId={props.sessionId} />
      {props.planModeEnabled && !props.isAgentBusy && props.onImplementPlan && (
        <ImplementPlanButton onClick={props.onImplementPlan} />
      )}
      <div className="ml-1 flex items-center gap-1">
        {props.onVoiceTranscript && (
          <VoiceInputButton
            onTranscript={props.onVoiceTranscript}
            onAutoSend={props.onVoiceAutoSend}
            disabled={props.isDisabled}
          />
        )}
        <SubmitButton
          isAgentBusy={props.isAgentBusy}
          hasContent={props.hasContent}
          isDisabled={props.isDisabled}
          submitDisabledReason={props.submitDisabledReason}
          isSending={props.isSending}
          planModeEnabled={props.planModeEnabled}
          onCancel={props.onCancel}
          onSubmit={props.onSubmit}
          submitShortcut={props.submitShortcut}
        />
      </div>
    </div>
  );
}

export function DesktopChatInputToolbar(props: DesktopToolbarProps) {
  const toolbarRef = useRef<HTMLDivElement>(null);
  const isCollapsed = useToolbarCollapsed(toolbarRef);
  const [isExpanded, setIsExpanded] = useState(false);
  const showCollapsed = isCollapsed && !isExpanded;
  const items = buildCollapsibleItems(props);
  const leftItems = items.filter((i) => i.section === "left" && i.visible !== false);
  const rightItems = items.filter((i) => i.section === "right" && i.visible !== false);

  return (
    <div
      ref={toolbarRef}
      data-testid="chat-input-toolbar"
      className={cn(
        "hidden items-center gap-1 px-1 pt-0 pb-0.5 border-t border-border md:flex",
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
              data-testid="chat-context-button"
              aria-label="Session context"
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

      <DesktopRightSection
        showCollapsed={showCollapsed}
        rightItems={rightItems}
        sessionId={props.sessionId}
        planModeEnabled={props.planModeEnabled}
        isAgentBusy={props.isAgentBusy}
        hasContent={props.hasContent ?? false}
        onImplementPlan={props.onImplementPlan}
        isDisabled={props.isDisabled}
        submitDisabledReason={props.submitDisabledReason}
        isSending={props.isSending}
        onCancel={props.onCancel}
        onSubmit={props.onSubmit}
        submitShortcut={props.submitShortcut}
        onVoiceTranscript={props.onVoiceTranscript}
        onVoiceAutoSend={props.onVoiceAutoSend}
      />
    </div>
  );
}
