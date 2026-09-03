"use client";

import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { CompositorPulse } from "@kandev/ui/compositor-pulse";
import { cn } from "@/lib/utils";
import { TipTapInput } from "./tiptap-input";
import { ChatInputFocusHint } from "./chat-input-focus-hint";
import { ResizeHandle } from "./resize-handle";
import { ChatInputToolbar } from "./chat-input-toolbar";
import { ContextZone } from "./context-items/context-zone";
import type { ContextItem } from "@/lib/types/context";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { ImagePasteIssue } from "./clipboard-attachments";
import type { MCPAttachmentHistory } from "@/lib/state/slices/session-runtime/types";
import {
  composerIdentity,
  composerInsertionText,
  useStablePluginComposerCapability,
} from "@/lib/plugins/composer-capability";
import type { PluginComposerCapability } from "@/lib/plugins/types";

export type ChatInputEditorAreaProps = {
  inputRef: React.RefObject<import("./tiptap-input").TipTapInputHandle | null>;
  value: string;
  handleChange: (val: string) => void;
  handleSubmitWithReset: () => void;
  inputPlaceholder: string;
  isDisabled: boolean;
  submitDisabled: boolean;
  submitDisabledReason?: string;
  hasPendingAttachmentUploads: boolean;
  planModeEnabled: boolean;
  planModeAvailable: boolean;
  mcpServers: string[];
  mcpAttachmentHistory?: MCPAttachmentHistory;
  submitKey: "enter" | "cmd_enter";
  setIsInputFocused: (focused: boolean) => void;
  sessionId: string | null;
  taskId: string | null;
  workspaceId?: string | null;
  entityReferencesEnabled?: boolean;
  onAddContextFile?: (file: ContextFile) => void;
  onToggleContextFile?: (file: ContextFile) => void;
  planContextEnabled: boolean;
  addFiles: (files: File[], issue?: ImagePasteIssue) => Promise<void>;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  showRequestChangesTooltip: boolean;
  hideSessionsDropdown?: boolean;
  minimalToolbar?: boolean;
  hideAgentControls?: boolean;
  hidePlanMode?: boolean;
  isAgentBusy: boolean;
  canCancelAgent?: boolean;
  onPlanModeChange: (enabled: boolean) => void;
  taskTitle?: string;
  taskDescription: string;
  isSending: boolean;
  onCancel: () => void;
  contextCount: number;
  contextPopoverOpen: boolean;
  setContextPopoverOpen: (open: boolean) => void;
  contextFiles: ContextFile[];
  editorClassName?: string;
  onImplementPlan?: (fresh: boolean) => void;
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
  isUtilityConfigured?: boolean;
};

function EditorWithTooltip({
  showTooltip,
  isEnhancingPrompt,
  className,
  children,
}: {
  showTooltip: boolean;
  isEnhancingPrompt?: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip open={showTooltip}>
      <TooltipTrigger asChild>
        <div
          className={cn(
            "flex-1 min-h-0 transition-opacity",
            isEnhancingPrompt && "opacity-50 pointer-events-none",
            className,
          )}
        >
          {children}
        </div>
      </TooltipTrigger>
      <TooltipContent side="top" className="bg-orange-600 text-white border-orange-700">
        <p className="font-medium">{t("task:writeYourChangesHere")}</p>
      </TooltipContent>
    </Tooltip>
  );
}

function FileInput({
  fileInputRef,
  addFiles,
}: {
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  addFiles: (files: File[]) => Promise<void>;
}) {
  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (files && files.length > 0) {
        void addFiles(Array.from(files));
      }
      // Reset so re-selecting the same file triggers onChange
      e.target.value = "";
    },
    [addFiles],
  );

  return (
    <input
      ref={fileInputRef}
      type="file"
      multiple
      className="hidden"
      onChange={handleChange}
      tabIndex={-1}
    />
  );
}

