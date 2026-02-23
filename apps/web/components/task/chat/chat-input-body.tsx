"use client";

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { TipTapInput } from "./tiptap-input";
import { ChatInputFocusHint } from "./chat-input-focus-hint";
import { ResizeHandle } from "./resize-handle";
import { TodoSummary } from "./todo-summary";
import {
  QueuedMessageIndicator,
  type QueuedMessageIndicatorHandle,
} from "./queued-message-indicator";
import { ChatInputToolbar } from "./chat-input-toolbar";
import { ContextZone } from "./context-items/context-zone";
import type { ContextItem } from "@/lib/types/context";
import type { ContextFile } from "@/lib/state/context-files-store";

type TodoItem = { text: string; done?: boolean };

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
  handleImagePaste: (files: File[]) => Promise<void>;
  showRequestChangesTooltip: boolean;
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
};

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
  handleImagePaste,
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
}: ChatInputEditorAreaProps) {
  return (
    <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
      <Tooltip open={showRequestChangesTooltip}>
        <TooltipTrigger asChild>
          <div className="flex-1 min-h-0">
            <TipTapInput
              ref={inputRef}
              value={value}
              onChange={handleChange}
              onSubmit={handleSubmitWithReset}
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
              onImagePaste={handleImagePaste}
              onPlanModeChange={onPlanModeChange}
            />
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" className="bg-orange-600 text-white border-orange-700">
          <p className="font-medium">Write your changes here</p>
        </TooltipContent>
      </Tooltip>
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
        onSubmit={handleSubmitWithReset}
        submitKey={submitKey}
        contextCount={contextCount}
        contextPopoverOpen={contextPopoverOpen}
        onContextPopoverOpenChange={setContextPopoverOpen}
        planContextEnabled={planContextEnabled}
        contextFiles={contextFiles}
        onToggleFile={onToggleContextFile}
        onImplementPlan={onImplementPlan}
      />
    </div>
  );
}

export type ChatInputContextAreaProps = {
  hasContextZone: boolean;
  allItems: ContextItem[];
  sessionId: string | null;
  hasQueuedMessage: boolean;
  queuedMessage?: string | null;
  onCancelQueue?: () => void;
  updateQueueContent?: (content: string) => Promise<void>;
  queuedMessageRef?: React.RefObject<QueuedMessageIndicatorHandle | null>;
  onQueueEditComplete?: () => void;
  hasTodos: boolean;
  todoItems: TodoItem[];
};

export function ChatInputContextArea({
  hasContextZone,
  allItems,
  sessionId,
  hasQueuedMessage,
  queuedMessage,
  onCancelQueue,
  updateQueueContent,
  queuedMessageRef,
  onQueueEditComplete,
  hasTodos,
  todoItems,
}: ChatInputContextAreaProps) {
  if (!hasContextZone) return null;
  const queueSlot = hasQueuedMessage ? (
    <QueuedMessageIndicator
      ref={queuedMessageRef}
      content={queuedMessage!}
      onCancel={onCancelQueue!}
      onUpdate={updateQueueContent!}
      isVisible={true}
      onEditComplete={onQueueEditComplete}
    />
  ) : undefined;
  const todoSlot = hasTodos ? <TodoSummary todos={todoItems} /> : undefined;
  return (
    <ContextZone
      items={allItems}
      sessionId={sessionId}
      queueSlot={queueSlot}
      todoSlot={todoSlot}
    />
  );
}

export type ChatInputBodyProps = {
  containerRef: React.RefObject<HTMLDivElement | null>;
  height: React.CSSProperties["height"];
  resizeHandleProps: { onMouseDown: (e: React.MouseEvent) => void; onDoubleClick: () => void };
  isPanelFocused: boolean | undefined;
  isInputFocused: boolean;
  isAgentBusy: boolean;
  hasClarification: boolean;
  showRequestChangesTooltip: boolean;
  hasPendingComments: boolean;
  planModeEnabled: boolean;
  showFocusHint: boolean;
  contextAreaProps: ChatInputContextAreaProps;
  editorAreaProps: ChatInputEditorAreaProps;
};

export function ChatInputBody({
  containerRef,
  height,
  resizeHandleProps,
  isPanelFocused,
  isInputFocused,
  isAgentBusy,
  hasClarification,
  showRequestChangesTooltip,
  hasPendingComments,
  planModeEnabled,
  showFocusHint,
  contextAreaProps,
  editorAreaProps,
}: ChatInputBodyProps) {
  console.log("[ChatInputBody] planModeEnabled:", planModeEnabled);
  return (
    <div
      ref={containerRef}
      className={cn(
        "relative flex flex-col border rounded ",
        isPanelFocused ? "bg-background border-border" : "bg-background/40 border-border",
        isAgentBusy && !planModeEnabled && "chat-input-running",
        isAgentBusy && planModeEnabled && "chat-input-running-plan",
        planModeEnabled && !isAgentBusy && "border-slate-400/50",
        hasClarification && "border-sky-400/50",
        showRequestChangesTooltip && "animate-pulse border-orange-500",
        hasPendingComments && "border-amber-500/50",
      )}
      style={{ height }}
    >
      <ResizeHandle visible={isPanelFocused || isInputFocused} planModeEnabled={planModeEnabled} {...resizeHandleProps} />
      <ChatInputFocusHint visible={showFocusHint} />
      <ChatInputContextArea {...contextAreaProps} />
      <ChatInputEditorArea {...editorAreaProps} />
    </div>
  );
}
