"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { DialogFooter } from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { IconGitBranch, IconLoader2 } from "@tabler/icons-react";
import { PromptResultRecovery } from "@/components/prompt-result-recovery";
import { AgentSelector, ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import type {
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { RepoChipsRow } from "@/components/task-create-dialog-repo-chips";
import type { useDialogHandlers } from "@/components/task-create-dialog-handlers";
import type { UtilityGenerationResult } from "@/hooks/use-utility-agent-generator";
import type { Repository } from "@/lib/types/http";
import type { SubtaskWorkspaceMode, useSubtaskFormState } from "./new-subtask-form-state";
import {
  AttachButton,
  ContextSelect,
  toContextItems,
  useDialogAttachments,
} from "./session-dialog-shared";
import { ContextZone } from "./chat/context-items/context-zone";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";
import { TaskAutopilotToggle } from "@/components/task-autopilot-toggle";
import { useRepositorySets } from "@/hooks/domains/workspace/use-repository-sets";
import { useApplyRepositorySet } from "@/components/task-create-dialog-repository-sets-apply";

export function WorktreeBadge({ show, branch }: { show: boolean; branch: string | null }) {
  const { t } = useTranslation();
  if (!show || !branch) return null;
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <Badge variant="outline" className="text-xs font-normal gap-1">
        <IconGitBranch className="h-3 w-3" />
        {branch}
      </Badge>
      <span>{t("task:sameBranchAsCurrentSession")}</span>
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
  /**
   * When true, hide the executor-profile selector. The subtask reuses the
   * parent's materialized environment (inherit_parent), so choosing an
   * executor would be meaningless — the parent's executor is always used.
   */
  hideExecutor: boolean;
};

export function SelectorsRow({
  profileOptions,
  executorProfileOptions,
  agentProfileId,
  executorProfileId,
  onAgentProfileChange,
  onExecutorProfileChange,
  disabled,
  hideExecutor,
}: SelectorsRowProps) {
  const { t } = useTranslation();
  const noAgents = profileOptions.length === 0;
  return (
    <div className={"grid min-w-0 grid-cols-1 gap-4" + (hideExecutor ? "" : " sm:grid-cols-2")}>
      <div className="min-w-0">
        <AgentSelector
          options={profileOptions}
          value={agentProfileId}
          onValueChange={onAgentProfileChange}
          disabled={disabled || noAgents}
          placeholder={noAgents ? t("task:noAgentsFound2") : t("task:selectAgentProfile")}
          popoverPortal
        />
      </div>
      {!hideExecutor && (
        <div className="min-w-0">
          <ExecutorProfileSelector
            options={executorProfileOptions}
            value={executorProfileId}
            onValueChange={onExecutorProfileChange}
            disabled={disabled}
            placeholder={t("task:selectExecutorProfile")}
            popoverPortal
          />
        </div>
      )}
    </div>
  );
}

type PromptZoneProps = {
  promptRef: React.RefObject<HTMLTextAreaElement | null>;
  promptValue: string;
  contextItems: ReturnType<typeof toContextItems>;
  attachments: ReturnType<typeof useDialogAttachments>;
  isCreating: boolean;
  isSummarizing: boolean;
  isEnhancingPrompt: boolean;
  isUtilityConfigured: boolean;
  handleEnhancePrompt: () => void;
  pendingResult: UtilityGenerationResult | null;
  onPromptChange: (value: string) => void;
  onApplyPending: () => void;
  onCopyPending: () => Promise<void> | void;
  onSubmitShortcut: (e: React.FormEvent) => void;
};

export function PromptZone({
  promptRef,
  promptValue,
  contextItems,
  attachments,
  isCreating,
  isSummarizing,
  isEnhancingPrompt,
  isUtilityConfigured,
  handleEnhancePrompt,
  pendingResult,
  onPromptChange,
  onApplyPending,
  onCopyPending,
  onSubmitShortcut,
}: PromptZoneProps) {
  const { t } = useTranslation();
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
      className="relative min-w-0 max-w-full"
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <div className="min-w-0 max-w-full rounded-md border border-input bg-transparent focus-within:ring-2 focus-within:ring-ring/30">
        <ContextZone items={contextItems} />
        <Textarea
          ref={promptRef}
          value={promptValue}
          placeholder={t("task:whatShouldTheAgentWorkOn")}
          className="min-w-0 max-w-full field-sizing-fixed wrap-anywhere border-0 focus-visible:ring-0 focus-visible:ring-offset-0 min-h-[120px] max-h-[240px] resize-none overflow-auto text-[13px]"
          autoFocus
          disabled={inputDisabled}
          data-testid="subtask-prompt-input"
          onChange={(event) => onPromptChange(event.target.value)}
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
      <PromptResultRecovery
        pendingResult={pendingResult}
        onApply={onApplyPending}
        onCopy={onCopyPending}
      />
      {isDragging && (
        <div className="absolute inset-0 flex items-center justify-center bg-primary/10 border-2 border-dashed border-primary rounded-md pointer-events-none">
          <span className="text-sm text-primary font-medium">{t("task:dropFilesHere")}</span>
        </div>
      )}
      {isSummarizing && (
        <div className="absolute inset-0 flex items-center justify-center rounded-md bg-background/80">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <IconLoader2 className="h-4 w-4 animate-spin" />
            <span>{t("task:generatingSummary")}</span>
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
  isLocalExecutor: boolean;
  freshBranchAvailable: boolean;
};

/**
 * Renders the workspace section under the workspace-mode toggle. When
 * inherit_parent is selected the repo pickers are hidden (the backend
 * inherits parent's repos); when new_workspace is selected we show the
 * existing chip row so the user can choose the isolated workspace source.
 */
function WorkspaceSection({
  inheritParent,
  fs,
  handlers,
  availableRepositories,
  workspaceId,
  worktreeBranch,
  isLocalExecutor,
  freshBranchAvailable,
}: WorkspaceSectionProps) {
  // Hooks before the early return: a subtask that inherits the parent workspace
  // renders no picker, but the rules of hooks do not care.
  const { sets } = useRepositorySets(workspaceId, !inheritParent);
  const onApplyRepositorySet = useApplyRepositorySet({
    rows: fs.repositories,
    repositories: availableRepositories,
    setRepositories: fs.setRepositories,
    setRepositoriesDirty: fs.setRepositoriesDirty,
  });
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
        onRowPolicyChange={handlers.handleRowPolicyChange}
        onPolicySelected={
          isLocalExecutor && freshBranchAvailable ? () => fs.setFreshBranchEnabled(true) : undefined
        }
        onToggleRemote={handlers.handleToggleRemote}
        freshBranchAvailable={freshBranchAvailable}
        freshBranchEnabled={fs.freshBranchEnabled}
        onToggleFreshBranch={fs.setFreshBranchEnabled}
        isLocalExecutor={isLocalExecutor}
        repositorySets={{ sets, onApply: onApplyRepositorySet }}
      />
    </>
  );
}

type SubtaskFormBodyProps = {
  fs: ReturnType<typeof useSubtaskFormState>;
  handlers: ReturnType<typeof useDialogHandlers>;
  title: string;
  setTitle: (v: string) => void;
  autoTitle?: boolean;
  autopilot: boolean;
  workspaceId: string | null;
  availableRepositories: Repository[];
  worktreeBranch: string | null;
  isLocalExecutor: boolean;
  freshBranchAvailable: boolean;
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
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-muted-foreground">{t("common:workspace")}</label>
      <div
        role="radiogroup"
        aria-label={t("task:workspaceMode")}
        className="grid grid-cols-1 gap-2 sm:grid-cols-2"
      >
        <WorkspaceModeOption
          value="inherit_parent"
          label={t("task:inheritParentWorkspace")}
          description={
            worktreeBranch
              ? t("task:runInTheParentSWorktree", { worktreeBranch })
              : t("task:runInTheParentSMaterialized")
          }
          checked={value === "inherit_parent"}
          disabled={disabled}
          onSelect={() => onChange("inherit_parent")}
          dataTestId="subtask-workspace-mode-inherit"
        />
        <WorkspaceModeOption
          value="new_workspace"
          label={t("task:createNewWorkspace")}
          description={t("task:pickADifferentRepoLocalFolder")}
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
// eslint-disable-next-line max-lines-per-function -- shared form keeps workspace, prompt, and submit controls together.
export function SubtaskFormBody({
  fs,
  handlers,
  title,
  setTitle,
  autoTitle = false,
  autopilot,
  workspaceId,
  availableRepositories,
  worktreeBranch,
  isLocalExecutor,
  freshBranchAvailable,
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
  const { t } = useTranslation();
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(title);
  const inheritParent = workspaceMode === "inherit_parent";
  return (
    <form onSubmit={onSubmit} className="min-w-0 space-y-4">
      {!autoTitle && (
        <div className="space-y-1.5">
          <label
            htmlFor="subtask-title-input"
            className="text-xs font-medium text-muted-foreground"
          >
            {t("common:title")}
          </label>
          <Input
            ref={inputRef}
            id="subtask-title-input"
            value={title}
            onChange={(e) => setTitle(clampChange(e))}
            placeholder={t("common:subtaskTitle")}
            className="min-w-0 max-w-full text-sm"
            data-testid="subtask-title-input"
            disabled={isCreating}
          />
        </div>
      )}
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
        isLocalExecutor={isLocalExecutor}
        freshBranchAvailable={freshBranchAvailable}
      />
      <SelectorsRow
        profileOptions={profileOptions}
        executorProfileOptions={executorProfileOptions}
        agentProfileId={agentProfileId}
        executorProfileId={fs.executorProfileId}
        onAgentProfileChange={handlers.handleAgentProfileChange}
        onExecutorProfileChange={handlers.handleExecutorProfileChange}
        disabled={isCreating}
        hideExecutor={inheritParent}
      />
      <div
        className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-end gap-2"
        data-testid="subtask-context-autopilot-row"
      >
        <ContextSelect
          value={contextValue}
          onValueChange={onContextChange}
          hasInitialPrompt={hasInitialPrompt}
          sessionOptions={sessionOptions}
          isSummarizing={isSummarizing}
        />
        <TaskAutopilotToggle
          checked={autopilot}
          onCheckedChange={fs.setAutopilot}
          disabled={isCreating}
        />
      </div>
      {promptZone}
      <DialogFooter>
        <Button
          type="button"
          variant="ghost"
          onClick={onClose}
          disabled={isCreating}
          className="cursor-pointer"
        >
          {t("common:cancel")}
        </Button>
        <Button
          type="submit"
          disabled={isCreating || isSummarizing || !hasPrompt || (!autoTitle && !title.trim())}
          className="cursor-pointer"
        >
          {isCreating ? t("task:creatingEllipsis") : t("task:createSubtask")}
        </Button>
      </DialogFooter>
    </form>
  );
}
