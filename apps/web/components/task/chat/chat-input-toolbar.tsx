"use client";

import { memo } from "react";

import { SHORTCUTS } from "@/lib/keyboard/constants";
import { DesktopChatInputToolbar } from "./chat-input-toolbar-desktop";
import { MobileChatInputToolbar } from "./chat-input-toolbar-mobile";
import { SubmitButton } from "./chat-input-toolbar-primitives";
import { shouldUseCompactTaskChrome } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
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
  canCancelAgent?: boolean;
  /** Whether the input has content to send (text, comments, or context) */
  hasContent?: boolean;
  isDisabled: boolean;
  submitDisabledReason?: string;
  isSending: boolean;
  onCancel: () => void | Promise<void>;
  onSubmit: () => void;
  submitKey?: "enter" | "cmd_enter";
  contextCount?: number;
  contextPopoverOpen?: boolean;
  onContextPopoverOpenChange?: (open: boolean) => void;
  /** Whether plan is selected as context in the popover (independent of plan panel) */
  planContextEnabled?: boolean;
  contextFiles?: ContextFile[];
  onToggleFile?: (file: ContextFile) => void;
  onImplementPlan?: (fresh: boolean) => void;
  /** Callback to enhance the current prompt with AI */
  onEnhancePrompt?: () => void;
  /** Whether prompt enhancement is in progress */
  isEnhancingPrompt?: boolean;
  /** Whether utility agent is configured for AI enhancement */
  isUtilityConfigured?: boolean;
  /** Callback to open file picker for attaching files */
  onAttachFiles?: () => void;
  /** Callback to insert a transcribed voice utterance into the editor. When
   *  omitted, the voice button is hidden — keeps quick-chat / read-only
   *  variants free of a button they can't wire. */
  onVoiceTranscript?: (text: string) => void;
  /** Optional auto-send hook fired after a voice transcript is inserted. */
  onVoiceAutoSend?: () => void;
  /** Hide the sessions dropdown (for quick chat) */
  hideSessionsDropdown?: boolean;
  /** When true, only render the submit/cancel button — no other controls */
  minimalToolbar?: boolean;
  /** Hide ACP/session-specific controls while keeping Plan, attachment, and context controls */
  hideAgentControls?: boolean;
  /** Hide the plan mode toggle button (for ephemeral/quick chat sessions) */
  hidePlanMode?: boolean;
};

function MinimalToolbar({
  isAgentBusy,
  canCancelAgent,
  hasContent,
  isDisabled,
  submitDisabledReason,
  isSending,
  onCancel,
  onSubmit,
  submitKey = "cmd_enter",
}: Pick<
  ChatInputToolbarProps,
  | "isAgentBusy"
  | "canCancelAgent"
  | "hasContent"
  | "isDisabled"
  | "submitDisabledReason"
  | "isSending"
  | "onCancel"
  | "onSubmit"
  | "submitKey"
>) {
  const submitShortcut = submitKey === "enter" ? SHORTCUTS.SUBMIT_ENTER : SHORTCUTS.SUBMIT;
  return (
    <div className="flex items-center justify-end gap-1 px-1 pt-0 pb-0.5 border-t border-border">
      <SubmitButton
        isAgentBusy={isAgentBusy}
        canCancelAgent={canCancelAgent}
        hasContent={hasContent ?? false}
        isDisabled={isDisabled}
        submitDisabledReason={submitDisabledReason}
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
  hideAgentControls: false,
  hidePlanMode: false,
};

export const ChatInputToolbar = memo(function ChatInputToolbar(rawProps: ChatInputToolbarProps) {
  const props = { ...toolbarDefaults, ...rawProps };
  const submitShortcut = props.submitKey === "enter" ? SHORTCUTS.SUBMIT_ENTER : SHORTCUTS.SUBMIT;
  const responsiveBreakpoint = useResponsiveBreakpoint();
  const usesCompactTaskChrome = shouldUseCompactTaskChrome(responsiveBreakpoint);

  if (props.minimalToolbar) {
    return (
      <MinimalToolbar
        isAgentBusy={props.isAgentBusy}
        canCancelAgent={props.canCancelAgent}
        hasContent={props.hasContent}
        isDisabled={props.isDisabled}
        submitDisabledReason={props.submitDisabledReason}
        isSending={props.isSending}
        onCancel={props.onCancel}
        onSubmit={props.onSubmit}
        submitKey={props.submitKey}
      />
    );
  }

  if (usesCompactTaskChrome) {
    return (
      <MobileChatInputToolbar
        planModeEnabled={props.planModeEnabled}
        planModeAvailable={props.planModeAvailable}
        onPlanModeChange={props.onPlanModeChange}
        hidePlanMode={props.hidePlanMode}
        hideAgentControls={props.hideAgentControls}
        hideSessionsDropdown={
          (props.hideSessionsDropdown ?? false) || responsiveBreakpoint.isMobile
        }
        mcpServers={props.mcpServers}
        sessionId={props.sessionId}
        taskId={props.taskId}
        taskTitle={props.taskTitle}
        onAttachFiles={props.onAttachFiles}
        contextPopoverOpen={props.contextPopoverOpen}
        onContextPopoverOpenChange={props.onContextPopoverOpenChange ?? (() => {})}
        contextCount={props.contextCount}
        planContextEnabled={props.planContextEnabled}
        contextFiles={props.contextFiles}
        onToggleFile={props.onToggleFile ?? (() => {})}
        isAgentBusy={props.isAgentBusy}
        canCancelAgent={props.canCancelAgent}
        hasContent={props.hasContent ?? false}
        onImplementPlan={props.onImplementPlan}
        onEnhancePrompt={props.onEnhancePrompt}
        isEnhancingPrompt={props.isEnhancingPrompt}
        isUtilityConfigured={props.isUtilityConfigured}
        isDisabled={props.isDisabled}
        submitDisabledReason={props.submitDisabledReason}
        isSending={props.isSending}
        onCancel={props.onCancel}
        onSubmit={props.onSubmit}
        submitShortcut={submitShortcut}
        onVoiceTranscript={props.onVoiceTranscript}
        onVoiceAutoSend={props.onVoiceAutoSend}
      />
    );
  }

  const { submitKey: _submitKey, ...desktopProps } = props;
  return <DesktopChatInputToolbar {...desktopProps} submitShortcut={submitShortcut} />;
});
