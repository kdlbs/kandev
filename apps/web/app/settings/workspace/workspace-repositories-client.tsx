"use client";

import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useRouter } from "@/lib/routing/client-router";
import { WorkspaceRepositoriesSection } from "./workspace-repositories-section";
import { WorkspaceRepositorySetsSection } from "./workspace-repository-sets-section";
import { AddLocalRepositoryDialog } from "./workspace-add-local-repository-dialog";
import { AddRemoteRepositoryDialog } from "./workspace-add-remote-repository-dialog";
import { generateUUID } from "@/lib/utils";
import { deleteRepositoryAction, validateRepositoryPathAction } from "@/app/actions/workspaces";
import { useRepositoryDiscovery } from "@/hooks/domains/workspace/use-repository-discovery";
import {
  repositoryId as toRepositoryId,
  type LocalRepository,
  type Repository,
  type RepositoryScript,
  type Workspace,
} from "@/lib/types/http";
import { useAppStore } from "@/components/state-provider";
import type { ManualValidation } from "@/app/settings/workspace/workspace-repositories-dialog";
import { WorkspaceNotFoundCard } from "@/app/settings/workspace/workspace-not-found-card";
import {
  cloneRepository,
  mergeSavedRepositoryDraft,
  persistedRepositoryItems,
  type RepositoryWithScripts,
} from "@/app/settings/workspace/workspace-repositories-dirty";
import { defaultWorktreeBranchTemplate } from "@/lib/worktree-branch-template";
import { saveRepositoryDraft } from "./workspace-repositories-save";
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

function useRepositorySaveHandlers({
  workspace,
  repositoryItems,
  setRepositoryItems,
  setSavedRepositoryItems,
  savedRepositoriesById,
  clearRepositoryScripts,
}: RepoHandlerArgs) {
  const pendingSaves = useRef(new Map<string, Promise<void>>());
  const handleSaveRepository = (repoId: string): Promise<void> => {
    const pending = pendingSaves.current.get(repoId);
    if (pending) return pending;
    const repo = repositoryItems.find((item) => item.id === repoId);
    if (!repo) return Promise.resolve();
    const submitted = cloneRepository(repo);
    const save = Promise.resolve()
      .then(() =>
        saveRepositoryDraft(
          submitted,
          savedRepositoriesById.get(repoId),
          workspace?.id ?? repo.workspace_id,
          (baseline, draft) => {
            setRepositoryItems((prev) =>
              prev.map((item) =>
                item.id === repoId ? mergeSavedRepositoryDraft(item, submitted, draft) : item,
              ),
            );
            setSavedRepositoryItems((prev) => [
              ...prev.filter((item) => item.id !== repoId && item.id !== baseline.id),
              cloneRepository(baseline),
            ]);
            clearRepositoryScripts(baseline.id);
          },
        ),
      )
      .finally(() => {
        pendingSaves.current.delete(repoId);
      });
    pendingSaves.current.set(repoId, save);
    return save;
  };
  const handleDiscardRepository = (repoId: string) => {
    const saved = savedRepositoriesById.get(repoId);
    setRepositoryItems((prev) =>
      saved
        ? prev.map((item) => (item.id === repoId ? { ...item, ...cloneRepository(saved) } : item))
        : prev.filter((item) => item.id !== repoId),
    );
  };
  return { handleSaveRepository, handleDiscardRepository };
}

function useRepositoryHandlers(args: RepoHandlerArgs) {
  const { repositoryItems, setRepositoryItems, setSavedRepositoryItems } = args;
  const saveHandlers = useRepositorySaveHandlers(args);
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
    ...saveHandlers,
    handleDeleteRepository,
  };
}

type LocalRepositoryDialogState = {
  workspaceId: string | null;
  context: symbol;
  attempt: symbol | null;
  open: boolean;
  selectedRepoPath: string | null;
  manualRepoPath: string;
  manualValidation: ManualValidation;
};

function emptyRepositoryDialog(workspaceId: string | null): LocalRepositoryDialogState {
  return {
    workspaceId,
    context: Symbol(),
    attempt: null,
    open: false,
    selectedRepoPath: null,
    manualRepoPath: "",
    manualValidation: { status: "idle" },
  };
}

