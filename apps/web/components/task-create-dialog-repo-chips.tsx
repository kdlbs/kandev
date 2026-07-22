"use client";

import { IconGitFork } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import { RemoteRepoChipsRow } from "@/components/task-create-dialog-remote-repo-chips";
import { FolderPicker } from "@/components/folder-picker";
import { SourceModeSwitch } from "@/components/task-create-dialog-source-mode";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";

type RepoChipsRowProps = {
  fs: DialogFormState;
  repositories: Repository[];
  isTaskStarted: boolean;
  /** Required for loading branches on discovered (path-keyed) rows. */
  workspaceId: string | null;
  /**
   * Per-row repo change handler. Resolves the picked value into either a
   * workspace `repositoryId` or a discovered `localPath` and writes that
   * into the row. Comes from useDialogHandlers so the resolution logic
   * stays in one place.
   */
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  /** Toggles the Remote tab on/off. Remote-mode rows live in `fs.remoteRepos`. */
  onToggleRemote?: () => void;
  /**
   * Fresh-branch toggle props. When `freshBranchAvailable` is true the toggle
   * renders inline at the right edge of the chip row so it sits next to the
   * branch pills it affects, instead of taking its own row under the
   * agent/executor selectors.
   */
  freshBranchAvailable?: boolean;
  freshBranchEnabled?: boolean;
  onToggleFreshBranch?: (enabled: boolean) => void;
  /**
   * When the task runs on the local executor, the chip seeds row.branch with
   * the workspace's current branch (so the user sees what's on disk and the
   * submit payload always carries an explicit value). The chip stays
   * editable — picking a different existing branch triggers `git checkout`
   * server-side; keeping the default skips git ops entirely. Fresh-branch
   * mode is independent: it creates a new branch from a chosen base.
   */
  isLocalExecutor?: boolean;
  /** "No repository" mode: replace the chip row with a folder picker. */
  onToggleNoRepository?: () => void;
  onWorkspacePathChange?: (value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
};

export function RepoChipsRow({
  fs,
  repositories,
  isTaskStarted,
  workspaceId,
  onRowRepositoryChange,
  onRowBranchChange,
  onToggleRemote,
  freshBranchAvailable,
  freshBranchEnabled,
  onToggleFreshBranch,
  isLocalExecutor,
  onToggleNoRepository,
  onWorkspacePathChange,
  lastUsedBranch,
  userSettingsLoaded,
}: RepoChipsRowProps) {
  // Local executor branch behavior:
  //   - chip is clickable (user can switch to any existing branch on disk)
  //   - row.branch seeds from the workspace's current branch (currentLocalBranch)
  //     via the autoselect path, so the chip displays the current branch by
  //     default and the submit payload always carries an explicit value
  //   - if user keeps the default, backend's "branch == current → skip" logic
  //     runs (no git ops)
  //   - if user picks a different existing branch, backend runs `git checkout`
  //   - "Fork a new branch" toggle is a separate flow that creates a NEW branch
  //     from the selected base
  // Other executors: branch is fully editable (no special pre-fill).
  const branchLocked = false;
  // No early returns above hooks. URL mode and started-state checks happen below.
  if (isTaskStarted) return null;

  // Multi-branch support: the same repo can appear multiple times on a task
  // when each row picks a different branch. Uniqueness is enforced on the
  // (repository_id, checkout_branch) pair at submit time by the backend, so
  // the dropdown never filters repos out — picking "frontend" twice and
  // assigning two different branches is a supported flow.
  const hasDiscovered = fs.discoveredRepositories.length > 0;
  const canAddMore = repositories.length > 0 || hasDiscovered;
  const addHint = computeAddHint(canAddMore, repositories.length);

  return (
    // min-h-9 reserves enough vertical space for the tallest mode body so the
    // modal doesn't jump when the user toggles between Repo / URL / None
    // (None renders a single pill, Repo can render chips + branch + add and
    // sometimes wraps when the segmented control crowds the row).
    <div className="flex min-h-9 flex-wrap items-center gap-2" data-testid="repo-chips-row">
      <ModeBody
        fs={fs}
        repositories={repositories}
        workspaceId={workspaceId}
        branchLocked={branchLocked}
        isLocalExecutor={!!isLocalExecutor}
        canAddMore={canAddMore}
        addHint={addHint}
        freshBranchAvailable={freshBranchAvailable}
        freshBranchEnabled={freshBranchEnabled}
        onRowRepositoryChange={onRowRepositoryChange}
        onRowBranchChange={onRowBranchChange}
        onToggleFreshBranch={onToggleFreshBranch}
        onWorkspacePathChange={onWorkspacePathChange}
        lastUsedBranch={lastUsedBranch}
        userSettingsLoaded={userSettingsLoaded}
      />
      <SourceModeSwitch
        useRemote={fs.useRemote}
        noRepository={fs.noRepository}
        onToggleRemote={onToggleRemote}
        onToggleNoRepository={onToggleNoRepository}
      />
    </div>
  );
}

function ModeBody({
  fs,
  repositories,
  workspaceId,
  branchLocked,
  isLocalExecutor,
  canAddMore,
  addHint,
  freshBranchAvailable,
  freshBranchEnabled,
  onRowRepositoryChange,
  onRowBranchChange,
  onToggleFreshBranch,
  onWorkspacePathChange,
  lastUsedBranch,
  userSettingsLoaded,
}: {
  fs: DialogFormState;
  repositories: Repository[];
  workspaceId: string | null;
  branchLocked: boolean;
  isLocalExecutor: boolean;
  canAddMore: boolean;
  addHint: string | undefined;
  freshBranchAvailable?: boolean;
  freshBranchEnabled?: boolean;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onToggleFreshBranch?: (enabled: boolean) => void;
  onWorkspacePathChange?: (value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
}) {
  if (fs.noRepository) {
    return (
      <FolderPicker
        value={fs.workspacePath}
        onChange={onWorkspacePathChange ?? (() => {})}
        placeholder="pick a starting folder (optional)"
      />
    );
  }
  if (fs.useRemote) {
    return (
      <RemoteRepoChipsRow
        workspaceId={workspaceId}
        fs={fs}
        onUpdateRow={fs.updateRemoteRepo}
        onAddRow={fs.addRemoteRepo}
        onRemoveRow={fs.removeRemoteRepo}
      />
    );
  }
  return (
    <WorkspaceRepoChips
      rows={fs.repositories}
      repositories={repositories}
      discoveredRepositories={fs.discoveredRepositories}
      workspaceId={workspaceId}
      branchLocked={branchLocked}
      isLocalExecutor={isLocalExecutor}
      currentLocalBranch={fs.currentLocalBranch}
      currentLocalBranchLoading={fs.currentLocalBranchLoading}
      freshBranchEnabled={fs.freshBranchEnabled}
      canAddMore={canAddMore}
      addHint={addHint}
      onAdd={fs.addRepository}
      onRemove={fs.removeRepository}
      onRowRepositoryChange={onRowRepositoryChange}
      onRowBranchChange={onRowBranchChange}
      lastUsedBranch={lastUsedBranch}
      userSettingsLoaded={userSettingsLoaded}
      freshBranchToggle={
        // Multi-repo runs use worktrees, so the existing-vs-fork choice
        // is irrelevant — only surface the toggle for single-repo flows.
        freshBranchAvailable && onToggleFreshBranch && fs.repositories.length === 1 ? (
          <FreshBranchToggle enabled={!!freshBranchEnabled} onToggle={onToggleFreshBranch} />
        ) : null
      }
    />
  );
}

function FreshBranchToggle({
  enabled,
  onToggle,
}: {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={() => onToggle(!enabled)}
          data-testid="fresh-branch-toggle"
          aria-pressed={enabled}
          aria-label={
            enabled
              ? "Fork a new branch from a base (turn off to use current checkout)"
              : "Fork a new branch from a base instead of using current checkout"
          }
          className={cn(
            "inline-flex h-7 w-7 items-center justify-center rounded-md border border-input cursor-pointer transition-colors",
            enabled
              ? "bg-muted text-foreground"
              : "bg-transparent text-muted-foreground hover:text-foreground hover:bg-muted/60",
          )}
        >
          <IconGitFork className="h-3.5 w-3.5" />
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        {enabled
          ? "Fork mode: a new branch will be created from the selected base before the agent runs. Click to turn off and use your repository's current checkout instead."
          : "By default the local executor uses your repository's current checkout. Click to fork a new branch from a base instead, leaving your working tree untouched."}
      </TooltipContent>
    </Tooltip>
  );
}

function computeAddHint(canAddMore: boolean, workspaceRepoCount: number): string | undefined {
  if (canAddMore) return undefined;
  if (workspaceRepoCount === 0) return "No repositories available in this workspace";
  return "All workspace repositories are already added";
}
