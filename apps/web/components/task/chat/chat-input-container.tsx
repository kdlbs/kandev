"use client";

import { forwardRef, useCallback, useState } from "react";
import { IconAlertTriangle, IconPlus, IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import { useAppStore } from "@/components/state-provider";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { Message } from "@/lib/types/http";
import type { DiffComment } from "@/lib/diff/types";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useChatInputContainer } from "./use-chat-input-container";
import {
  ChatInputBody,
  type ChatInputContextAreaProps,
  type ChatInputEditorAreaProps,
} from "./chat-input-body";
import type { ContextItem } from "@/lib/types/context";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";

// Re-export ImageAttachment type for consumers
export type { ImageAttachment } from "./image-attachment-preview";

// Type for message attachments sent to backend
export type MessageAttachment = {
  type: "image" | "audio" | "resource";
  data: string;
  mime_type: string;
  name?: string;
};

export type ChatInputContainerHandle = {
  focusInput: () => void;
  getTextareaElement: () => HTMLElement | null;
  getValue: () => string;
  getSelectionStart: () => number;
  insertText: (text: string, from: number, to: number) => void;
  clear: () => void;
  getAttachments: () => MessageAttachment[];
};

type ChatInputContainerProps = {
  onSubmit: (
    message: string,
    reviewComments?: DiffComment[],
    attachments?: MessageAttachment[],
    inlineMentions?: ContextFile[],
  ) => void;
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
  taskDescription: string;
  planModeEnabled: boolean;
  planModeAvailable?: boolean;
  mcpServers?: string[];
  onPlanModeChange: (enabled: boolean) => void;
  isAgentBusy: boolean;
  isStarting: boolean;
  /** True only while a containerized executor is bootstrapping (Docker
   * prepare, Sprites sandbox spin-up). Distinct from the brief STARTING
   * state every session — including local quick-chat — passes through;
   * see useSessionState. */
  isPreparingEnvironment?: boolean;
  isMoving?: boolean;
  isSending: boolean;
  onCancel: () => void;
  placeholder?: string;
  pendingClarification?: Message | null;
  onClarificationResolved?: () => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  pendingCommentsByFile?: Record<string, DiffComment[]>;
  hasContextComments?: boolean;
  submitKey?: "enter" | "cmd_enter";
  hasAgentCommands?: boolean;
  isFailed?: boolean;
  needsRecovery?: boolean;
  executorUnavailable?: boolean;
  executorUnavailableReason?: string;
  contextItems?: ContextItem[];
  planContextEnabled?: boolean;
  contextFiles?: ContextFile[];
  onToggleContextFile?: (file: ContextFile) => void;
  onAddContextFile?: (file: ContextFile) => void;
  onImplementPlan?: (fresh: boolean) => void;
  hideSessionsDropdown?: boolean;
  minimalToolbar?: boolean;
  /** Hide the plan mode toggle button (for ephemeral/quick chat sessions) */
  hidePlanMode?: boolean;
};

