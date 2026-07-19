"use client";

import { IconAt } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { ModelSelector } from "@/components/task/model-selector";
import { ModeSelector } from "@/components/task/mode-selector";
import { SessionsDropdown } from "@/components/task/sessions-dropdown";
import { TokenUsageDisplay } from "@/components/task/chat/token-usage-display";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { VoiceInputButton } from "./voice-input-button";
import { ChatInputPluginActions } from "./chat-input-plugin-actions";
import { ContextPopover } from "./context-popover";
import { ImplementPlanButton } from "./implement-plan-button";
import { ResetContextButton } from "./reset-context-button";
import {
  AttachFilesButton,
  McpIndicator,
  PlanToggleButton,
  SubmitButton,
} from "./chat-input-toolbar-primitives";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { SHORTCUTS } from "@/lib/keyboard/constants";

type MobileToolbarProps = {
  planModeEnabled: boolean;
  planModeAvailable: boolean;
  onPlanModeChange: (enabled: boolean) => void;
  hidePlanMode: boolean;
  hideAgentControls: boolean;
  hideSessionsDropdown: boolean;
  mcpServers: string[];
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
  onAttachFiles?: () => void;
  contextPopoverOpen: boolean;
  onContextPopoverOpenChange: (open: boolean) => void;
  contextCount: number;
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  onToggleFile: (file: ContextFile) => void;
  isAgentBusy: boolean;
  hasContent: boolean;
  onImplementPlan?: (fresh: boolean) => void;
  onEnhancePrompt?: () => void;
  isEnhancingPrompt: boolean;
  isUtilityConfigured: boolean;
  isDisabled: boolean;
  submitDisabledReason?: string;
  isSending: boolean;
  onCancel: () => void | Promise<void>;
  onSubmit: () => void;
  submitShortcut: (typeof SHORTCUTS)[keyof typeof SHORTCUTS];
  onVoiceTranscript?: (text: string) => void;
  onVoiceAutoSend?: () => void;
};

function mobileContextButton(contextCount: number) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 relative"
      data-testid="chat-context-button"
      aria-label="Session context"
    >
      <IconAt className="h-4 w-4" />
      {contextCount > 0 && (
        <span className="absolute -top-1 -right-1 h-4 min-w-4 rounded-full bg-muted-foreground/80 text-[10px] text-background flex items-center justify-center px-0.5 pointer-events-none">
          {contextCount}
        </span>
      )}
    </Button>
  );
}

function MobileLeftActions(props: MobileToolbarProps) {
  return (
    <div className="relative min-w-0 flex-1">
      <div
        data-testid="mobile-chat-toolbar-left-actions"
        className="min-w-0 overflow-x-auto overscroll-x-contain scrollbar-hide pr-8"
      >
        <div className="flex w-max items-center gap-0.5 pr-3">
          {!props.hidePlanMode && (
            <PlanToggleButton
              planModeEnabled={props.planModeEnabled}
              planModeAvailable={props.planModeAvailable}
              onPlanModeChange={props.onPlanModeChange}
            />
          )}
          {!props.hideAgentControls && (
            <>
              <div data-testid="toolbar-item-mcp">
                <McpIndicator mcpServers={props.mcpServers} />
              </div>
              <div data-testid="toolbar-item-mode">
                <ModeSelector sessionId={props.sessionId} triggerClassName="max-w-[46vw]" />
              </div>
              <div data-testid="toolbar-item-model">
                <ModelSelector
                  sessionId={props.sessionId}
                  triggerClassName="max-w-[56vw] min-w-0 overflow-hidden"
                />
              </div>
              {!props.hideSessionsDropdown && (
                <div data-testid="toolbar-item-sessions">
                  <SessionsDropdown
                    taskId={props.taskId}
                    activeSessionId={props.sessionId}
                    taskTitle={props.taskTitle}
                  />
                </div>
              )}
            </>
          )}
          {props.onAttachFiles && <AttachFilesButton onClick={props.onAttachFiles} />}
          <div data-testid="toolbar-item-context">
            <ContextPopover
              open={props.contextPopoverOpen}
              onOpenChange={props.onContextPopoverOpenChange}
              trigger={mobileContextButton(props.contextCount)}
              sessionId={props.sessionId}
              planContextEnabled={props.planContextEnabled}
              contextFiles={props.contextFiles}
              onToggleFile={props.onToggleFile}
            />
          </div>
          {!props.hideAgentControls && props.sessionId && !props.isAgentBusy && (
            <div data-testid="toolbar-item-reset-context">
              <ResetContextButton sessionId={props.sessionId} />
            </div>
          )}
          {!props.hideAgentControls && !props.isAgentBusy && (
            <div data-testid="toolbar-item-enhance">
              <EnhancePromptButton
                onClick={props.onEnhancePrompt ?? (() => {})}
                isLoading={props.isEnhancingPrompt}
                isConfigured={props.isUtilityConfigured}
              />
            </div>
          )}
        </div>
      </div>
      <div
        aria-hidden="true"
        data-testid="mobile-chat-toolbar-scroll-fade"
        className="pointer-events-none absolute inset-y-0 right-0 w-9 bg-gradient-to-l from-background via-background/80 to-transparent"
      />
    </div>
  );
}

export function MobileChatInputToolbar(props: MobileToolbarProps) {
  return (
    <div
      data-testid="mobile-chat-input-toolbar"
      data-legacy-testid="chat-input-toolbar"
      className="flex items-center gap-1 px-1 pt-0 pb-0.5 border-t border-border"
    >
      <MobileLeftActions {...props} />
      <div className="flex shrink-0 items-center gap-1">
        <TokenUsageDisplay sessionId={props.sessionId} />
        {props.planModeEnabled && !props.isAgentBusy && props.onImplementPlan && (
          <ImplementPlanButton onClick={props.onImplementPlan} />
        )}
        <ChatInputPluginActions
          sessionId={props.sessionId}
          taskId={props.taskId}
          taskTitle={props.taskTitle}
        />
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
