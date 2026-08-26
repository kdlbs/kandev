"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useRouter } from "@/lib/routing/client-router";
import { IconGitBranch } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { SettingsSection } from "@/components/settings/settings-section";
import { RepositoryCard } from "@/components/settings/repository-card";
import { WorkspaceRepositorySetsSection } from "./workspace-repository-sets-section";
import { AddLocalRepositoryDialog } from "./workspace-add-local-repository-dialog";
import { generateUUID } from "@/lib/utils";
import {
  createRepositoryAction,
  createRepositoryScriptAction,
  deleteRepositoryAction,
  deleteRepositoryScriptAction,
  discoverRepositoriesAction,
  updateRepositoryAction,
  updateRepositoryScriptAction,
  validateRepositoryPathAction,
} from "@/app/actions/workspaces";
import {
  repositoryId as toRepositoryId,
  type LocalRepository,
  type Repository,
  type RepositoryScript,
  type Workspace,
} from "@/lib/types/http";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import type { ManualValidation } from "@/app/settings/workspace/workspace-repositories-dialog";
import { WorkspaceNotFoundCard } from "@/app/settings/workspace/workspace-not-found-card";
import {
  areRepositoryScriptsDirty,
  cloneRepository,
  isRepositoryDirty,
  mergeSavedRepositoryDraft,
  persistedRepositoryItems,
  type RepositoryWithScripts,
} from "@/app/settings/workspace/workspace-repositories-dirty";
import { defaultWorktreeBranchTemplate } from "@/lib/worktree-branch-template";
import { isValidManualRepository } from "@/app/settings/workspace/workspace-repositories-validation";

type RepositoryItem = RepositoryWithScripts & { __autoOpen?: boolean };
type WorkspaceRepositoriesClientProps = {
  workspace: Workspace | null;
  repositories: RepositoryWithScripts[];
  /** The dedicated Improve Kandev workspace is configuration-immutable. */
  isImproveWorkspace?: boolean;
};

function buildDraftRepo(
  workspace: Workspace,
  selectedRepo: LocalRepository | undefined,
  manualValidation: ManualValidation,
  manualRepoPath: string,
): RepositoryItem {
  const path = selectedRepo?.path ?? manualValidation.path ?? manualRepoPath.trim();
  // i18n-exempt: persisted repository name. See the comment below.
  // "New Repository" is PERSISTED as the repository's name, not rendered copy —
  // a stored name must not depend on the locale it was created in.
  // i18n-exempt: persisted repository name. See the comment above.
  const name =
    selectedRepo?.name ?? path.split("/").filter(Boolean).slice(-1)[0] ?? "New Repository";
  return {
    id: toRepositoryId(`temp-repo-${generateUUID()}`),
    workspace_id: workspace.id,
    name,
    source_type: "local",
    local_path: path,
    provider: "",
    provider_repo_id: "",
    provider_host: "",
    provider_owner: "",
    provider_name: "",
    default_branch: selectedRepo?.default_branch ?? "",
    worktree_branch_prefix: "feature/",
    worktree_branch_template: defaultWorktreeBranchTemplate,
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    secret_bindings: [],
    created_at: "",
    updated_at: "",
    scripts: [],
    __autoOpen: true,
  };
}

type RepoHandlerArgs = {
  workspace: Workspace | null;
  repositoryItems: RepositoryItem[];
  setRepositoryItems: React.Dispatch<React.SetStateAction<RepositoryItem[]>>;
  setSavedRepositoryItems: React.Dispatch<React.SetStateAction<RepositoryWithScripts[]>>;
  savedRepositoriesById: Map<string, RepositoryWithScripts>;
  clearRepositoryScripts: (id: string) => void;
};

