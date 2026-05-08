"use client";

import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { DialogFooter } from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { IconGitBranch, IconLoader2 } from "@tabler/icons-react";
import { AgentSelector, ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import type {
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { RepoChipsRow } from "@/components/task-create-dialog-repo-chips";
import type { useDialogHandlers } from "@/components/task-create-dialog-handlers";
import type { Repository } from "@/lib/types/http";
import type { useSubtaskFormState } from "./new-subtask-form-state";
import {
  AttachButton,
  ContextSelect,
  toContextItems,
  useDialogAttachments,
} from "./session-dialog-shared";
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
  attachments: ReturnType<typeof useDialogAttachments>;
  isCreating: boolean;
  isSummarizing: boolean;
  isEnhancingPrompt: boolean;
  isUtilityConfigured: boolean;
  handleEnhancePrompt: () => void;
  setHasPrompt: (v: boolean) => void;
  onSubmitShortcut: (e: React.FormEvent) => void;
};

export function PromptZone({
  promptRef,
  contextItems,
  attachments,
  isCreating,
  isSummarizing,
  isEnhancingPrompt,
  isUtilityConfigured,
  handleEnhancePrompt,
  setHasPrompt,
  onSubmitShortcut,
}: PromptZoneProps) {
  const {
    isDragging,
    fileInputRef,
    handlePaste,
    handleDragOver,
    handleDragLeave,
    handleDrop,
    handleAttachClick,
    handleFileInputChange,
  } = attachments;
  const inputDisabled = isCreating || isSummarizing;
  return (
    <div
      className="relative"
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
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
          onPaste={handlePaste}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              onSubmitShortcut(e);
            }
          }}
        />
        <div className="flex items-center px-1 pb-1">
          <AttachButton onClick={handleAttachClick} disabled={inputDisabled} />
          <EnhancePromptButton
            onClick={handleEnhancePrompt}
            isLoading={isEnhancingPrompt}
            isConfigured={isUtilityConfigured}
          />
        </div>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={handleFileInputChange}
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

type SubtaskFormBodyProps = {
  fs: ReturnType<typeof useSubtaskFormState>;
  handlers: ReturnType<typeof useDialogHandlers>;
  title: string;
  setTitle: (v: string) => void;
  workspaceId: string | null;
  availableRepositories: Repository[];
  parentRepositoryId: string | null;
  worktreeBranch: string | null;
  profileOptions: ReturnType<typeof useAgentProfileOptions>;
  executorProfileOptions: ReturnType<typeof useExecutorProfileOptions>;
  agentProfileId: string;
  contextValue: string;
  onContextChange: (value: string) => void | Promise<void>;
  hasInitialPrompt: boolean;
  sessionOptions: React.ComponentProps<typeof ContextSelect>["sessionOptions"];
  promptZone: React.ReactNode;
  isCreating: boolean;
  isSummarizing: boolean;
  hasPrompt: boolean;
  onClose: () => void;
  onSubmit: (e: React.FormEvent) => void;
};

/**
 * Renders the entire subtask form body (title input, repo chips, selectors,
 * context picker, prompt zone, footer). Extracted from `NewSubtaskForm` so
 * the parent stays under the per-function complexity cap.
 */
export function SubtaskFormBody({
  fs,
  handlers,
  title,
  setTitle,
  workspaceId,
  availableRepositories,
  parentRepositoryId,
  worktreeBranch,
  profileOptions,
  executorProfileOptions,
  agentProfileId,
  contextValue,
  onContextChange,
  hasInitialPrompt,
  sessionOptions,
  promptZone,
  isCreating,
  isSummarizing,
  hasPrompt,
  onClose,
  onSubmit,
}: SubtaskFormBodyProps) {
  // Worktree badge only when subtask still targets parent's repo (single chip,
  // same id). Adding repos or pasting a URL makes it ambiguous, so hide.
  const showWorktreeBadge =
    !!worktreeBranch &&
    fs.repositories.length === 1 &&
    fs.repositories[0]?.repositoryId === parentRepositoryId &&
    !fs.useGitHubUrl;
  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-muted-foreground">Title</label>
        <Input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Subtask title"
          className="text-sm"
          data-testid="subtask-title-input"
          disabled={isCreating}
        />
      </div>
      <RepoChipsRow
        fs={fs}
        repositories={availableRepositories}
        isTaskStarted={false}
        workspaceId={workspaceId}
        onRowRepositoryChange={handlers.handleRowRepositoryChange}
        onRowBranchChange={handlers.handleRowBranchChange}
        onToggleGitHubUrl={handlers.handleToggleGitHubUrl}
        onGitHubUrlChange={handlers.handleGitHubUrlChange}
      />
      <WorktreeBadge show={showWorktreeBadge} branch={worktreeBranch} />
      <SelectorsRow
        profileOptions={profileOptions}
        executorProfileOptions={executorProfileOptions}
        agentProfileId={agentProfileId}
        executorProfileId={fs.executorProfileId}
        onAgentProfileChange={handlers.handleAgentProfileChange}
        onExecutorProfileChange={handlers.handleExecutorProfileChange}
        disabled={isCreating}
      />
      <ContextSelect
        value={contextValue}
        onValueChange={onContextChange}
        hasInitialPrompt={hasInitialPrompt}
        sessionOptions={sessionOptions}
        isSummarizing={isSummarizing}
      />
      {promptZone}
      <DialogFooter>
        <Button
          type="button"
          variant="ghost"
          onClick={onClose}
          disabled={isCreating}
          className="cursor-pointer"
        >
          Cancel
        </Button>
        <Button
          type="submit"
          disabled={isCreating || isSummarizing || !hasPrompt}
          className="cursor-pointer"
        >
          {isCreating ? "Creating..." : "Create Subtask"}
        </Button>
      </DialogFooter>
    </form>
  );
}
