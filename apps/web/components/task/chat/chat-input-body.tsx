"use client";

import { useState, useCallback } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { TipTapInput } from "./tiptap-input";
import { ChatInputFocusHint } from "./chat-input-focus-hint";
import { ResizeHandle } from "./resize-handle";
import { ChatInputToolbar } from "./chat-input-toolbar";
import { ContextZone } from "./context-items/context-zone";
import type { ContextItem } from "@/lib/types/context";
import type { ContextFile } from "@/lib/state/context-files-store";

export type ChatInputEditorAreaProps = {
  inputRef: React.RefObject<import("./tiptap-input").TipTapInputHandle | null>;
  value: string;
  handleChange: (val: string) => void;
  handleSubmitWithReset: () => void;
  inputPlaceholder: string;
  isDisabled: boolean;
  hasClarification: boolean;
  planModeEnabled: boolean;
  planModeAvailable: boolean;
  mcpServers: string[];
  submitKey: "enter" | "cmd_enter";
  setIsInputFocused: (focused: boolean) => void;
  sessionId: string | null;
  taskId: string | null;
  onAddContextFile?: (file: ContextFile) => void;
  onToggleContextFile?: (file: ContextFile) => void;
  planContextEnabled: boolean;
  handleAgentCommand: (command: string) => void;
  addFiles: (files: File[]) => Promise<void>;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  showRequestChangesTooltip: boolean;
  hideSessionsDropdown?: boolean;
  isAgentBusy: boolean;
  onPlanModeChange: (enabled: boolean) => void;
  taskTitle?: string;
  taskDescription: string;
  isSending: boolean;
  onCancel: () => void;
  contextCount: number;
  contextPopoverOpen: boolean;
  setContextPopoverOpen: (open: boolean) => void;
  contextFiles: ContextFile[];
  onImplementPlan?: () => void;
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
};

function EditorWithTooltip({
  showTooltip,
  isEnhancingPrompt,
  children,
}: {
  showTooltip: boolean;
  isEnhancingPrompt?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Tooltip open={showTooltip}>
      <TooltipTrigger asChild>
        <div
          className={cn(
            "flex-1 min-h-0 transition-opacity",
            isEnhancingPrompt && "opacity-50 pointer-events-none",
          )}
        >
          {children}
        </div>
      </TooltipTrigger>
      <TooltipContent side="top" className="bg-orange-600 text-white border-orange-700">
        <p className="font-medium">Write your changes here</p>
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

export function ChatInputEditorArea({
  inputRef,
  value,
  handleChange,
  handleSubmitWithReset,
  inputPlaceholder,
  isDisabled,
  hasClarification,
  planModeEnabled,
  planModeAvailable,
  mcpServers,
  submitKey,
  setIsInputFocused,
  sessionId,
  taskId,
  onAddContextFile,
  onToggleContextFile,
  planContextEnabled,
  handleAgentCommand,
  addFiles,
  fileInputRef,
  showRequestChangesTooltip,
  isAgentBusy,
  onPlanModeChange,
  taskTitle,
  taskDescription,
  isSending,
  onCancel,
  contextCount,
  contextPopoverOpen,
  setContextPopoverOpen,
  contextFiles,
  onImplementPlan,
  onEnhancePrompt,
  isEnhancingPrompt,
  hideSessionsDropdown,
}: ChatInputEditorAreaProps) {
  // Block submit while enhancing prompt, but keep editor editable for programmatic updates
  const wrappedSubmit = isEnhancingPrompt ? () => {} : handleSubmitWithReset;

  const handleAttachFiles = useCallback(() => {
    fileInputRef.current?.click();
  }, [fileInputRef]);

  return (
    <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
      <EditorWithTooltip
        showTooltip={showRequestChangesTooltip}
        isEnhancingPrompt={isEnhancingPrompt}
      >
        <TipTapInput
          ref={inputRef}
          value={value}
          onChange={handleChange}
          onSubmit={wrappedSubmit}
          placeholder={inputPlaceholder}
          disabled={isDisabled || hasClarification}
          planModeEnabled={planModeEnabled}
          submitKey={submitKey}
          onFocus={() => setIsInputFocused(true)}
          onBlur={() => setIsInputFocused(false)}
          sessionId={sessionId}
          taskId={taskId}
          onAddContextFile={onAddContextFile}
          onToggleContextFile={onToggleContextFile}
          planContextEnabled={planContextEnabled}
          onAgentCommand={handleAgentCommand}
          onImagePaste={addFiles}
          onPlanModeChange={onPlanModeChange}
        />
      </EditorWithTooltip>
      <FileInput fileInputRef={fileInputRef} addFiles={addFiles} />
      <ChatInputToolbar
        planModeEnabled={planModeEnabled}
        planModeAvailable={planModeAvailable}
        mcpServers={mcpServers}
        onPlanModeChange={onPlanModeChange}
        sessionId={sessionId}
        taskId={taskId}
        taskTitle={taskTitle}
        taskDescription={taskDescription}
        isAgentBusy={isAgentBusy}
        isDisabled={isDisabled}
        isSending={isSending}
        onCancel={onCancel}
        onSubmit={wrappedSubmit}
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
        onAttachFiles={handleAttachFiles}
        hideSessionsDropdown={hideSessionsDropdown}
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
};

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
}: ChatInputBodyProps) {
  const [isDragging, setIsDragging] = useState(false);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer.types.includes("Files")) {
      setIsDragging(true);
    }
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    // Only set false when leaving the container (not entering a child)
    const rect = e.currentTarget.getBoundingClientRect();
    const { clientX, clientY } = e;
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
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDragging(false);
      const files = Array.from(e.dataTransfer.files).filter((f) => f.size > 0 || f.type !== "");
      if (files.length > 0) {
        void addFiles(files);
      }
    },
    [addFiles],
  );

  return (
    <div className="relative">
      <ResizeHandle
        planModeEnabled={planModeEnabled}
        isAgentBusy={isAgentBusy}
        isStarting={isStarting}
        {...resizeHandleProps}
      />
      <div
        ref={containerRef}
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
          isDragging && "border-primary ring-1 ring-primary/30",
        )}
        style={{ height }}
        onDragOver={handleDragOver}
        onDragEnter={handleDragEnter}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <ChatInputFocusHint visible={showFocusHint} />
        <ChatInputContextArea {...contextAreaProps} />
        <ChatInputEditorArea {...editorAreaProps} />
      </div>
    </div>
  );
}
