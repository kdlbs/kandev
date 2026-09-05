"use client";

import { useState } from "react";
import { IconAt } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { ModelSelector } from "@/components/task/model-selector";
import { ModeSelector } from "@/components/task/mode-selector";
import { SessionsDropdown } from "@/components/task/sessions-dropdown";
import { TokenUsageDisplay } from "@/components/task/chat/token-usage-display";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { ChatInputPluginActions } from "./chat-input-plugin-actions";
import { ContextPopover } from "./context-popover";
import { ImplementPlanButton } from "./implement-plan-button";
import { ResetContextButton } from "./reset-context-button";
import { AttachFilesButton, PlanToggleButton, SubmitButton } from "./chat-input-toolbar-primitives";
import { McpIndicator } from "./mcp-explorer/mcp-indicator";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { SHORTCUTS } from "@/lib/keyboard/constants";
import type { MCPAttachmentHistory } from "@/lib/state/slices/session-runtime/types";
import type { PluginComposerCapability } from "@/lib/plugins/types";
import { t } from "@/lib/i18n";

type MobileToolbarProps = {
  planModeEnabled: boolean;
  planModeAvailable: boolean;
  onPlanModeChange: (enabled: boolean) => void;
  hidePlanMode: boolean;
  hideAgentControls: boolean;
  hideSessionsDropdown: boolean;
  mcpServers: string[];
  mcpAttachmentHistory?: MCPAttachmentHistory;
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
  canCancelAgent?: boolean;
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
  composerCapability?: PluginComposerCapability;
  composerSurface?: "task-chat" | "quick-chat";
  presentation?: "desktop" | "mobile";
};

type MobileLeftActionsProps = MobileToolbarProps & {
  resetConfirmationOpen: boolean;
  onResetConfirmationOpenChange: (open: boolean) => void;
};

function mobileContextButton(contextCount: number, presentation: "desktop" | "mobile") {
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className={`${presentation === "mobile" ? "min-h-11 min-w-11" : "h-7"} gap-1.5 px-2 cursor-pointer hover:bg-muted/40 relative`}
      data-testid="chat-context-button"
      aria-label={t("task:sessionContext")}
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

function MobileDefaultLeftActions(props: MobileToolbarProps) {
  const presentation = props.presentation ?? "mobile";
  return (
    <>
      {!props.hidePlanMode && (
        <PlanToggleButton
          planModeEnabled={props.planModeEnabled}
          planModeAvailable={props.planModeAvailable}
          onPlanModeChange={props.onPlanModeChange}
          presentation={presentation}
        />
      )}
      {!props.hideAgentControls && (
        <>
          <div data-testid="toolbar-item-mcp">
            <McpIndicator
              mcpServers={props.mcpServers}
              attachmentHistory={props.mcpAttachmentHistory}
            />
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
      {props.onAttachFiles && (
        <AttachFilesButton onClick={props.onAttachFiles} presentation={presentation} />
      )}
      <div data-testid="toolbar-item-context">
        <ContextPopover
          open={props.contextPopoverOpen}
          onOpenChange={props.onContextPopoverOpenChange}
          trigger={mobileContextButton(props.contextCount, presentation)}
          sessionId={props.sessionId}
          planContextEnabled={props.planContextEnabled}
          contextFiles={props.contextFiles}
          onToggleFile={props.onToggleFile}
        />
      </div>
    </>
  );
}

function MobileLeftActions(props: MobileLeftActionsProps) {
  return (
    <div className="relative min-w-0 flex-1">
      <div
        data-testid="mobile-chat-toolbar-left-actions"
        className={
          props.resetConfirmationOpen
            ? "min-w-0"
            : "min-w-0 overflow-x-auto overscroll-x-contain scrollbar-hide pr-8"
        }
      >
        <div
          className={
            props.resetConfirmationOpen
              ? "flex w-full min-w-0 items-center"
              : "flex w-max items-center gap-0.5 pr-3"
          }
        >
          {!props.resetConfirmationOpen ? <MobileDefaultLeftActions {...props} /> : null}
          {!props.hideAgentControls && props.sessionId && !props.isAgentBusy && (
            <div
              key="reset-context"
              data-testid="toolbar-item-reset-context"
              className={props.resetConfirmationOpen ? "w-full min-w-0" : undefined}
            >
              <ResetContextButton
                sessionId={props.sessionId}
                presentation="mobile"
                onConfirmationOpenChange={props.onResetConfirmationOpenChange}
              />
            </div>
          )}
          {!props.resetConfirmationOpen && !props.hideAgentControls && !props.isAgentBusy && (
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
      {!props.resetConfirmationOpen ? (
        <div
          aria-hidden="true"
          data-testid="mobile-chat-toolbar-scroll-fade"
          className="pointer-events-none absolute inset-y-0 right-0 w-9 bg-gradient-to-l from-background via-background/80 to-transparent"
        />
      ) : null}
    </div>
  );
}

export function MobileChatInputToolbar(props: MobileToolbarProps) {
  const [resetConfirmationOpen, setResetConfirmationOpen] = useState(false);
  const presentation = props.presentation ?? "mobile";

  return (
    <div
      data-testid="mobile-chat-input-toolbar"
      data-legacy-testid="chat-input-toolbar"
      className={`flex items-center gap-1 px-1 pt-0 pb-0.5 border-t border-border ${
        resetConfirmationOpen ? "relative z-10 bg-background" : ""
      }`}
    >
      <MobileLeftActions
        {...props}
        resetConfirmationOpen={resetConfirmationOpen}
        onResetConfirmationOpenChange={setResetConfirmationOpen}
      />
      {!resetConfirmationOpen ? (
        <div className="flex shrink-0 items-center gap-1">
          <TokenUsageDisplay sessionId={props.sessionId} />
          {props.planModeEnabled && !props.isAgentBusy && props.onImplementPlan && (
            <ImplementPlanButton onClick={props.onImplementPlan} presentation={presentation} />
          )}
          {!props.hideAgentControls && (
            <ChatInputPluginActions
              sessionId={props.sessionId}
              taskId={props.taskId}
              taskTitle={props.taskTitle}
              surface={props.composerSurface ?? (props.taskId ? "task-chat" : "quick-chat")}
              presentation="mobile"
              disabled={props.isDisabled}
              submittable={!props.isDisabled && props.hasContent}
              disabledReason={props.submitDisabledReason}
              composer={props.composerCapability}
            />
          )}
          <SubmitButton
            isAgentBusy={props.isAgentBusy}
            canCancelAgent={props.canCancelAgent}
            sessionId={props.sessionId}
            hasContent={props.hasContent}
            isDisabled={props.isDisabled}
            submitDisabledReason={props.submitDisabledReason}
            isSending={props.isSending}
            planModeEnabled={props.planModeEnabled}
            onCancel={props.onCancel}
            onSubmit={props.onSubmit}
            submitShortcut={props.submitShortcut}
            presentation={presentation}
          />
        </div>
      ) : null}
    </div>
  );
}
