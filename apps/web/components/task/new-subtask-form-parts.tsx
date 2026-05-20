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
import type { SubtaskWorkspaceMode, useSubtaskFormState } from "./new-subtask-form-state";
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

type WorkspaceSectionProps = {
  inheritParent: boolean;
  fs: ReturnType<typeof useSubtaskFormState>;
  handlers: ReturnType<typeof useDialogHandlers>;
  availableRepositories: Repository[];
  workspaceId: string | null;
  worktreeBranch: string | null;
  showWorktreeBadge: boolean;
};

/**
 * Renders the workspace section under the workspace-mode toggle. When
 * inherit_parent is selected the repo pickers are hidden (the backend
 * inherits parent's repos); when new_workspace is selected we show the
 * existing chip row + branch badge so the user can override.
 */
function WorkspaceSection({
  inheritParent,
  fs,
  handlers,
  availableRepositories,
  workspaceId,
  worktreeBranch,
  showWorktreeBadge,
}: WorkspaceSectionProps) {
  if (inheritParent) {
    return <WorktreeBadge show={!!worktreeBranch} branch={worktreeBranch} />;
  }
  return (
    <>
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
    </>
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
  /** Office task-handoffs phase 5 — workspace mode toggle. */
  workspaceMode: SubtaskWorkspaceMode;
  onWorkspaceModeChange: (m: SubtaskWorkspaceMode) => void;
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

type WorkspaceModeToggleProps = {
  value: SubtaskWorkspaceMode;
  onChange: (m: SubtaskWorkspaceMode) => void;
  disabled: boolean;
  worktreeBranch: string | null;
};

/**
 * Two-option toggle: inherit the parent task's materialized workspace,
 * or create a new workspace from selected repositories. Office task-
 * handoffs phase 5 — the backend records group membership when
 * inherit_parent is selected so launch reuses the parent's environment.
 */
export function WorkspaceModeToggle({
  value,
  onChange,
  disabled,
  worktreeBranch,
}: WorkspaceModeToggleProps) {
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-muted-foreground">Workspace</label>
      <div
        role="radiogroup"
        aria-label="Workspace mode"
        className="grid grid-cols-1 gap-2 sm:grid-cols-2"
      >
        <WorkspaceModeOption
          value="inherit_parent"
          label="Inherit parent workspace"
          description={
            worktreeBranch
              ? `Run in the parent's worktree (${worktreeBranch})`
              : "Run in the parent's materialized workspace"
          }
          checked={value === "inherit_parent"}
          disabled={disabled}
          onSelect={() => onChange("inherit_parent")}
          dataTestId="subtask-workspace-mode-inherit"
        />
        <WorkspaceModeOption
          value="new_workspace"
          label="Create new workspace"
          description="Pick a different repo, local folder, or remote URL"
          checked={value === "new_workspace"}
          disabled={disabled}
          onSelect={() => onChange("new_workspace")}
          dataTestId="subtask-workspace-mode-new"
        />
      </div>
    </div>
  );
}

type WorkspaceModeOptionProps = {
  value: SubtaskWorkspaceMode;
  label: string;
  description: string;
  checked: boolean;
  disabled: boolean;
  onSelect: () => void;
  dataTestId: string;
};

function WorkspaceModeOption({
  value,
  label,
  description,
  checked,
  disabled,
  onSelect,
  dataTestId,
}: WorkspaceModeOptionProps) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={checked}
      data-testid={dataTestId}
      data-value={value}
      disabled={disabled}
      onClick={onSelect}
      className={
        "cursor-pointer rounded-md border px-3 py-2 text-left text-xs transition-colors " +
        (checked
          ? "border-primary bg-primary/5 text-foreground"
          : "border-border hover:border-primary/60 text-muted-foreground hover:text-foreground") +
        (disabled ? " cursor-not-allowed opacity-60" : "")
      }
    >
      <div className="font-medium">{label}</div>
      <div className="mt-0.5 text-[11px] text-muted-foreground">{description}</div>
    </button>
  );
}

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
  workspaceMode,
  onWorkspaceModeChange,
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
  const inheritParent = workspaceMode === "inherit_parent";
  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label htmlFor="subtask-title-input" className="text-xs font-medium text-muted-foreground">
          Title
        </label>
        <Input
          id="subtask-title-input"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Subtask title"
          className="text-sm"
          data-testid="subtask-title-input"
          disabled={isCreating}
        />
      </div>
      <WorkspaceModeToggle
        value={workspaceMode}
        onChange={onWorkspaceModeChange}
        disabled={isCreating}
        worktreeBranch={worktreeBranch}
      />
      <WorkspaceSection
        inheritParent={inheritParent}
        fs={fs}
        handlers={handlers}
        availableRepositories={availableRepositories}
        workspaceId={workspaceId}
        worktreeBranch={worktreeBranch}
        showWorktreeBadge={showWorktreeBadge}
      />
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