async function validateManualPath(
  workspaceId: string,
  path: string,
  t: TFunction,
): Promise<ManualValidation> {
  try {
    const result = await validateRepositoryPathAction(workspaceId, path);
    const isValid = isValidManualRepository(result);
    return {
      status: isValid ? "success" : "error",
      isValid,
      path: result.path,
      // Backend diagnostics are data; only missing-message fallbacks are translated.
      message: isValid
        ? t("workspaces:validGitRepository")
        : result.message || t("workspaces:invalidRepositoryPath"),
    };
  } catch (error) {
    return {
      status: "error",
      isValid: false,
      message: error instanceof Error ? error.message : t("common:requestFailed"),
    };
  }
}

function useLocalRepositorySelection(workspaceId: string | null, t: TFunction) {
  const [state, setState] = useState(() => emptyRepositoryDialog(workspaceId));
  const owner = useRef<LocalRepositoryDialogState | null>(null);
  useLayoutEffect(() => {
    const next = emptyRepositoryDialog(workspaceId);
    owner.current = next;
    setState(next);
    return () => {
      owner.current = null;
    };
  }, [workspaceId]);
  const view = state.workspaceId === workspaceId ? state : emptyRepositoryDialog(workspaceId);
  const current = () => (owner.current?.context === view.context ? owner.current : null);
  const publish = (next: LocalRepositoryDialogState) => {
    owner.current = next;
    setState(next);
  };
  const setLocalRepoDialogOpen = (open: boolean) => {
    if (!current()) return;
    publish({ ...emptyRepositoryDialog(workspaceId), open });
  };
  const changeSelection = (manualRepoPath: string, selectedRepoPath: string | null) => {
    const active = current();
    if (!active || !active.open) return;
    const unchanged =
      active.manualRepoPath.trim() === manualRepoPath.trim() &&
      active.selectedRepoPath === selectedRepoPath;
    publish({
      ...active,
      manualRepoPath,
      selectedRepoPath,
      ...(unchanged
        ? {}
        : { context: Symbol(), attempt: null, manualValidation: { status: "idle" as const } }),
    });
  };
  const handleValidateManualPath = async () => {
    const active = current();
    if (!active?.open || !active.workspaceId || !active.manualRepoPath.trim()) return;
    const attempt = Symbol();
    publish({ ...active, attempt, manualValidation: { status: "loading" } });
    const manualValidation = await validateManualPath(
      active.workspaceId,
      active.manualRepoPath.trim(),
      t,
    );
    const latest = current();
    if (latest?.attempt === attempt) publish({ ...latest, manualValidation });
  };
  const isValid =
    view.manualValidation.status === "success" &&
    view.manualValidation.isValid === true &&
    Boolean(view.manualRepoPath.trim());
  const canSave = view.open && Boolean(workspaceId) && (Boolean(view.selectedRepoPath) || isValid);
  const getConfirmedSelection = () => {
    const active = current();
    if (
      !canSave ||
      !active ||
      active.attempt !== view.attempt ||
      active.manualValidation !== view.manualValidation
    )
      return null;
    return {
      selectedRepoPath: active.selectedRepoPath,
      manualValidation: active.manualValidation,
      manualRepoPath: active.manualRepoPath,
    };
  };
  return {
    localRepoDialogOpen: view.open,
    setLocalRepoDialogOpen,
    canSave,
    getConfirmedSelection,
    selectedRepoPath: view.selectedRepoPath,
    manualRepoPath: view.manualRepoPath,
    manualValidation: view.manualValidation,
    handleValidateManualPath,
    isValidating: view.open && view.manualValidation.status === "loading",
    handleSelectRepoPath: (path: string) => changeSelection("", path),
    handleManualRepoPathChange: (value: string) => changeSelection(value, null),
  };
}