async function saveNewRepository(
  repo: RepositoryItem,
  repoId: string,
  workspace: Workspace | null,
  setRepositoryItems: React.Dispatch<React.SetStateAction<RepositoryItem[]>>,
  setSavedRepositoryItems: React.Dispatch<React.SetStateAction<RepositoryWithScripts[]>>,
) {
  const created = await createRepositoryAction({
    workspace_id: workspace?.id ?? repo.workspace_id,
    // i18n-exempt: persisted repository name, same contract as buildDraftRepo above.
    name: repo.name.trim() || "New Repository",
    source_type: repo.source_type || "local",
    local_path: repo.local_path,
    provider: repo.provider,
    provider_repo_id: repo.provider_repo_id,
    provider_host: repo.provider_host,
    provider_owner: repo.provider_owner,
    provider_name: repo.provider_name,
    default_branch: repo.default_branch,
    worktree_branch_prefix: repo.worktree_branch_prefix,
    worktree_branch_template: repo.worktree_branch_template ?? defaultWorktreeBranchTemplate,
    pull_before_worktree: repo.pull_before_worktree,
    setup_script: repo.setup_script,
    cleanup_script: repo.cleanup_script,
    dev_script: repo.dev_script,
    copy_files: repo.copy_files,
    secret_bindings: repo.secret_bindings ?? [],
  });
  // Like the repository name above, the seeded script name and command are
  // PERSISTED and sent to a shell verbatim, so both stay in English.
  const scripts = await Promise.all(
    // i18n-exempt: persisted script name, sent to a shell verbatim. See the comment above.
    repo.scripts.map((script, index) =>
      createRepositoryScriptAction({
        repository_id: created.id,
        name: script.name.trim() || "New Script",
        command: script.command.trim() || 'echo ""',
        position: script.position ?? index,
      }),
    ),
  );
  const nextRepo: RepositoryWithScripts = { ...created, scripts };
  setRepositoryItems((prev) =>
    prev.map((item) =>
      item.id === repoId ? mergeSavedRepositoryDraft(item, repo, nextRepo) : item,
    ),
  );
  setSavedRepositoryItems((prev) => [cloneRepository(nextRepo), ...prev]);
}

type SaveExistingArgs = {
  repo: RepositoryItem;
  repoId: string;
  savedRepositoriesById: Map<string, RepositoryWithScripts>;
  clearRepositoryScripts: (id: string) => void;
  setRepositoryItems: React.Dispatch<React.SetStateAction<RepositoryItem[]>>;
  setSavedRepositoryItems: React.Dispatch<React.SetStateAction<RepositoryWithScripts[]>>;
};

async function saveExistingRepository({
  repo,
  repoId,
  savedRepositoriesById,
  clearRepositoryScripts,
  setRepositoryItems,
  setSavedRepositoryItems,
}: SaveExistingArgs) {
  const updated = await updateRepositoryAction(repoId, {
    name: repo.name,
    source_type: repo.source_type,
    local_path: repo.local_path,
    provider: repo.provider,
    provider_repo_id: repo.provider_repo_id,
    provider_host: repo.provider_host,
    provider_owner: repo.provider_owner,
    provider_name: repo.provider_name,
    default_branch: repo.default_branch,
    worktree_branch_prefix: repo.worktree_branch_prefix,
    worktree_branch_template: repo.worktree_branch_template ?? defaultWorktreeBranchTemplate,
    pull_before_worktree: repo.pull_before_worktree,
    setup_script: repo.setup_script,
    cleanup_script: repo.cleanup_script,
    dev_script: repo.dev_script,
    copy_files: repo.copy_files,
    secret_bindings: repo.secret_bindings ?? [],
  });
  const savedScripts = savedRepositoriesById.get(repoId)?.scripts ?? [];
  const currentScriptIds = new Set(repo.scripts.map((s) => s.id));
  await Promise.all(
    savedScripts
      .filter((s) => !currentScriptIds.has(s.id))
      .map((s) => deleteRepositoryScriptAction(s.id)),
  );
  const nextScripts = await Promise.all(
    repo.scripts.map((script, index) => {
      if (script.id.startsWith("temp-script-"))
        return createRepositoryScriptAction({
          repository_id: repoId,
          // i18n-exempt: persisted script name, sent to a shell verbatim.
          name: script.name.trim() || "New Script",
          command: script.command.trim() || 'echo ""',
          position: script.position ?? index,
        });
      return updateRepositoryScriptAction(script.id, {
        name: script.name,
        command: script.command,
        position: script.position ?? index,
      });
    }),
  );
  const nextRepo: RepositoryWithScripts = { ...updated, scripts: nextScripts };
  setRepositoryItems((prev) =>
    prev.map((item) =>
      item.id === repoId ? mergeSavedRepositoryDraft(item, repo, nextRepo) : item,
    ),
  );
  setSavedRepositoryItems((prev) =>
    prev.some((item) => item.id === repoId)
      ? prev.map((item) => (item.id === repoId ? cloneRepository(nextRepo) : item))
      : [...prev, cloneRepository(nextRepo)],
  );
  clearRepositoryScripts(repoId);
}