function FailedSessionBanner({
  showDialog,
  onShowDialog,
  taskId,
  sessionId,
  message = "This agent has stopped.",
  detail,
  resumeLabel = "Resume",
  resumingLabel = "Resuming...",
}: {
  showDialog: boolean;
  onShowDialog: (open: boolean) => void;
  taskId: string | null;
  sessionId: string | null;
  message?: string;
  detail?: string;
  resumeLabel?: string;
  resumingLabel?: string;
}) {
  const [isResuming, setIsResuming] = useState(false);

  const agentProfileId = useAppStore((s) =>
    sessionId ? (s.taskSessions.items[sessionId]?.agent_profile_id ?? "") : "",
  );
  const profileExists = useAppStore(
    (s) =>
      agentProfileId !== "" &&
      s.agentProfiles.items.some((p: { id: string }) => p.id === agentProfileId),
  );

  const handleResume = useCallback(async () => {
    if (!sessionId || !taskId) return;
    const client = getWebSocketClient();
    if (!client) return;
    setIsResuming(true);
    try {
      await client.request(
        "session.launch",
        { task_id: taskId, intent: "resume", session_id: sessionId },
        30000,
      );
    } catch {
      setIsResuming(false);
    }
  }, [sessionId, taskId]);

  return (
    <>
      <div className="rounded border border-border overflow-hidden">
        <div className="flex items-center justify-between gap-3 px-4 py-3">
          <div className="flex min-w-0 items-center gap-2 text-sm text-muted-foreground">
            <IconAlertTriangle className="h-4 w-4 text-orange-500 shrink-0" />
            <span className="truncate">{message}</span>
            {detail && <span className="shrink-0 text-xs text-muted-foreground">({detail})</span>}
          </div>
          <div className="flex items-center gap-2">
            {sessionId && taskId && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="inline-flex" data-testid="failed-session-resume-wrapper">
                    <Button
                      variant="default"
                      size="sm"
                      data-testid="failed-session-resume-button"
                      className="shrink-0 gap-1.5 cursor-pointer"
                      onClick={handleResume}
                      disabled={isResuming || !profileExists}
                    >
                      <IconPlayerPlay className="h-3.5 w-3.5" />
                      {isResuming ? resumingLabel : resumeLabel}
                    </Button>
                  </span>
                </TooltipTrigger>
                {!profileExists && <TooltipContent>Agent profile no longer exists</TooltipContent>}
              </Tooltip>
            )}
            <Button
              variant="outline"
              size="sm"
              className="shrink-0 gap-1.5 cursor-pointer"
              onClick={() => onShowDialog(true)}
            >
              <IconPlus className="h-3.5 w-3.5" />
              New Agent
            </Button>
          </div>
        </div>
      </div>
      {taskId && <NewSessionDialog open={showDialog} onOpenChange={onShowDialog} taskId={taskId} />}
    </>
  );
}

type ContainerState = ReturnType<typeof useChatInputContainer>;
type NormalizedChatInputProps = ChatInputContainerProps & {
  isFailed: boolean;
  hasAgentCommands: boolean;
  submitKey: "enter" | "cmd_enter";
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  contextItems: ContextItem[];
  showRequestChangesTooltip: boolean;
};

function normalizeChatInputProps(p: ChatInputContainerProps): NormalizedChatInputProps {
  return {
    ...p,
    isFailed: p.isFailed ?? false,
    hasAgentCommands: p.hasAgentCommands ?? false,
    submitKey: p.submitKey ?? "cmd_enter",
    planContextEnabled: p.planContextEnabled ?? false,
    contextFiles: p.contextFiles ?? [],
    contextItems: p.contextItems ?? [],
    showRequestChangesTooltip: p.showRequestChangesTooltip ?? false,
  };
}

function buildContextAreaProps(
  s: ContainerState,
  p: ChatInputContainerProps,
): ChatInputContextAreaProps {
  return {
    hasContextZone: s.hasContextZone,
    allItems: s.allItems,
    sessionId: p.sessionId,
  };
}

type EnhancePromptExtras = {
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
  isUtilityConfigured?: boolean;
};

function buildEditorAreaProps(
  s: ContainerState,
  p: NormalizedChatInputProps,
  extras: EnhancePromptExtras = {},
): ChatInputEditorAreaProps {
  return {
    inputRef: s.inputRef,
    value: s.value,
    handleChange: s.handleChange,
    handleSubmitWithReset: s.handleSubmitWithReset,
    inputPlaceholder: s.inputPlaceholder,
    isDisabled: s.isDisabled,
    submitDisabled: s.submitDisabled,
    submitDisabledReason: s.submitDisabledReason,
    hasClarification: s.hasClarification,
    planModeEnabled: p.planModeEnabled,
    planModeAvailable: p.planModeAvailable ?? true,
    mcpServers: p.mcpServers ?? [],
    submitKey: p.submitKey,
    setIsInputFocused: s.setIsInputFocused,
    sessionId: p.sessionId,
    taskId: p.taskId,
    onAddContextFile: p.onAddContextFile,
    onToggleContextFile: p.onToggleContextFile,
    planContextEnabled: p.planContextEnabled,
    handleAgentCommand: s.handleAgentCommand,
    addFiles: s.addFiles,
    fileInputRef: s.fileInputRef,
    showRequestChangesTooltip: p.showRequestChangesTooltip,
    isAgentBusy: p.isAgentBusy,
    onPlanModeChange: p.onPlanModeChange,
    taskTitle: p.taskTitle,
    taskDescription: p.taskDescription,
    isSending: p.isSending,
    onCancel: p.onCancel,
    contextCount: s.allItems.length,
    contextPopoverOpen: s.contextPopoverOpen,
    setContextPopoverOpen: s.setContextPopoverOpen,
    contextFiles: p.contextFiles,
    onImplementPlan: p.onImplementPlan,
    onEnhancePrompt: extras.onEnhancePrompt,
    isEnhancingPrompt: extras.isEnhancingPrompt,
    isUtilityConfigured: extras.isUtilityConfigured,
    hideSessionsDropdown: p.hideSessionsDropdown,
    minimalToolbar: p.minimalToolbar,
    hidePlanMode: p.hidePlanMode,
  };
}

