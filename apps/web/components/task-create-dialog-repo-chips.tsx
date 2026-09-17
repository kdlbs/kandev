"use client";

import { useCallback, useRef, useState } from "react";
import { IconGitFork } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { Repository } from "@/lib/types/http";
import type {
  DialogFormState,
  TaskRepoRow,
  TaskRepositorySetsConfig,
} from "@/components/task-create-dialog-types";
import { RemoteRepoChipsRow } from "@/components/task-create-dialog-remote-repo-chips";
import { FolderPicker } from "@/components/folder-picker";
import { SourceModeSwitch } from "@/components/task-create-dialog-source-mode";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import { CreateLocalRepositorySurface } from "@/components/create-local-repository-surface";
import { MixedRepositoryChips } from "@/components/task-create-dialog-mixed-repository-chips";
import { RepositorySetsControl } from "@/components/task-create-dialog-repository-sets-control";
import { SaveRepositorySetDialog } from "@/components/task-create-dialog-repository-sets-save";
import { SaveRepositorySetMenuAction } from "@/components/task-create-dialog-repository-sets-save-action";
import type { DirectLocalExecutorSelection } from "@/components/task-create-dialog-handlers";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

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
  onRowPolicyChange?: (key: string, policyId: string, baseBranch: string) => void;
  onPolicySelected?: () => void;
  repositoryLocked?: boolean;
  branchLocked?: boolean;
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
  executorSourcePolicy?: import("@/components/task-create-dialog-executor-source-policy").ExecutorSourcePolicy;
  folderDisabledReason?: string;
  remoteOriginStates?: Record<
    string,
    import("@/hooks/domains/repositories/use-repository-clone-source").RepositoryCloneSourceState
  >;
  onRefreshRemoteOrigins?: () => void;
  onFolderSelectionAdded?: (wasEmpty: boolean) => void;
  onRepositorySelectionAdded?: (wasFolderOnly: boolean) => void;
  onAllWorkspaceSourcesRemoved?: () => void;
  onRepositorySelectionRemoved?: (
    remaining: import("@/components/task-create-dialog-types").TaskRepositorySelection[],
  ) => void;
  /** "No repository" mode: replace the chip row with a folder picker. */
  onToggleNoRepository?: () => void;
  onWorkspacePathChange?: (value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
  localRepositoryCreation?: {
    executorSelection: DirectLocalExecutorSelection | null;
    onCreated: (rowKey: string, repository: Repository) => void;
  };
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
  /**
   * The workspace's repository sets, plus how to apply one. Grouped into a single
   * prop so both surfaces that render this row (task create, new subtask) opt in
   * with one line, and Quick Chat - which renders WorkspaceRepoChips directly -
   * is untouched.
   */
  repositorySets?: TaskRepositorySetsConfig;
};

function applyRowBranchChange(
  fs: DialogFormState,
  isLocalExecutor: boolean | undefined,
  onRowBranchChange: (key: string, value: string) => void,
  key: string,
  value: string,
) {
  const hasSavedWorktreeBase =
    !isLocalExecutor && fs.repositories.some((row) => row.key === key && row.baseBranch);
  if (hasSavedWorktreeBase) {
    fs.updateRepository(key, { baseBranch: value || undefined });
    return;
  }
  onRowBranchChange(key, value);
}

function updateSavedBaseBranch(fs: DialogFormState, key: string, value: string) {
  fs.updateRepository(key, { baseBranch: value || undefined });
}

type CreatingRepositoryTarget = { rowKey: string; requestId: number };

function useCreatingRepositoryTarget() {
  const [target, setTarget] = useState<CreatingRepositoryTarget | null>(null);
  const targetRef = useRef<CreatingRepositoryTarget | null>(null);
  const nextRequestId = useRef(0);
  const openForRow = useCallback((rowKey: string) => {
    const nextTarget = { rowKey, requestId: ++nextRequestId.current };
    targetRef.current = nextTarget;
    setTarget(nextTarget);
  }, []);
  const clear = useCallback(() => {
    const rowKey = targetRef.current?.rowKey ?? null;
    targetRef.current = null;
    setTarget(null);
    return rowKey;
  }, []);
  return { target, targetRef, openForRow, clear };
}