function useRepositoryHandlers({
  workspace,
  repositoryItems,
  setRepositoryItems,
  setSavedRepositoryItems,
  savedRepositoriesById,
  clearRepositoryScripts,
}: RepoHandlerArgs) {
  const handleUpdateRepository = (repoId: string, updates: Partial<Repository>) => {
    setRepositoryItems((prev) =>
      prev.map((repo) => (repo.id === repoId ? { ...repo, ...updates } : repo)),
    );
  };
  const handleAddRepositoryScript = (repoId: string) => {
    const script: RepositoryScript = {
      id: `temp-script-${generateUUID()}`,
      repository_id: toRepositoryId(repoId),
      name: "",
      command: "",
      position: repositoryItems.find((repo) => repo.id === repoId)?.scripts.length ?? 0,
      created_at: "",
      updated_at: "",
    };
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId ? { ...repo, scripts: [...repo.scripts, script] } : repo,
      ),
    );
  };
  const handleUpdateRepositoryScript = (
    repoId: string,
    scriptId: string,
    updates: Partial<RepositoryScript>,
  ) => {
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId
          ? {
              ...repo,
              scripts: repo.scripts.map((script) =>
                script.id === scriptId ? { ...script, ...updates } : script,
              ),
            }
          : repo,
      ),
    );
  };
  const handleDeleteRepositoryScript = (repoId: string, scriptId: string) => {
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId
          ? { ...repo, scripts: repo.scripts.filter((script) => script.id !== scriptId) }
          : repo,
      ),
    );
  };
  const handleSaveRepository = async (repoId: string) => {
    const repo = repositoryItems.find((item) => item.id === repoId);
    if (!repo) return;
    if (repoId.startsWith("temp-repo-")) {
      await saveNewRepository(repo, repoId, workspace, setRepositoryItems, setSavedRepositoryItems);
      return;
    }
    await saveExistingRepository({
      repo,
      repoId,
      savedRepositoriesById,
      clearRepositoryScripts,
      setRepositoryItems,
      setSavedRepositoryItems,
    });
  };
  const handleDeleteRepository = async (repoId: string) => {
    if (repoId.startsWith("temp-repo-")) {
      setRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
      return;
    }
    await deleteRepositoryAction(repoId);
    setRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
    setSavedRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
  };
  return {
    handleUpdateRepository,
    handleAddRepositoryScript,
    handleUpdateRepositoryScript,
    handleDeleteRepositoryScript,
    handleSaveRepository,
    handleDeleteRepository,
  };
}

