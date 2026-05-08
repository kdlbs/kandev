"use client";

import { Badge } from "@kandev/ui/badge";
import { Textarea } from "@kandev/ui/textarea";
import { IconGitBranch, IconLoader2 } from "@tabler/icons-react";
import { AgentSelector, ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import type {
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { AttachButton, toContextItems } from "./session-dialog-shared";
import { ContextZone } from "./chat/context-items/context-zone";

export function WorktreeBadge({ show, branch }: { show: boolean; branch: string | null }) {
  if (!show || !branch) return null;
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <Badge variant="outline" className="text-xs font-normal gap-1">
        <IconGitBranch className="h-3 w-3" />
        {branch}
      </Badge>
      <span>Same branch as current session</span>
    </div>
  );
}

type SelectorsRowProps = {
  profileOptions: ReturnType<typeof useAgentProfileOptions>;
  executorProfileOptions: ReturnType<typeof useExecutorProfileOptions>;
  agentProfileId: string;
  executorProfileId: string;
  onAgentProfileChange: (value: string) => void;
  onExecutorProfileChange: (value: string) => void;
  disabled: boolean;
};

export function SelectorsRow({
  profileOptions,
  executorProfileOptions,
  agentProfileId,
  executorProfileId,
  onAgentProfileChange,
  onExecutorProfileChange,
  disabled,
}: SelectorsRowProps) {
  const noAgents = profileOptions.length === 0;
  return (
    <div className="grid gap-4 grid-cols-1 sm:grid-cols-2">
      <div>
        <AgentSelector
          options={profileOptions}
          value={agentProfileId}
          onValueChange={onAgentProfileChange}
          disabled={disabled || noAgents}
          placeholder={noAgents ? "No agents found" : "Select agent profile"}
        />
      </div>
      <div>
        <ExecutorProfileSelector
          options={executorProfileOptions}
          value={executorProfileId}
          onValueChange={onExecutorProfileChange}
          disabled={disabled}
          placeholder="Select executor profile"
        />
      </div>
    </div>
  );
}

type PromptZoneProps = {
  promptRef: React.RefObject<HTMLTextAreaElement | null>;
  contextItems: ReturnType<typeof toContextItems>;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  isDragging: boolean;
  isCreating: boolean;
  isSummarizing: boolean;
  isEnhancingPrompt: boolean;
  isUtilityConfigured: boolean;
  onDragOver: (e: React.DragEvent) => void;
  onDragLeave: (e: React.DragEvent) => void;
  onDrop: (e: React.DragEvent) => void;
  onPaste: (e: React.ClipboardEvent<HTMLTextAreaElement>) => void;
  onFileInputChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onAttachClick: () => void;
  onEnhancePrompt: () => void;
  setHasPrompt: (v: boolean) => void;
  onSubmitShortcut: (e: React.FormEvent) => void;
};

export function PromptZone({
  promptRef,
  contextItems,
  fileInputRef,
  isDragging,
  isCreating,
  isSummarizing,
  isEnhancingPrompt,
  isUtilityConfigured,
  onDragOver,
  onDragLeave,
  onDrop,
  onPaste,
  onFileInputChange,
  onAttachClick,
  onEnhancePrompt,
  setHasPrompt,
  onSubmitShortcut,
}: PromptZoneProps) {
  const inputDisabled = isCreating || isSummarizing;
  return (
    <div className="relative" onDragOver={onDragOver} onDragLeave={onDragLeave} onDrop={onDrop}>
      <div className="rounded-md border border-input bg-transparent">
        <ContextZone items={contextItems} />
        <Textarea
          ref={promptRef}
          placeholder="What should the agent work on?"
          className="border-0 focus-visible:ring-0 focus-visible:ring-offset-0 min-h-[120px] max-h-[240px] resize-none overflow-auto text-[13px]"
          autoFocus
          disabled={inputDisabled}
          data-testid="subtask-prompt-input"
          onInput={(e) => setHasPrompt((e.target as HTMLTextAreaElement).value.trim().length > 0)}
          onPaste={onPaste}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              onSubmitShortcut(e);
            }
          }}
        />
        <div className="flex items-center px-1 pb-1">
          <AttachButton onClick={onAttachClick} disabled={inputDisabled} />
          <EnhancePromptButton
            onClick={onEnhancePrompt}
            isLoading={isEnhancingPrompt}
            isConfigured={isUtilityConfigured}
          />
        </div>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={onFileInputChange}
          tabIndex={-1}
        />
      </div>
      {isDragging && (
        <div className="absolute inset-0 flex items-center justify-center bg-primary/10 border-2 border-dashed border-primary rounded-md pointer-events-none">
          <span className="text-sm text-primary font-medium">Drop files here</span>
        </div>
      )}
      {isSummarizing && (
        <div className="absolute inset-0 flex items-center justify-center rounded-md bg-background/80">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <IconLoader2 className="h-4 w-4 animate-spin" />
            <span>Generating summary...</span>
          </div>
        </div>
      )}
    </div>
  );
}