function useChatPluginComposer(p: {
  inputRef: ChatInputEditorAreaProps["inputRef"];
  submitDisabled: boolean;
  isEnhancingPrompt: boolean;
  hasContent: boolean;
  identity: string;
  onSubmit: () => void;
}): PluginComposerCapability {
  return useStablePluginComposerCapability(
    {
      insertText: (text) => {
        const editor = p.inputRef.current;
        if (!editor) return false;
        const insertion = composerInsertionText(text, editor.getCharBefore());
        if (!insertion) return false;
        editor.insertText(insertion, editor.getSelectionStart(), editor.getSelectionEnd());
        editor.focus();
        return true;
      },
      focus: () => {
        if (!p.inputRef.current) return false;
        p.inputRef.current.focus();
        return true;
      },
      // Revalidated at call time against the same gate the slot advertises as
      // `submittable`, so a capability never submits an empty draft it just
      // told the plugin was not submittable.
      //
      // The draft is read from the editor, not from `hasContent`. A plugin may
      // insert a transcript and submit in one callback, and React has not
      // re-rendered by then, so the render snapshot still says "empty" while
      // the editor already holds the text. `submitDraft` reads its own
      // synchronous `valueRef` for the same reason.
      submit: async () => {
        if (p.isEnhancingPrompt || p.submitDisabled) return false;
        const liveDraft = p.inputRef.current?.getValue() ?? "";
        if (liveDraft.trim().length === 0 && !p.hasContent) return false;
        p.onSubmit();
        return true;
      },
    },
    p.identity,
  );
}

