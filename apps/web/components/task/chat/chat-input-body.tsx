"use client";

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { TipTapInput } from "./tiptap-input";
import { ChatInputFocusHint } from "./chat-input-focus-hint";
import { ResizeHandle } from "./resize-handle";
import { TodoSummary } from "./todo-summary";
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

export function ChatInputEditorArea({ inputRef, ...p }: ChatInputEditorAreaProps) {
  const wrappedSubmit = p.isEnhancingPrompt ? () => {} : p.handleSubmitWithReset;
  const inputDisabled = p.isDisabled || p.hasClarification;

  return (
    <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
      <Tooltip open={p.showRequestChangesTooltip}>
        <TooltipTrigger asChild>
          <div
            className={cn(
              "flex-1 min-h-0 transition-opacity",
              p.isEnhancingPrompt && "opacity-50 pointer-events-none",
            )}
          >
            <TipTapInput
              ref={inputRef}
              value={p.value}
              onChange={p.handleChange}
              onSubmit={wrappedSubmit}
              placeholder={p.inputPlaceholder}
              disabled={inputDisabled}
              planModeEnabled={p.planModeEnabled}
              submitKey={p.submitKey}
              onFocus={() => p.setIsInputFocused(true)}
              onBlur={() => p.setIsInputFocused(false)}
              sessionId={p.sessionId}
              taskId={p.taskId}
              onAddContextFile={p.onAddContextFile}
              onToggleContextFile={p.onToggleContextFile}
              planContextEnabled={p.planContextEnabled}
              onAgentCommand={p.handleAgentCommand}
              onImagePaste={p.handleImagePaste}
              onPlanModeChange={p.onPlanModeChange}
            />
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" className="bg-orange-600 text-white border-orange-700">
          <p className="font-medium">Write your changes here</p>
        </TooltipContent>
      </Tooltip>
      <ChatInputToolbar
        planModeEnabled={p.planModeEnabled}
        planModeAvailable={p.planModeAvailable}
        mcpServers={p.mcpServers}
        onPlanModeChange={p.onPlanModeChange}
        sessionId={p.sessionId}
        taskId={p.taskId}
        taskTitle={p.taskTitle}
        taskDescription={p.taskDescription}
        isAgentBusy={p.isAgentBusy}
        isDisabled={p.isDisabled}
        isSending={p.isSending}
        onCancel={p.onCancel}
        onSubmit={wrappedSubmit}
        submitKey={p.submitKey}
        contextCount={p.contextCount}
        contextPopoverOpen={p.contextPopoverOpen}
        onContextPopoverOpenChange={p.setContextPopoverOpen}
        planContextEnabled={p.planContextEnabled}
        contextFiles={p.contextFiles}
        onToggleFile={p.onToggleContextFile}
        onImplementPlan={p.onImplementPlan}
        onEnhancePrompt={p.onEnhancePrompt}
        isEnhancingPrompt={p.isEnhancingPrompt}
        hideSessionsDropdown={p.hideSessionsDropdown}
      />
    </div>
  );
}

export type ChatInputContextAreaProps = {
  hasContextZone: boolean;
  allItems: ContextItem[];
  sessionId: string | null;
  hasTodos: boolean;
  todoItems: TodoItem[];
};

export function ChatInputContextArea({
  hasContextZone,
  allItems,
  sessionId,
  hasTodos,
  todoItems,
}: ChatInputContextAreaProps) {
  if (!hasContextZone) return null;
  const todoSlot = hasTodos ? <TodoSummary todos={todoItems} /> : undefined;
  return <ContextZone items={allItems} sessionId={sessionId} todoSlot={todoSlot} />;
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
  contextAreaProps,
  editorAreaProps,
}: ChatInputBodyProps) {
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
        )}
        style={{ height }}
      >
        <ChatInputFocusHint visible={showFocusHint} />
        <ChatInputContextArea {...contextAreaProps} />
        <ChatInputEditorArea {...editorAreaProps} />
      </div>
    </div>
  );
}
