"use client";

import { useEffect, useRef, useState, useMemo, useCallback } from "react";
import type {
  LocalRepository,
  Workspace,
  Repository,
  Environment,
  Executor,
  Branch,
} from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { DEFAULT_LOCAL_ENVIRONMENT_KIND, DEFAULT_LOCAL_EXECUTOR_TYPE } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useRepositoryBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import {
  useRepositoryOptions,
  useBranchOptions,
  useAgentProfileOptions,
  useExecutorOptions,
  useExecutorHint,
} from "@/components/task-create-dialog-options";
import { useToast } from "@/components/toast-provider";
import {
  discoverRepositoriesAction,
  listLocalRepositoryBranchesAction,
} from "@/app/actions/workspaces";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import type {
  StepType,
  TaskCreateDialogInitialValues,
  DialogFormState,
  StoreSelections,
  DialogComputedValues,
  DialogComputedArgs,
  TaskCreateEffectsArgs,
} from "@/components/task-create-dialog-types";
import { autoSelectBranch, computePassthroughProfile, computeEffectiveStepId } from "@/components/task-create-dialog-helpers";

export type { StepType, TaskCreateDialogInitialValues } from "@/components/task-create-dialog-types";
export { autoSelectBranch } from "@/components/task-create-dialog-helpers";

function useCreationStatusState() {
  const [isCreatingSession, setIsCreatingSession] = useState(false);
  const [isCreatingTask, setIsCreatingTask] = useState(false);
  return { isCreatingSession, setIsCreatingSession, isCreatingTask, setIsCreatingTask };
}

type FormResetters = {
  setTaskName: (v: string) => void;
  setHasTitle: (v: boolean) => void;
  setHasDescription: (v: boolean) => void;
  setRepositoryId: (v: string) => void;
  setBranch: (v: string) => void;
  setAgentProfileId: (v: string) => void;
  setEnvironmentId: (v: string) => void;
  setExecutorId: (v: string) => void;
  setSelectedWorkflowId: (v: string | null) => void;
  setFetchedSteps: (v: StepType[] | null) => void;
  setDiscoveredRepositories: (v: LocalRepository[]) => void;
  setDiscoveredRepoPath: (v: string) => void;
  setSelectedLocalRepo: (v: LocalRepository | null) => void;
  setLocalBranches: (v: Branch[]) => void;
  setDiscoverReposLoaded: (v: boolean) => void;
};