export function ChatInputEditorArea(p: ChatInputEditorAreaProps) {
  const { t } = useTranslation("chat");
  const { inputRef, value, handleChange, handleSubmitWithReset, inputPlaceholder } = p;
  const { isDisabled, planModeEnabled, planModeAvailable, mcpServers } = p;
  const { submitKey, setIsInputFocused, sessionId, taskId, planContextEnabled } = p;
  const { onAddContextFile, onToggleContextFile, addFiles, fileInputRef } = p;
  const { showRequestChangesTooltip, isAgentBusy, onPlanModeChange, taskTitle, taskDescription } =
    p;
  const { isSending, onCancel, contextCount, contextPopoverOpen, setContextPopoverOpen } = p;
  const { contextFiles, onImplementPlan, onEnhancePrompt, isEnhancingPrompt } = p;
  const { isUtilityConfigured, hideSessionsDropdown, minimalToolbar, hideAgentControls } = p;
  const { hidePlanMode } = p;
  // Exclude auto-added plan context from the count — it's always present in plan mode
  // and shouldn't by itself enable the send button.
  const userContextCount = planContextEnabled ? Math.max(0, contextCount - 1) : contextCount;
  const hasContent = value.trim().length > 0 || userContextCount > 0;
  // Block submit while enhancing prompt, but keep editor editable for programmatic updates
  const wrappedSubmit = isEnhancingPrompt || p.submitDisabled ? () => {} : handleSubmitWithReset;

  const composerCapability = useChatPluginComposer({
    inputRef,
    submitDisabled: p.submitDisabled,
    isEnhancingPrompt: Boolean(isEnhancingPrompt),
    hasContent,
    // The container is not remounted when the user switches task or session,
    // so this is what tells a captured plugin handle it has been superseded.
    identity: composerIdentity(taskId ? "task-chat" : "quick-chat", taskId, sessionId),
    onSubmit: handleSubmitWithReset,
  });
  const submitDisabledReason = p.hasPendingAttachmentUploads
    ? t("chat:attachmentUploadPendingSubmit")
    : p.submitDisabledReason;
  const handleAttachFiles = useCallback(() => fileInputRef.current?.click(), [fileInputRef]);
  return (
    <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
      <EditorWithTooltip
        showTooltip={showRequestChangesTooltip}
        isEnhancingPrompt={isEnhancingPrompt}
        className={p.editorClassName}
      >
        <TipTapInput
          ref={inputRef}
          value={value}
          onChange={handleChange}
          onSubmit={wrappedSubmit}
          placeholder={inputPlaceholder}
          disabled={isDisabled}
          planModeEnabled={planModeEnabled}
          submitKey={submitKey}
          onFocus={() => setIsInputFocused(true)}
          onBlur={() => setIsInputFocused(false)}
          sessionId={sessionId}
          taskId={taskId}
          workspaceId={p.workspaceId ?? null}
          entityReferencesEnabled={p.entityReferencesEnabled ?? false}
          onAddContextFile={onAddContextFile}
          onToggleContextFile={onToggleContextFile}
          planContextEnabled={planContextEnabled}
          onImagePaste={addFiles}
          onPlanModeChange={onPlanModeChange}
        />
      </EditorWithTooltip>
      <FileInput fileInputRef={fileInputRef} addFiles={addFiles} />
      <ChatInputToolbar
        planModeEnabled={planModeEnabled}
        planModeAvailable={planModeAvailable}
        mcpServers={mcpServers}
        mcpAttachmentHistory={p.mcpAttachmentHistory}
        onPlanModeChange={onPlanModeChange}
        sessionId={sessionId}
        taskId={taskId}
        taskTitle={taskTitle}
        taskDescription={taskDescription}
        isAgentBusy={isAgentBusy}
        canCancelAgent={p.canCancelAgent}
        hasContent={hasContent}
        isDisabled={p.submitDisabled}
        submitDisabledReason={submitDisabledReason}
        isSending={isSending}
        onCancel={onCancel}
        onSubmit={wrappedSubmit}
        composerCapability={composerCapability}
        composerSurface={taskId ? "task-chat" : "quick-chat"}
        submitKey={submitKey}
        contextCount={contextCount}
        contextPopoverOpen={contextPopoverOpen}
        onContextPopoverOpenChange={setContextPopoverOpen}
        planContextEnabled={planContextEnabled}
        contextFiles={contextFiles}
        onToggleFile={onToggleContextFile}
        onImplementPlan={onImplementPlan}
        onEnhancePrompt={onEnhancePrompt}
        isEnhancingPrompt={isEnhancingPrompt}
        isUtilityConfigured={isUtilityConfigured}
        onAttachFiles={handleAttachFiles}
        hideSessionsDropdown={hideSessionsDropdown}
        minimalToolbar={minimalToolbar}
        hideAgentControls={hideAgentControls}
        hidePlanMode={hidePlanMode}
      />
    </div>
  );
}

export type ChatInputContextAreaProps = {
  hasContextZone: boolean;
  allItems: ContextItem[];
  sessionId: string | null;
};

export function ChatInputContextArea({
  hasContextZone,
  allItems,
  sessionId,
}: ChatInputContextAreaProps) {
  if (!hasContextZone) return null;
  return <ContextZone items={allItems} sessionId={sessionId} />;
}

export type ChatInputBodyProps = {
  containerRef: React.RefObject<HTMLDivElement | null>;
  height: React.CSSProperties["height"];
  resizeHandleProps: { onMouseDown: (e: React.MouseEvent) => void; onDoubleClick: () => void };
  isStarting: boolean;
  isAgentBusy: boolean;
  hasClarification: boolean;
  showRequestChangesTooltip: boolean;
  hasPendingComments: boolean;
  planModeEnabled: boolean;
  showFocusHint: boolean;
  needsRecovery: boolean;
  addFiles: (files: File[]) => Promise<void>;
  contextAreaProps: ChatInputContextAreaProps;
  editorAreaProps: ChatInputEditorAreaProps;
  promptResultRecovery?: React.ReactNode;
};

function PromptResultRecoveryArea({ children }: { children?: React.ReactNode }) {
  if (!children) return null;
  return <div className="mt-2">{children}</div>;
}