function buildStoppedBannerProps(p: ChatInputContainerProps) {
  if (!p.executorUnavailable) return {};
  return {
    message: "Executor environment is unavailable.",
    detail: p.executorUnavailableReason,
    resumeLabel: "Restart",
    resumingLabel: "Restarting...",
  };
}

export const ChatInputContainer = forwardRef<ChatInputContainerHandle, ChatInputContainerProps>(
  function ChatInputContainer(props, ref) {
    const { sessionId, taskId, taskTitle, taskDescription, isAgentBusy, isStarting, isSending } =
      props;
    const p = normalizeChatInputProps(props);
    const isMoving = props.isMoving ?? false;
    const executorUnavailable = props.executorUnavailable ?? false;
    const isBusyVisual = isStarting || isMoving;

    const s = useChatInputContainer({
      ref,
      sessionId,
      isSending,
      isStarting,
      isPreparingEnvironment: props.isPreparingEnvironment ?? false,
      isMoving,
      isFailed: p.isFailed,
      needsRecovery: props.needsRecovery ?? false,
      executorUnavailable,
      isAgentBusy,
      hasAgentCommands: p.hasAgentCommands,
      placeholder: props.placeholder,
      contextItems: p.contextItems,
      pendingClarification: props.pendingClarification,
      onClarificationResolved: props.onClarificationResolved,
      pendingCommentsByFile: props.pendingCommentsByFile,
      hasContextComments: props.hasContextComments ?? false,
      showRequestChangesTooltip: p.showRequestChangesTooltip,
      onRequestChangesTooltipDismiss: props.onRequestChangesTooltipDismiss,
      onSubmit: props.onSubmit,
    });

    const isUtilityConfigured = useIsUtilityConfigured();
    const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({
      sessionId,
      taskTitle,
      taskDescription,
    });

    const handleEnhancePrompt = useCallback(() => {
      const currentValue = s.value?.trim();
      if (!currentValue) return;
      enhancePrompt(currentValue, (enhanced) => {
        // Use setValue to directly update TipTap editor (handleChange only updates React state)
        s.inputRef.current?.setValue(enhanced);
      });
    }, [s, enhancePrompt]);

    if (p.isFailed || executorUnavailable) {
      return (
        <FailedSessionBanner
          showDialog={s.showNewSessionDialog}
          onShowDialog={s.setShowNewSessionDialog}
          taskId={taskId}
          sessionId={sessionId}
          {...buildStoppedBannerProps(props)}
        />
      );
    }

    return (
      <ChatInputBody
        containerRef={s.containerRef}
        height={s.height}
        resizeHandleProps={s.resizeHandleProps}
        isStarting={isBusyVisual}
        isAgentBusy={isAgentBusy}
        hasClarification={s.hasClarification}
        showRequestChangesTooltip={p.showRequestChangesTooltip}
        hasPendingComments={s.hasPendingComments}
        planModeEnabled={props.planModeEnabled}
        showFocusHint={s.showFocusHint}
        needsRecovery={(props.needsRecovery ?? false) || executorUnavailable}
        addFiles={s.addFiles}
        contextAreaProps={buildContextAreaProps(s, p)}
        editorAreaProps={buildEditorAreaProps(s, p, {
          onEnhancePrompt: handleEnhancePrompt,
          isEnhancingPrompt,
          isUtilityConfigured,
        })}
      />
    );
  },
);
