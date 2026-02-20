"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { IconGitBranch } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { RepositoryCard } from "@/components/settings/repository-card";
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
import type { LocalRepository, Repository, RepositoryScript, Workspace } from "@/lib/types/http";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import {
  DiscoverRepoDialog,
  type ManualValidation,
} from "@/app/settings/workspace/workspace-repositories-dialog";
import { WorkspaceNotFoundCard } from "@/app/settings/workspace/workspace-not-found-card";

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };
type RepositoryItem = RepositoryWithScripts & { __autoOpen?: boolean };
type WorkspaceRepositoriesClientProps = {
  workspace: Workspace | null;
  repositories: RepositoryWithScripts[];
};

function cloneRepository(repo: RepositoryWithScripts): RepositoryWithScripts {
  return { ...repo, scripts: repo.scripts.map((script) => ({ ...script })) };
}

function isRepositoryDirty(
  repo: RepositoryItem,
  saved: RepositoryWithScripts | undefined,
): boolean {
  if (!saved) return true;
  return (
    repo.name !== saved.name ||
    repo.source_type !== saved.source_type ||
    repo.local_path !== saved.local_path ||
    repo.provider !== saved.provider ||
    repo.provider_repo_id !== saved.provider_repo_id ||
    repo.provider_owner !== saved.provider_owner ||
    repo.provider_name !== saved.provider_name ||
    repo.default_branch !== saved.default_branch ||
    repo.worktree_branch_prefix !== saved.worktree_branch_prefix ||
    repo.pull_before_worktree !== saved.pull_before_worktree ||
    repo.setup_script !== saved.setup_script ||
    repo.cleanup_script !== saved.cleanup_script ||
    repo.dev_script !== saved.dev_script
  );
}

function areRepositoryScriptsDirty(
  repo: RepositoryItem,
  saved: RepositoryWithScripts | undefined,
): boolean {
  if (!saved) return repo.scripts.length > 0;
  if (repo.scripts.length !== saved.scripts.length) return true;
  const savedScripts = new Map(saved.scripts.map((script) => [script.id, script]));
  for (const script of repo.scripts) {
    const savedScript = savedScripts.get(script.id);
    if (
      !savedScript ||
      script.name !== savedScript.name ||
      script.command !== savedScript.command ||
      script.position !== savedScript.position
    )
      return true;
  }
  return false;
}

function buildDraftRepo(
  workspace: Workspace,
  selectedRepo: LocalRepository | undefined,
  manualValidation: ManualValidation,
  manualRepoPath: string,
): RepositoryItem {
  const path = selectedRepo?.path ?? manualValidation.path ?? manualRepoPath.trim();
  const name =
    selectedRepo?.name ?? path.split("/").filter(Boolean).slice(-1)[0] ?? "New Repository";
  return {
    id: `temp-repo-${generateUUID()}`,
    workspace_id: workspace.id,
    name,
    source_type: "local",
    local_path: path,
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: selectedRepo?.default_branch ?? "",
    worktree_branch_prefix: "feature/",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
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
    name: repo.name.trim() || "New Repository",
    source_type: repo.source_type || "local",
    local_path: repo.local_path,
    provider: repo.provider,
    provider_repo_id: repo.provider_repo_id,
    provider_owner: repo.provider_owner,
    provider_name: repo.provider_name,
    default_branch: repo.default_branch,
    worktree_branch_prefix: repo.worktree_branch_prefix,
    pull_before_worktree: repo.pull_before_worktree,
    setup_script: repo.setup_script,
    cleanup_script: repo.cleanup_script,
    dev_script: repo.dev_script,
  });
  const scripts = await Promise.all(
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
  setRepositoryItems((prev) => prev.map((item) => (item.id === repoId ? nextRepo : item)));
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
    provider_owner: repo.provider_owner,
    provider_name: repo.provider_name,
    default_branch: repo.default_branch,
    worktree_branch_prefix: repo.worktree_branch_prefix,
    pull_before_worktree: repo.pull_before_worktree,
    setup_script: repo.setup_script,
    cleanup_script: repo.cleanup_script,
    dev_script: repo.dev_script,
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
  setRepositoryItems((prev) => prev.map((item) => (item.id === repoId ? nextRepo : item)));
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
      repository_id: repoId,
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
        title: "Failed to discover repositories",
        description: error instanceof Error ? error.message : "Request failed",
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
      if (result.allowed && result.exists && result.is_git)
        setManualValidation({
          status: "success",
          isValid: true,
          message: "Valid git repository",
          path: result.path,
        });
      else
        setManualValidation({
          status: "error",
          isValid: false,
          message: result.message || "Invalid repository path",
          path: result.path,
        });
    } catch (error) {
      setManualValidation({
        status: "error",
        isValid: false,
        message: error instanceof Error ? error.message : "Request failed",
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
function useWorkspaceRepositoriesPage(
  workspace: Workspace | null,
  repositories: RepositoryWithScripts[],
) {
  const router = useRouter();
  const { toast } = useToast();
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

  const discover = useDiscoverDialog(workspace, toast);
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
}: WorkspaceRepositoriesClientProps) {
  const state = useWorkspaceRepositoriesPage(workspace, repositories);
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
  } = state;

  if (!workspace)
    return <WorkspaceNotFoundCard onBack={() => router.push("/settings/workspace")} />;

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">{workspace.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Manage repositories connected to this workspace.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`/settings/workspace/${workspace.id}`}>Workspace settings</Link>
        </Button>
      </div>
      <Separator />
      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title="Repositories"
        description="Repositories in this workspace"
        action={
          <Button size="sm" className="cursor-pointer" onClick={openDialog}>
            Add Local Repository
          </Button>
        }
      >
        <div className="grid gap-3">
          {repositoryItems.map((repo) => (
            <RepositoryCard
              key={repo.id}
              repository={repo}
              isRepositoryDirty={isRepositoryDirty(repo, savedRepositoriesById.get(repo.id))}
              areScriptsDirty={areRepositoryScriptsDirty(repo, savedRepositoriesById.get(repo.id))}
              autoOpen={Boolean(repo.__autoOpen)}
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
      <DiscoverRepoDialog
        open={localRepoDialogOpen}
        onOpenChange={setLocalRepoDialogOpen}
        isLoading={isDiscovering}
        filteredRepositories={filteredRepositories}
        repoSearch={repoSearch}
        onRepoSearchChange={setRepoSearch}
        selectedRepoPath={selectedRepoPath}
        onSelectRepoPath={handleSelectRepoPath}
        manualRepoPath={manualRepoPath}
        onManualRepoPathChange={handleManualRepoPathChange}
        manualValidation={manualValidation}
        onValidateManualPath={handleValidateManualPath}
        isValidating={isValidating}
        canSave={canSave}
        onConfirm={handleConfirmLocalRepository}
      />
    </div>
  );
}