function useFormResetEffects(
  open: boolean,
  workspaceId: string | null,
  workflowId: string | null,
  initialValues: TaskCreateDialogInitialValues | undefined,
  resetters: FormResetters,
) {
  useEffect(() => {
    if (!open) return;
    const name = initialValues?.title || "";
    void Promise.resolve().then(() => {
      resetters.setTaskName(name);
      resetters.setHasTitle(name.trim().length > 0);
      resetters.setHasDescription(Boolean(initialValues?.description?.trim()));
      resetters.setRepositoryId(initialValues?.repositoryId ?? "");
      resetters.setBranch(initialValues?.branch ?? "");
      resetters.setAgentProfileId("");
      resetters.setEnvironmentId("");
      resetters.setExecutorId("");
      resetters.setSelectedWorkflowId(workflowId);
      resetters.setFetchedSteps(null);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    initialValues?.branch,
    initialValues?.description,
    initialValues?.repositoryId,
    initialValues?.title,
    open,
    workflowId,
  ]);
  useEffect(() => {
    if (!open) return;
    void Promise.resolve().then(() => {
      resetters.setDiscoveredRepositories([]);
      resetters.setDiscoveredRepoPath("");
      resetters.setSelectedLocalRepo(null);
      resetters.setLocalBranches([]);
      resetters.setDiscoverReposLoaded(false);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, workspaceId]);
}

export function useDialogFormState(
  open: boolean,
  workspaceId: string | null,
  workflowId: string | null,
  initialValues?: TaskCreateDialogInitialValues,
) {
  const [taskName, setTaskName] = useState("");
  const [hasTitle, setHasTitle] = useState(Boolean(initialValues?.title?.trim()));
  const [hasDescription, setHasDescription] = useState(Boolean(initialValues?.description?.trim()));
  const descriptionInputRef = useRef<{ getValue: () => string } | null>(null);
  const [repositoryId, setRepositoryId] = useState(initialValues?.repositoryId ?? "");
  const [branch, setBranch] = useState(initialValues?.branch ?? "");
  const [agentProfileId, setAgentProfileId] = useState("");
  const [environmentId, setEnvironmentId] = useState("");
  const [executorId, setExecutorId] = useState("");
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [discoveredRepoPath, setDiscoveredRepoPath] = useState("");
  const [selectedLocalRepo, setSelectedLocalRepo] = useState<LocalRepository | null>(null);
  const [localBranches, setLocalBranches] = useState<Branch[]>([]);
  const [localBranchesLoading, setLocalBranchesLoading] = useState(false);
  const [discoverReposLoading, setDiscoverReposLoading] = useState(false);
  const [discoverReposLoaded, setDiscoverReposLoaded] = useState(false);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState(workflowId);
  const [fetchedSteps, setFetchedSteps] = useState<StepType[] | null>(null);
  const { isCreatingSession, setIsCreatingSession, isCreatingTask, setIsCreatingTask } =
    useCreationStatusState();
  useFormResetEffects(open, workspaceId, workflowId, initialValues, {
    setTaskName,
    setHasTitle,
    setHasDescription,
    setRepositoryId,
    setBranch,
    setAgentProfileId,
    setEnvironmentId,
    setExecutorId,
    setSelectedWorkflowId,
    setFetchedSteps,
    setDiscoveredRepositories,
    setDiscoveredRepoPath,
    setSelectedLocalRepo,
    setLocalBranches,
    setDiscoverReposLoaded,
  });
  return {
    taskName,
    setTaskName,
    hasTitle,
    setHasTitle,
    hasDescription,
    setHasDescription,
    descriptionInputRef,
    repositoryId,
    setRepositoryId,
    branch,
    setBranch,
    agentProfileId,
    setAgentProfileId,
    environmentId,
    setEnvironmentId,
    executorId,
    setExecutorId,
    discoveredRepositories,
    setDiscoveredRepositories,
    discoveredRepoPath,
    setDiscoveredRepoPath,
    selectedLocalRepo,
    setSelectedLocalRepo,
    localBranches,
    setLocalBranches,
    localBranchesLoading,
    setLocalBranchesLoading,
    discoverReposLoading,
    setDiscoverReposLoading,
    discoverReposLoaded,
    setDiscoverReposLoaded,
    selectedWorkflowId,
    setSelectedWorkflowId,
    fetchedSteps,
    setFetchedSteps,
    isCreatingSession,
    setIsCreatingSession,
    isCreatingTask,
    setIsCreatingTask,
  };
}

export type { DialogFormState } from "@/components/task-create-dialog-types";

export function useWorkflowStepsEffect(fs: DialogFormState, workflowId: string | null) {
  const { selectedWorkflowId, setFetchedSteps } = fs;
  useEffect(() => {
    if (!selectedWorkflowId || selectedWorkflowId === workflowId) {
      void Promise.resolve().then(() => setFetchedSteps(null));
      return;
    }
    let cancelled = false;
    listWorkflowSteps(selectedWorkflowId)
      .then((response) => {
        if (cancelled) return;
        const sorted = [...response.steps].sort((a, b) => a.position - b.position);
        setFetchedSteps(sorted.map((s) => ({ id: s.id, title: s.name, events: s.events })));
      })
      .catch(() => {
        if (!cancelled) setFetchedSteps(null);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedWorkflowId, workflowId, setFetchedSteps]);
}

export function useRepositoryAutoSelectEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  repositories: Repository[],
) {
  const { repositoryId, selectedLocalRepo, setRepositoryId } = fs;
  useEffect(() => {
    if (!open || !workspaceId || repositoryId || selectedLocalRepo) return;
    const lastUsedRepoId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null);
    if (lastUsedRepoId && repositories.some((r: Repository) => r.id === lastUsedRepoId)) {
      void Promise.resolve().then(() => setRepositoryId(lastUsedRepoId));
      return;
    }
    if (repositories.length === 1)
      void Promise.resolve().then(() => setRepositoryId(repositories[0].id));
  }, [open, repositories, repositoryId, selectedLocalRepo, workspaceId, setRepositoryId]);
}

export function useDiscoverReposEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  repositoriesLoading: boolean,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const {
    discoverReposLoaded,
    discoverReposLoading,
    setDiscoveredRepositories,
    setDiscoverReposLoading,
    setDiscoverReposLoaded,
  } = fs;
  useEffect(() => {
    if (!open || !workspaceId || repositoriesLoading || discoverReposLoaded || discoverReposLoading)
      return;
    void Promise.resolve()
      .then(() => setDiscoverReposLoading(true))
      .then(() => discoverRepositoriesAction(workspaceId))
      .then((r) => {
        setDiscoveredRepositories(r.repositories);
      })
      .catch((e) => {
        toast({
          title: "Failed to discover repositories",
          description: e instanceof Error ? e.message : "Request failed",
          variant: "error",
        });
        setDiscoveredRepositories([]);
      })
      .finally(() => {
        setDiscoverReposLoading(false);
        setDiscoverReposLoaded(true);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    discoverReposLoaded,
    discoverReposLoading,
    open,
    fs.discoveredRepositories.length,
    repositoriesLoading,
    toast,
    workspaceId,
  ]);
}

export function useLocalBranchesEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const { selectedLocalRepo, setLocalBranches, setLocalBranchesLoading } = fs;
  useEffect(() => {
    if (!open || !workspaceId || !selectedLocalRepo) return;
    const repoPath = selectedLocalRepo.path;
    void Promise.resolve()
      .then(() => setLocalBranchesLoading(true))
      .then(() => listLocalRepositoryBranchesAction(workspaceId, repoPath))
      .then((r) => {
        setLocalBranches(r.branches);
      })
      .catch((e) => {
        toast({
          title: "Failed to load branches",
          description: e instanceof Error ? e.message : "Request failed",
          variant: "error",
        });
        setLocalBranches([]);
      })
      .finally(() => {
        setLocalBranchesLoading(false);
      });
  }, [open, selectedLocalRepo, toast, workspaceId, setLocalBranches, setLocalBranchesLoading]);
}

export function useDefaultSelectionsEffect(
  fs: DialogFormState,
  open: boolean,
  sel: StoreSelections,
) {
  const { agentProfiles, environments, executors, workspaceDefaults } = sel;
  const {
    agentProfileId,
    environmentId,
    executorId,
    setAgentProfileId,
    setEnvironmentId,
    setExecutorId,
  } = fs;
  useEffect(() => {
    if (!open || agentProfileId || agentProfiles.length === 0) return;
    const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, null);
    if (lastId && agentProfiles.some((p: AgentProfileOption) => p.id === lastId)) {
      void Promise.resolve().then(() => setAgentProfileId(lastId));
      return;
    }
    const defId = workspaceDefaults?.default_agent_profile_id ?? null;
    if (defId && agentProfiles.some((p: AgentProfileOption) => p.id === defId)) {
      void Promise.resolve().then(() => setAgentProfileId(defId));
      return;
    }
    void Promise.resolve().then(() => setAgentProfileId(agentProfiles[0].id));
  }, [open, agentProfileId, agentProfiles, workspaceDefaults, setAgentProfileId]);

  useEffect(() => {
    if (!open || environmentId || environments.length === 0) return;
    const defId = workspaceDefaults?.default_environment_id ?? null;
    if (defId && environments.some((e: Environment) => e.id === defId)) {
      void Promise.resolve().then(() => setEnvironmentId(defId));
      return;
    }
    const local = environments.find((e: Environment) => e.kind === DEFAULT_LOCAL_ENVIRONMENT_KIND);
    void Promise.resolve().then(() => setEnvironmentId(local?.id ?? environments[0].id));
  }, [open, environmentId, environments, workspaceDefaults, setEnvironmentId]);

  useEffect(() => {
    if (!open || executorId || executors.length === 0) return;
    const defId = workspaceDefaults?.default_executor_id ?? null;
    if (defId && executors.some((e: Executor) => e.id === defId)) {
      void Promise.resolve().then(() => setExecutorId(defId));
      return;
    }
    const local = executors.find((e: Executor) => e.type === DEFAULT_LOCAL_EXECUTOR_TYPE);
    void Promise.resolve().then(() => setExecutorId(local?.id ?? executors[0].id));
  }, [open, executorId, executors, workspaceDefaults, setExecutorId]);
}

export function useBranchAutoSelectEffect(fs: DialogFormState, branches: Branch[]) {
  const { repositoryId, branch, localBranches, setBranch } = fs;
  useEffect(() => {
    if (!repositoryId || branch) return;
    autoSelectBranch(branches, setBranch);
  }, [branch, branches, repositoryId, setBranch]);
  useEffect(() => {
    if (repositoryId || localBranches.length === 0 || branch) return;
    autoSelectBranch(localBranches, setBranch);
  }, [branch, localBranches, repositoryId, setBranch]);
}

export function useDialogHandlers(fs: DialogFormState, repositories: Repository[]) {
  const handleSelectLocalRepository = useCallback(
    (path: string) => {
      fs.setDiscoveredRepoPath(path);
      fs.setSelectedLocalRepo(fs.discoveredRepositories.find((r) => r.path === path) ?? null);
      fs.setRepositoryId("");
      fs.setBranch("");
      fs.setLocalBranches([]);
    },
    [fs],
  );

  const handleRepositoryChange = useCallback(
    (value: string) => {
      if (repositories.find((r: Repository) => r.id === value)) {
        fs.setRepositoryId(value);
        setLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID, value);
        fs.setDiscoveredRepoPath("");
        fs.setSelectedLocalRepo(null);
        fs.setLocalBranches([]);
        fs.setBranch("");
        return;
      }
      handleSelectLocalRepository(value);
    },
    [repositories, fs, handleSelectLocalRepository],
  );

  const handleAgentProfileChange = useCallback(
    (value: string) => {
      fs.setAgentProfileId(value);
      setLocalStorage(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, value);
    },
    [fs],
  );
  const handleTaskNameChange = useCallback(
    (value: string) => {
      fs.setTaskName(value);
      fs.setHasTitle(value.trim().length > 0);
    },
    [fs],
  );
  const handleBranchChange = useCallback(
    (value: string) => {
      fs.setBranch(value);
      setLocalStorage(STORAGE_KEYS.LAST_BRANCH, value);
    },
    [fs],
  );
  const handleWorkflowChange = useCallback(
    (value: string) => {
      fs.setSelectedWorkflowId(value);
    },
    [fs],
  );

  return {
    handleRepositoryChange,
    handleAgentProfileChange,
    handleTaskNameChange,
    handleBranchChange,
    handleWorkflowChange,
  };
}

export {
  computePassthroughProfile,
  computeEffectiveStepId,
  computeIsTaskStarted,
} from "@/components/task-create-dialog-helpers";

export function useDialogComputed({
  fs,
  open,
  workspaceId,
  workflowId,
  defaultStepId,
  branches,
  settingsData,
  agentProfiles,
  workspaces,
  executors,
  repositories,
}: DialogComputedArgs): DialogComputedValues {
  const isPassthroughProfile = useMemo(
    () => computePassthroughProfile(fs.agentProfileId, agentProfiles),
    [fs.agentProfileId, agentProfiles],
  );
  const effectiveWorkflowId = fs.selectedWorkflowId ?? workflowId;
  const effectiveDefaultStepId = computeEffectiveStepId(
    fs.selectedWorkflowId,
    workflowId,
    fs.fetchedSteps,
    defaultStepId,
  );
  const workspaceDefaults = workspaceId
    ? workspaces.find((ws: Workspace) => ws.id === workspaceId)
    : null;
  const hasRepositorySelection = Boolean(fs.repositoryId || fs.selectedLocalRepo);
  const branchOptions = useBranchOptions(fs.repositoryId ? branches : fs.localBranches);
  const agentProfileOptions = useAgentProfileOptions(agentProfiles);
  const executorOptions = useExecutorOptions(executors);
  const executorHint = useExecutorHint(executors, fs.executorId);
  const { headerRepositoryOptions } = useRepositoryOptions(repositories, fs.discoveredRepositories);
  const agentProfilesLoading = open && !settingsData.agentsLoaded;
  const executorsLoading = open && !settingsData.executorsLoaded;
  return {
    isPassthroughProfile,
    effectiveWorkflowId,
    effectiveDefaultStepId,
    workspaceDefaults,
    hasRepositorySelection,
    branchOptions,
    agentProfileOptions,
    executorOptions,
    executorHint,
    headerRepositoryOptions,
    agentProfilesLoading,
    executorsLoading,
  };
}

export function useSessionRepoName(isSessionMode: boolean) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);
  const reposByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  return useMemo(() => {
    if (!isSessionMode) return undefined;
    const activeTask = activeTaskId ? kanbanTasks.find((t) => t.id === activeTaskId) : null;
    const repoId = activeTask?.repositoryId;
    if (!repoId) return undefined;
    for (const repos of Object.values(reposByWorkspace)) {
      const repo = repos.find((r) => r.id === repoId);
      if (repo) return repo.name;
    }
    return undefined;
  }, [isSessionMode, activeTaskId, kanbanTasks, reposByWorkspace]);
}