function useDiscoverDialog(workspace: Workspace | null, t: TFunction) {
  const selection = useLocalRepositorySelection(workspace?.id ?? null, t);
  const [repoSearch, setRepoSearch] = useState("");
  const discovery = useRepositoryDiscovery(workspace?.id ?? null, selection.localRepoDialogOpen);
  const discoveredRepositories = discovery.repositories;
  const filteredRepositories = useMemo(() => {
    const query = repoSearch.trim().toLowerCase();
    return query
      ? discoveredRepositories.filter(
          (repo) =>
            repo.name.toLowerCase().includes(query) || repo.path.toLowerCase().includes(query),
        )
      : discoveredRepositories;
  }, [discoveredRepositories, repoSearch]);
  const openDialog = () => {
    selection.setLocalRepoDialogOpen(true);
    setRepoSearch("");
  };
  return {
    ...selection,
    filteredRepositories,
    repoSearch,
    setRepoSearch,
    openDialog,
    isDiscovering: discovery.isLoading || discovery.isRefreshing,
    discoveredRepositories,
    desktopRuntime: discovery.desktopRuntime,
    onRefreshDiscovery: () => {
      if (workspace) void discovery.refresh();
    },
    workspaceId: workspace?.id ?? null,
  };
}
export function useWorkspaceRepositoriesPage(
  workspace: Workspace | null,
  repositories: RepositoryWithScripts[],
) {
  const router = useRouter();
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
    handleDiscardRepository,
    handleDeleteRepository,
  } = handlers;

  const discover = useDiscoverDialog(workspace, t);
  const handleConfirmLocalRepository = () => {
    const selection = discover.getConfirmedSelection();
    if (!workspace || !selection) return;
    const selectedRepo = discover.discoveredRepositories.find(
      (repo) => repo.path === selection.selectedRepoPath,
    );
    if (selection.selectedRepoPath && !selectedRepo) return;
    const draftRepo = buildDraftRepo(
      workspace,
      selectedRepo,
      selection.manualValidation,
      selection.manualRepoPath,
    );
    if (!draftRepo.local_path) return;
    setRepositoryItems((prev) => [draftRepo, ...prev]);
    discover.setLocalRepoDialogOpen(false);
  };

  const [remoteRepoDialogOpen, setRemoteRepoDialogOpen] = useState(false);
  // A remote repository is saved by the backend before it reaches the page, so
  // it joins the saved baseline directly instead of becoming an unsaved draft.
  // A repository the page already lists keeps its loaded baseline and scripts;
  // the registration response never carries scripts.
  const handleRemoteRepositoryRegistered = (repository: Repository) => {
    const saved: RepositoryWithScripts = { ...repository, scripts: [] };
    setSavedRepositoryItems((prev) =>
      prev.some((item) => item.id === saved.id) ? prev : [cloneRepository(saved), ...prev],
    );
    setRepositoryItems((prev) =>
      prev.some((item) => item.id === saved.id) ? prev : [{ ...saved, __autoOpen: true }, ...prev],
    );
  };

  return {
    remoteRepoDialogOpen,
    setRemoteRepoDialogOpen,
    handleRemoteRepositoryRegistered,
    router,
    repositoryItems,
    savedRepositoriesById,
    handleUpdateRepository,
    handleAddRepositoryScript,
    handleUpdateRepositoryScript,
    handleDeleteRepositoryScript,
    handleSaveRepository,
    handleDiscardRepository,
    handleDeleteRepository,
    ...discover,
    handleConfirmLocalRepository,
  };
}

export function WorkspaceRepositoriesClient({
  workspace,
  repositories,
  isImproveWorkspace = false,
}: WorkspaceRepositoriesClientProps) {
  const state = useWorkspaceRepositoriesPage(workspace, repositories);
  // The add-local-repository dialog reads the rest of `state` directly, so only
  // what this component renders is destructured here.
  const {
    router,
    repositoryItems,
    remoteRepoDialogOpen,
    setRemoteRepoDialogOpen,
    handleRemoteRepositoryRegistered,
  } = state;

  if (!workspace)
    return <WorkspaceNotFoundCard onBack={() => router.push("/settings/workspaces")} />;

  return (
    <div className="space-y-8">
      {/* No section header: the Repositories section below already carries the
          name, mark and description, and the tab strip above says which tab you
          are on. A second copy of all three read as the page repeating itself. */}
      <WorkspaceRepositoriesSection
        workspaceId={workspace.id}
        readOnly={isImproveWorkspace}
        state={state}
      />
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
      {!isImproveWorkspace && (
        <AddRemoteRepositoryDialog
          open={remoteRepoDialogOpen}
          onOpenChange={setRemoteRepoDialogOpen}
          workspaceId={workspace.id}
          onRegistered={handleRemoteRepositoryRegistered}
        />
      )}
    </div>
  );
}