function LocalRepositoryCreationSurface({
  creation,
  target,
  targetRef,
  workspaceId,
  multiRow,
  onOpenChange,
}: {
  creation: RepoChipsRowProps["localRepositoryCreation"];
  target: CreatingRepositoryTarget | null;
  targetRef: { current: CreatingRepositoryTarget | null };
  workspaceId: string | null;
  multiRow: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  if (!creation) return null;
  return (
    <CreateLocalRepositorySurface
      open={target !== null}
      onOpenChange={onOpenChange}
      workspaceId={workspaceId}
      executorSelection={creation.executorSelection}
      context={multiRow ? "task-create-multi" : "task-create"}
      onCreated={(repository) => {
        if (!target) return false;
        const isCurrentRequest = targetRef.current?.requestId === target.requestId;
        creation.onCreated(target.rowKey, repository);
        return isCurrentRequest;
      }}
    />
  );
}

type RepositoryCreationController = ReturnType<typeof useRepositoryCreationController>;

function useRepositoryCreationController() {
  const chipRowRef = useRef<HTMLDivElement>(null);
  const { target, targetRef, openForRow, clear } = useCreatingRepositoryTarget();
  const handleCreationOpenChange = (open: boolean) => {
    if (open || target === null) return;
    const rowKey = clear();
    if (rowKey === null) return;
    requestAnimationFrame(() => {
      const candidates = [
        ...Array.from(
          chipRowRef.current?.querySelectorAll<HTMLElement>("[data-repo-row-key]") ?? [],
        ),
        ...Array.from(document.querySelectorAll<HTMLElement>("[data-repo-row-key]")),
      ];
      const row = candidates.find((candidate) => candidate.dataset.repoRowKey === rowKey);
      row?.querySelector<HTMLElement>("[data-testid='repo-chip-trigger']")?.focus();
    });
  };
  return { chipRowRef, target, targetRef, openForRow, handleCreationOpenChange };
}

type RepoChipsRenderProps = RepoChipsRowProps & {
  creation: RepositoryCreationController;
};

export function RepoChipsRow(props: RepoChipsRowProps) {
  const creation = useRepositoryCreationController();
  const handleRowBranchChange = (key: string, value: string) =>
    applyRowBranchChange(props.fs, props.isLocalExecutor, props.onRowBranchChange, key, value);
  if (props.isTaskStarted) return null;

  return (
    <div
      ref={creation.chipRowRef}
      className="flex min-h-9 flex-wrap items-center gap-2"
      data-testid="repo-chips-row"
    >
      <RepoChipsContent {...props} creation={creation} onRowBranchChange={handleRowBranchChange} />
    </div>
  );
}

function RepoChipsContent(props: RepoChipsRenderProps) {
  if (props.fs.repositorySelections) return <MixedRepositorySurface {...props} />;
  return <LegacyRepositorySurface {...props} />;
}

function MixedRepositorySurface({
  fs,
  repositories,
  workspaceId,
  onRowRepositoryChange,
  onRowBranchChange,
  onRowPolicyChange,
  onPolicySelected,
  repositoryLocked,
  branchLocked,
  freshBranchAvailable,
  freshBranchEnabled,
  onToggleFreshBranch,
  isLocalExecutor,
  executorSourcePolicy,
  folderDisabledReason,
  remoteOriginStates,
  onRefreshRemoteOrigins,
  onFolderSelectionAdded,
  onRepositorySelectionAdded,
  onAllWorkspaceSourcesRemoved,
  onRepositorySelectionRemoved,
  onWorkspacePathChange,
  lastUsedBranch,
  userSettingsLoaded,
  localRepositoryCreation,
  onRefreshRepositories,
  repositoriesRefreshing,
  repositorySets,
  creation,
}: RepoChipsRenderProps) {
  return (
    <>
      <MixedRepositoryChips
        fs={fs}
        repositories={repositories}
        workspaceId={workspaceId}
        isLocalExecutor={!!isLocalExecutor}
        executorSourcePolicy={executorSourcePolicy}
        folderDisabledReason={folderDisabledReason}
        remoteOriginStates={remoteOriginStates}
        onRefreshRemoteOrigins={onRefreshRemoteOrigins}
        repositoryLocked={repositoryLocked}
        branchLocked={branchLocked}
        freshBranchEnabled={freshBranchEnabled}
        branchPolicyDisabledReason={policyDisabled(isLocalExecutor, freshBranchAvailable)}
        freshBranchToggle={buildFreshBranchToggle(
          fs.repositories.length,
          freshBranchAvailable,
          freshBranchEnabled,
          onToggleFreshBranch,
        )}
        onRowRepositoryChange={onRowRepositoryChange}
        onRowBranchChange={onRowBranchChange}
        onRowPolicyChange={onRowPolicyChange}
        onPolicySelected={onPolicySelected}
        onWorkspacePathChange={onWorkspacePathChange}
        lastUsedBranch={lastUsedBranch}
        userSettingsLoaded={userSettingsLoaded}
        onCreateRepository={localRepositoryCreation ? creation.openForRow : undefined}
        repositoryCreationOpen={creation.target !== null}
        onRefreshRepositories={onRefreshRepositories}
        repositoriesRefreshing={repositoriesRefreshing}
        repositorySets={repositoryLocked || branchLocked ? undefined : repositorySets}
        onFolderSelectionAdded={onFolderSelectionAdded}
        onRepositorySelectionAdded={onRepositorySelectionAdded}
        onAllWorkspaceSourcesRemoved={onAllWorkspaceSourcesRemoved}
        onRepositorySelectionRemoved={onRepositorySelectionRemoved}
      />
      <LocalRepositoryCreationSurface
        creation={localRepositoryCreation}
        target={creation.target}
        targetRef={creation.targetRef}
        workspaceId={workspaceId}
        multiRow={(fs.repositorySelections?.length ?? 0) > 1}
        onOpenChange={creation.handleCreationOpenChange}
      />
    </>
  );
}

function LegacyRepositorySurface({
  fs,
  repositories,
  workspaceId,
  onRowRepositoryChange,
  onRowBranchChange,
  onRowPolicyChange,
  onPolicySelected,
  repositoryLocked,
  branchLocked,
  onToggleRemote,
  freshBranchAvailable,
  freshBranchEnabled,
  onToggleFreshBranch,
  isLocalExecutor,
  onToggleNoRepository,
  onWorkspacePathChange,
  lastUsedBranch,
  userSettingsLoaded,
  localRepositoryCreation,
  onRefreshRepositories,
  repositoriesRefreshing,
  repositorySets,
  creation,
}: RepoChipsRenderProps) {
  const hasDiscovered = fs.discoveredRepositories.length > 0;
  const canAddMore = repositories.length > 0 || hasDiscovered;
  const addHint = computeAddHint(canAddMore, repositories.length);
  const branchPolicyDisabledReason = policyDisabled(isLocalExecutor, freshBranchAvailable);

  return (
    <>
      <ModeBody
        fs={fs}
        repositories={repositories}
        workspaceId={workspaceId}
        isLocalExecutor={!!isLocalExecutor}
        canAddMore={canAddMore}
        addHint={addHint}
        freshBranchAvailable={freshBranchAvailable}
        freshBranchEnabled={freshBranchEnabled}
        branchPolicyDisabledReason={branchPolicyDisabledReason}
        repositoryLocked={repositoryLocked}
        branchLocked={branchLocked}
        onRowRepositoryChange={onRowRepositoryChange}
        onRowBranchChange={onRowBranchChange}
        onRowPolicyChange={onRowPolicyChange}
        onPolicySelected={onPolicySelected}
        onToggleFreshBranch={onToggleFreshBranch}
        onWorkspacePathChange={onWorkspacePathChange}
        lastUsedBranch={lastUsedBranch}
        userSettingsLoaded={userSettingsLoaded}
        onCreateRepository={localRepositoryCreation ? creation.openForRow : undefined}
        onRefreshRepositories={onRefreshRepositories}
        repositoriesRefreshing={repositoriesRefreshing}
      />
      {repositorySets && !repositoryLocked && !branchLocked && !fs.useRemote && !fs.noRepository ? (
        <RepositorySetsSurface
          repositorySets={repositorySets}
          repositories={repositories}
          rows={fs.repositories}
        />
      ) : null}
      {repositoryLocked ? null : (
        <SourceModeSwitch
          useRemote={fs.useRemote}
          noRepository={fs.noRepository}
          onToggleRemote={onToggleRemote}
          onToggleNoRepository={onToggleNoRepository}
        />
      )}
      <LocalRepositoryCreationSurface
        creation={localRepositoryCreation}
        target={creation.target}
        targetRef={creation.targetRef}
        workspaceId={workspaceId}
        multiRow={fs.repositories.length > 1}
        onOpenChange={creation.handleCreationOpenChange}
      />
    </>
  );
}

/**
 * The Sets control plus its save dialog. Extracted so RepoChipsRow stays under
 * the function-length cap.
 */
function RepositorySetsSurface({
  repositorySets,
  repositories,
  rows,
}: {
  repositorySets: NonNullable<RepoChipsRowProps["repositorySets"]>;
  repositories: Repository[];
  rows: TaskRepoRow[];
}) {
  const save = repositorySets.save;
  return (
    <>
      <RepositorySetsControl
        sets={repositorySets.sets}
        repositories={repositories}
        rows={rows}
        onApply={repositorySets.onApply}
        footerActions={
          save ? <SaveRepositorySetMenuAction onSelect={() => save.setOpen(true)} /> : null
        }
      />
      {save ? (
        <SaveRepositorySetDialog
          open={save.open}
          onOpenChange={save.setOpen}
          workspaceId={save.workspaceId}
          rows={save.rows}
          selections={save.selections}
          repositories={save.repositories}
          isLocalExecutor={save.isLocalExecutor}
          freshBranchEnabled={save.freshBranchEnabled}
        />
      ) : null}
    </>
  );
}

function ModeBody({
  fs,
  repositories,
  workspaceId,
  isLocalExecutor,
  repositoryLocked,
  branchLocked,
  canAddMore,
  addHint,
  freshBranchAvailable,
  freshBranchEnabled,
  branchPolicyDisabledReason,
  onRowRepositoryChange,
  onRowBranchChange,
  onRowPolicyChange,
  onPolicySelected,
  onToggleFreshBranch,
  onWorkspacePathChange,
  lastUsedBranch,
  userSettingsLoaded,
  onCreateRepository,
  onRefreshRepositories,
  repositoriesRefreshing,
}: {
  fs: DialogFormState;
  repositories: Repository[];
  workspaceId: string | null;
  isLocalExecutor: boolean;
  repositoryLocked?: boolean;
  branchLocked?: boolean;
  canAddMore: boolean;
  addHint: string | undefined;
  freshBranchAvailable?: boolean;
  freshBranchEnabled?: boolean;
  branchPolicyDisabledReason?: string;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onRowPolicyChange?: (key: string, policyId: string, baseBranch: string) => void;
  onPolicySelected?: () => void;
  onToggleFreshBranch?: (enabled: boolean) => void;
  onWorkspacePathChange?: (value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
  onCreateRepository?: (key: string) => void;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
}) {
  if (fs.noRepository) {
    return <NoRepositoryMode fs={fs} onWorkspacePathChange={onWorkspacePathChange} />;
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
      repositoryLocked={repositoryLocked}
      branchLocked={branchLocked}
      isLocalExecutor={isLocalExecutor}
      currentLocalBranch={fs.currentLocalBranch}
      currentLocalBranchLoading={fs.currentLocalBranchLoading}
      freshBranchEnabled={fs.freshBranchEnabled}
      branchPolicyDisabledReason={branchPolicyDisabledReason}
      canAddMore={canAddMore}
      addHint={addHint}
      onAdd={fs.addRepository}
      onRemove={fs.removeRepository}
      onRowRepositoryChange={onRowRepositoryChange}
      onRowBranchChange={onRowBranchChange}
      onRowBaseBranchChange={(key, value) => updateSavedBaseBranch(fs, key, value)}
      onRowPolicyChange={onRowPolicyChange}
      onPolicySelected={onPolicySelected}
      showBranchPolicies
      showDiscoveryControls
      lastUsedBranch={lastUsedBranch}
      userSettingsLoaded={userSettingsLoaded}
      onCreateRepository={onCreateRepository}
      onRefreshRepositories={onRefreshRepositories}
      repositoriesRefreshing={repositoriesRefreshing}
      freshBranchToggle={buildFreshBranchToggle(
        fs.repositories.length,
        freshBranchAvailable,
        freshBranchEnabled,
        onToggleFreshBranch,
      )}
    />
  );
}

function NoRepositoryMode({
  fs,
  onWorkspacePathChange,
}: {
  fs: DialogFormState;
  onWorkspacePathChange?: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <FolderPicker
      value={fs.workspacePath}
      onChange={onWorkspacePathChange ?? (() => {})}
      placeholder={t("task:pickAStartingFolderOptional")}
    />
  );
}

function buildFreshBranchToggle(
  repositoryCount: number,
  available: boolean | undefined,
  enabled: boolean | undefined,
  onToggle?: (enabled: boolean) => void,
) {
  if (!available || !onToggle || repositoryCount !== 1) return null;
  return <FreshBranchToggle enabled={!!enabled} onToggle={onToggle} />;
}

function FreshBranchToggle({
  enabled,
  onToggle,
}: {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={() => onToggle(!enabled)}
          data-testid="fresh-branch-toggle"
          aria-pressed={enabled}
          aria-label={enabled ? t("task:forkANewBranchFromA") : t("task:forkANewBranchFromA2")}
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
        {enabled ? t("task:forkModeANewBranchWill") : t("task:byDefaultTheLocalExecutorUses")}
      </TooltipContent>
    </Tooltip>
  );
}

function computeAddHint(canAddMore: boolean, workspaceRepoCount: number): string | undefined {
  if (canAddMore) return undefined;
  if (workspaceRepoCount === 0) return t("task:noRepositoriesAvailableInWorkspace");
  return t("task:allWorkspaceRepositoriesAdded");
}

function policyDisabled(
  isLocalExecutor: boolean | undefined,
  freshBranchAvailable: boolean | undefined,
): string | undefined {
  return isLocalExecutor && !freshBranchAvailable
    ? t("task:branchPolicyRequiresSingleRepository")
    : undefined;
}