/** Glow class for the absolute pulse target outside the overflow-hidden box. */
function chatInputGlowClass(isAgentBusy: boolean, isStarting: boolean): string {
  if (isAgentBusy) return "chat-input-glow-running";
  if (isStarting) return "chat-input-glow-starting";
  return "";
}

function ChatInputGlow({ className }: { className: string }) {
  if (!className) return null;
  return (
    <CompositorPulse
      aria-hidden
      data-testid="chat-input-glow"
      className={className}
      minimumOpacity={0.4}
      minimumAtEndpoints
    />
  );
}

function useChatInputDrop(addFiles: (files: File[]) => Promise<void>) {
  const [isDragging, setIsDragging] = useState(false);
  const handleDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
  }, []);
  const handleDragEnter = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    if (event.dataTransfer.types.includes("Files")) setIsDragging(true);
  }, []);
  const handleDragLeave = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    const rect = event.currentTarget.getBoundingClientRect();
    const { clientX, clientY } = event;
    if (
      clientX <= rect.left ||
      clientX >= rect.right ||
      clientY <= rect.top ||
      clientY >= rect.bottom
    ) {
      setIsDragging(false);
    }
  }, []);
  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      event.stopPropagation();
      setIsDragging(false);
      const files = Array.from(event.dataTransfer.files).filter(
        (file) => file.size > 0 || file.type !== "",
      );
      if (files.length > 0) void addFiles(files);
    },
    [addFiles],
  );
  return { handleDragEnter, handleDragLeave, handleDragOver, handleDrop, isDragging };
}

export function ChatInputBody({
  containerRef,
  height,
  resizeHandleProps,
  isStarting,
  isAgentBusy,
  hasClarification,
  showRequestChangesTooltip,
  hasPendingComments,
  planModeEnabled,
  showFocusHint,
  needsRecovery,
  addFiles,
  contextAreaProps,
  editorAreaProps,
  promptResultRecovery,
}: ChatInputBodyProps) {
  const glowClass = chatInputGlowClass(isAgentBusy, isStarting);
  const hasGlow = Boolean(glowClass);
  const drop = useChatInputDrop(addFiles);

  return (
    <div className={cn("relative", { isolate: hasGlow })}>
      <ChatInputGlow className={glowClass} />
      <ResizeHandle
        planModeEnabled={planModeEnabled}
        isAgentBusy={isAgentBusy}
        isStarting={isStarting}
        {...resizeHandleProps}
      />
      <div
        className={cn(
          "flex flex-col overflow-hidden border rounded ",
          "bg-background border-border",
          needsRecovery && "opacity-40 pointer-events-none border-red-500/30",
          isStarting && !isAgentBusy && "chat-input-starting",
          isAgentBusy && !planModeEnabled && "chat-input-running",
          isAgentBusy && planModeEnabled && "chat-input-running-plan",
          planModeEnabled && !isAgentBusy && "border-violet-400/50",
          hasClarification && "border-sky-400/50",
          showRequestChangesTooltip && "animate-pulse border-orange-500",
          hasPendingComments && "border-amber-500/50",
          drop.isDragging && "border-primary ring-1 ring-primary/30",
        )}
        onDragOver={drop.handleDragOver}
        onDragEnter={drop.handleDragEnter}
        onDragLeave={drop.handleDragLeave}
        onDrop={drop.handleDrop}
      >
        <ChatInputFocusHint visible={showFocusHint} />
        <ChatInputContextArea {...contextAreaProps} />
        <div
          ref={containerRef}
          style={{ height }}
          data-testid="chat-input-editor-shell"
          className="flex flex-col min-h-0 overflow-hidden"
        >
          <ChatInputEditorArea
            {...editorAreaProps}
            editorClassName={cn(editorAreaProps.editorClassName, showFocusHint && "pr-28")}
          />
        </div>
      </div>
      <PromptResultRecoveryArea>{promptResultRecovery}</PromptResultRecoveryArea>
    </div>
  );
}