export function useTaskCreateDialogData(
  open: boolean,
  workspaceId: string | null,
  workflowId: string | null,
  defaultStepId: string | null,
  fs: DialogFormState,
) {
  const workflows = useAppStore((state) => state.workflows.items);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const environments = useAppStore((state) => state.environments.items);
  const settingsData = useAppStore((state) => state.settingsData);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);

  useSettingsData(open);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, open);
  const { branches, isLoading: branchesLoading } = useRepositoryBranches(
    fs.repositoryId || null,
    Boolean(open && fs.repositoryId),
  );
  const computed = useDialogComputed({
    fs,
    open,
    workspaceId,
    workflowId,
    defaultStepId,
    branches,
    settingsData,
    agentProfiles,
    workspaces,
    executors,
    repositories,
  });
  return {
    workflows,
    workspaces,
    agentProfiles,
    executors,
    environments,
    snapshots,
    repositories,
    repositoriesLoading,
    branches,
    branchesLoading,
    computed,
  };
}

export function useTaskCreateDialogEffects(fs: DialogFormState, args: TaskCreateEffectsArgs) {
  const {
    open,
    workspaceId,
    workflowId,
    repositories,
    repositoriesLoading,
    branches,
    agentProfiles,
    environments,
    executors,
    workspaceDefaults,
    toast,
  } = args;
  useWorkflowStepsEffect(fs, workflowId);
  useRepositoryAutoSelectEffect(fs, open, workspaceId, repositories);
  useDiscoverReposEffect(fs, open, workspaceId, repositoriesLoading, toast);
  useBranchAutoSelectEffect(fs, branches);
  useLocalBranchesEffect(fs, open, workspaceId, toast);
  useDefaultSelectionsEffect(fs, open, {
    agentProfiles,
    environments,
    executors,
    workspaceDefaults,
  });
}