function useDiscoverDialog(
  workspace: Workspace | null,
  toast: ReturnType<typeof useToast>["toast"],
  t: TFunction,
) {
  const [localRepoDialogOpen, setLocalRepoDialogOpen] = useState(false);
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [repoSearch, setRepoSearch] = useState("");
  const [selectedRepoPath, setSelectedRepoPath] = useState<string | null>(null);
  const [manualRepoPath, setManualRepoPath] = useState("");
  const [manualValidation, setManualValidation] = useState<ManualValidation>({ status: "idle" });
  const discoverRequest = useRequest(discoverRepositoriesAction);
  const validateRequest = useRequest(validateRepositoryPathAction);

  const filteredRepositories = useMemo(() => {
    const query = repoSearch.trim().toLowerCase();
    if (!query) return discoveredRepositories;
    return discoveredRepositories.filter(
      (repo) => repo.name.toLowerCase().includes(query) || repo.path.toLowerCase().includes(query),
    );
  }, [discoveredRepositories, repoSearch]);

  const handleDiscover = async () => {
    if (!workspace) return;
    try {
      const result = await discoverRequest.run(workspace.id);
      setDiscoveredRepositories(result.repositories);
    } catch (error) {
      toast({
        title: t("workspaces:failedToDiscoverRepositories"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    }
  };

  const openDialog = async () => {
    setLocalRepoDialogOpen(true);
    setRepoSearch("");
    setSelectedRepoPath(null);
    setManualRepoPath("");
    setManualValidation({ status: "idle" });
    await handleDiscover();
  };

  const handleValidateManualPath = async () => {
    if (!workspace || !manualRepoPath.trim()) return;
    setManualValidation({ status: "loading" });
    try {
      const result = await validateRequest.run(workspace.id, manualRepoPath.trim());
      if (isValidManualRepository(result))
        setManualValidation({
          status: "success",
          isValid: true,
          message: t("workspaces:validGitRepository"),
          path: result.path,
        });
      else
        // `result.message` is the backend's diagnostic and stays English by
        // design; only the fallback for a missing payload is copy.
        setManualValidation({
          status: "error",
          isValid: false,
          message: result.message || t("workspaces:invalidRepositoryPath"),
          path: result.path,
        });
    } catch (error) {
      setManualValidation({
        status: "error",
        isValid: false,
        message: error instanceof Error ? error.message : t("common:requestFailed"),
      });
    }
  };

  const handleSelectRepoPath = (path: string) => {
    setSelectedRepoPath(path);
    setManualRepoPath("");
    setManualValidation({ status: "idle" });
  };
  const handleManualRepoPathChange = (value: string) => {
    setManualRepoPath(value);
    setSelectedRepoPath(null);
    setManualValidation({ status: "idle" });
  };
  const canSave =
    Boolean(selectedRepoPath) ||
    (manualValidation.status === "success" && manualValidation.isValid === true);

  return {
    localRepoDialogOpen,
    setLocalRepoDialogOpen,
    filteredRepositories,
    repoSearch,
    setRepoSearch,
    selectedRepoPath,
    handleSelectRepoPath,
    manualRepoPath,
    handleManualRepoPathChange,
    manualValidation,
    handleValidateManualPath,
    isValidating: validateRequest.isLoading,
    isDiscovering: discoverRequest.isLoading,
    canSave,
    openDialog,
    discoveredRepositories,
  };
}
export function useWorkspaceRepositoriesPage(
  workspace: Workspace | null,
  repositories: RepositoryWithScripts[],
) {
  const router = useRouter();
  const { toast } = useToast();
  const { t } = useTranslation();
  const clearRepositoryScripts = useAppStore((state) => state.clearRepositoryScripts);
  const [repositoryItems, setRepositoryItems] = useState<RepositoryItem[]>(repositories);
  const [savedRepositoryItems, setSavedRepositoryItems] =
    useState<RepositoryWithScripts[]>(repositories);
  const savedRepositoriesById = useMemo(
    () => new Map(savedRepositoryItems.map((repo) => [repo.id, repo])),
    [savedRepositoryItems],
  );

  const handlers = useRepositoryHandlers({
    workspace,
    repositoryItems,
    setRepositoryItems,
    setSavedRepositoryItems,
    savedRepositoriesById,
    clearRepositoryScripts,
  });
  const {
    handleUpdateRepository,
    handleAddRepositoryScript,
    handleUpdateRepositoryScript,
    handleDeleteRepositoryScript,
    handleSaveRepository,
    handleDeleteRepository,
  } = handlers;

  const discover = useDiscoverDialog(workspace, toast, t);
  const {
    localRepoDialogOpen,
    setLocalRepoDialogOpen,
    filteredRepositories,
    repoSearch,
    setRepoSearch,
    selectedRepoPath,
    handleSelectRepoPath,
    manualRepoPath,
    handleManualRepoPathChange,
    manualValidation,
    handleValidateManualPath,
    isValidating,
    isDiscovering,
    canSave,
    openDialog,
    discoveredRepositories,
  } = discover;

  const handleConfirmLocalRepository = () => {
    if (!workspace) return;
    const draftRepo = buildDraftRepo(
      workspace,
      discoveredRepositories.find((repo) => repo.path === selectedRepoPath),
      manualValidation,
      manualRepoPath,
    );
    if (!draftRepo.local_path) return;
    setRepositoryItems((prev) => [draftRepo, ...prev]);
    setLocalRepoDialogOpen(false);
  };

  return {
    router,
    toast,
    repositoryItems,
    savedRepositoriesById,
    handleUpdateRepository,
    handleAddRepositoryScript,
    handleUpdateRepositoryScript,
    handleDeleteRepositoryScript,
    handleSaveRepository,
    handleDeleteRepository,
    localRepoDialogOpen,
    setLocalRepoDialogOpen,
    filteredRepositories,
    repoSearch,
    setRepoSearch,
    selectedRepoPath,
    handleSelectRepoPath,
    manualRepoPath,
    handleManualRepoPathChange,
    manualValidation,
    handleValidateManualPath,
    isValidating,
    isDiscovering,
    canSave,
    openDialog,
    handleConfirmLocalRepository,
  };
}

export function WorkspaceRepositoriesClient({
  workspace,
  repositories,
  isImproveWorkspace = false,
}: WorkspaceRepositoriesClientProps) {
  const { t } = useTranslation();
  const state = useWorkspaceRepositoriesPage(workspace, repositories);
  // The add-local-repository dialog reads the rest of `state` directly, so only
  // what this component renders is destructured here.
  const {
    router,
    repositoryItems,
    savedRepositoriesById,
    handleUpdateRepository,
    handleAddRepositoryScript,
    handleUpdateRepositoryScript,
    handleDeleteRepositoryScript,
    handleSaveRepository,
    handleDeleteRepository,
    openDialog,
  } = state;

  if (!workspace)
    return <WorkspaceNotFoundCard onBack={() => router.push("/settings/workspaces")} />;

  return (
    <div className="space-y-8">
      {/* No section header: the Repositories section below already carries the
          name, mark and description, and the tab strip above says which tab you
          are on. A second copy of all three read as the page repeating itself. */}
      <SettingsSection
        divided
        icon={<IconGitBranch className="h-5 w-5" />}
        title={t("workspaces:repositories")}
        description={
          isImproveWorkspace
            ? t("workspaces:repositoriesReadOnlyImprove")
            : t("workspaces:repositoriesInThisWorkspace")
        }
        action={
          isImproveWorkspace ? undefined : (
            <Button size="sm" className="cursor-pointer" onClick={openDialog}>
              {t("workspaces:addLocalRepository")}
            </Button>
          )
        }
      >
        <div className="grid gap-3">
          {repositoryItems.map((repo) => (
            <RepositoryCard
              key={repo.id}
              repository={repo}
              workspaceId={workspace.id}
              savedRepository={savedRepositoriesById.get(repo.id)}
              isRepositoryDirty={isRepositoryDirty(repo, savedRepositoriesById.get(repo.id))}
              areScriptsDirty={areRepositoryScriptsDirty(repo, savedRepositoriesById.get(repo.id))}
              autoOpen={Boolean(repo.__autoOpen)}
              readOnly={isImproveWorkspace}
              onUpdate={handleUpdateRepository}
              onAddScript={handleAddRepositoryScript}
              onUpdateScript={handleUpdateRepositoryScript}
              onDeleteScript={handleDeleteRepositoryScript}
              onSave={handleSaveRepository}
              onDelete={handleDeleteRepository}
            />
          ))}
        </div>
      </SettingsSection>
      {/* Sets group the repositories listed above, so they belong on this page
          rather than on a tab of their own. */}
      <WorkspaceRepositorySetsSection
        workspaceId={workspace.id}
        // Keep the member picker in sync with repository edits on this page,
        // while excluding temporary rows that do not exist server-side.
        repositories={persistedRepositoryItems(repositoryItems)}
        readOnly={isImproveWorkspace}
      />
      {!isImproveWorkspace && <AddLocalRepositoryDialog state={state} />}
    </div>
  );
}
