"use client";

import { useEffect, useRef, useState, useMemo, useCallback } from "react";
import type { LocalRepository, Workspace, ExecutorProfile, Branch } from "@/lib/types/http";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useRepositoryBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import {
  useRepositoryOptions,
  useBranchOptions,
  useAgentProfileOptions,
  useExecutorHint,
  useExecutorProfileOptions,
  useIsLocalExecutor,
} from "@/components/task-create-dialog-options";
import { getTaskCreateDraft, setTaskCreateDraft, removeTaskCreateDraft } from "@/lib/local-storage";
import type {
  StepType,
  TaskCreateDialogInitialValues,
  DialogFormState,
  DialogComputedValues,
  DialogComputedArgs,
} from "@/components/task-create-dialog-types";
import {
  computePassthroughProfile,
  computeEffectiveStepId,
} from "@/components/task-create-dialog-helpers";

export type {
  StepType,
  TaskCreateDialogInitialValues,
} from "@/components/task-create-dialog-types";
export { autoSelectBranch } from "@/components/task-create-dialog-helpers";

type FormResetters = {
  setTaskName: (v: string) => void;
  setHasTitle: (v: boolean) => void;
  setHasDescription: (v: boolean) => void;
  setRepositoryId: (v: string) => void;
  setBranch: (v: string) => void;
  setAgentProfileId: (v: string) => void;
  setExecutorId: (v: string) => void;
  setExecutorProfileId: (v: string) => void;
  setSelectedWorkflowId: (v: string | null) => void;
  setFetchedSteps: (v: StepType[] | null) => void;
  setDiscoveredRepositories: (v: LocalRepository[]) => void;
  setDiscoveredRepoPath: (v: string) => void;
  setSelectedLocalRepo: (v: LocalRepository | null) => void;
  setLocalBranches: (v: Branch[]) => void;
  setDiscoverReposLoaded: (v: boolean) => void;
  setUseGitHubUrl: (v: boolean) => void;
  setGitHubUrl: (v: string) => void;
  setGitHubBranches: (v: Branch[]) => void;
  setGitHubUrlError: (v: string | null) => void;
  setGitHubPrHeadBranch: (v: string | null) => void;
  setFreshBranchEnabled: (v: boolean) => void;
  setCurrentLocalBranch: (v: string) => void;
};

type FormResetEffectsArgs = {
  open: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  initialValues: TaskCreateDialogInitialValues | undefined;
  resetters: FormResetters;
  setDraftDescription: (v: string) => void;
  setCurrentDefaults: (v: { name: string; description: string }) => void;
  setOpenCycle: React.Dispatch<React.SetStateAction<number>>;
  prevOpenRef: React.RefObject<boolean>;
};

function useFormResetEffects({
  open,
  workspaceId,
  workflowId,
  initialValues,
  resetters,
  setDraftDescription,
  setCurrentDefaults,
  setOpenCycle,
  prevOpenRef,
}: FormResetEffectsArgs) {
  // Restore draft or initialValues when dialog opens
  useEffect(() => {
    // Only run on rising edge (dialog opening)
    const wasOpen = prevOpenRef.current;
    (prevOpenRef as React.MutableRefObject<boolean>).current = open;

    if (!open || wasOpen) return;

    // Increment cycle to force TaskFormInputs remount
    setOpenCycle((c) => c + 1);

    const defaults = resolveFormDefaults(initialValues, workspaceId);
    setCurrentDefaults(defaults);
    resetTaskForm(resetters, defaults.name, defaults.description, workflowId, initialValues);
    setDraftDescription(defaults.description);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, workflowId, workspaceId]);

  useEffect(() => {
    if (!open) return;
    resetDiscoveryState(resetters, initialValues);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, workspaceId]);
}

/** Checks if initialValues has any user-provided content */
function hasUserContent(initialValues?: TaskCreateDialogInitialValues): boolean {
  const title = initialValues?.title ?? "";
  const description = initialValues?.description ?? "";
  return title.trim().length > 0 || description.trim().length > 0;
}

/** Resolves form defaults from draft (for create) or initialValues (for edit) */
function resolveFormDefaults(
  initialValues: TaskCreateDialogInitialValues | undefined,
  workspaceId: string | null,
) {
  // In edit mode (has content), use initialValues; in create mode, try draft
  const draft =
    !hasUserContent(initialValues) && workspaceId ? getTaskCreateDraft(workspaceId) : null;
  const initTitle = initialValues?.title ?? "";
  const initDesc = initialValues?.description ?? "";
  return {
    name: draft?.title ?? initTitle,
    description: draft?.description ?? initDesc,
  };
}

/** Resets task form fields to specified values */
function resetTaskForm(
  resetters: FormResetters,
  name: string,
  description: string,
  workflowId: string | null,
  initialValues?: TaskCreateDialogInitialValues,
) {
  resetters.setTaskName(name);
  resetters.setHasTitle(name.trim().length > 0);
  resetters.setHasDescription(description.trim().length > 0);
  resetters.setRepositoryId(initialValues?.repositoryId ?? "");
  resetters.setBranch(initialValues?.branch ?? "");
  resetters.setAgentProfileId("");
  resetters.setExecutorId("");
  resetters.setExecutorProfileId("");
  resetters.setSelectedWorkflowId(workflowId);
  resetters.setFetchedSteps(null);
}

/** Resets repository discovery state */
function resetDiscoveryState(resetters: FormResetters, iv?: TaskCreateDialogInitialValues) {
  const ghUrl = iv?.githubUrl ?? "";
  resetters.setDiscoveredRepositories([]);
  resetters.setDiscoveredRepoPath("");
  resetters.setSelectedLocalRepo(null);
  resetters.setLocalBranches([]);
  resetters.setDiscoverReposLoaded(false);
  resetters.setUseGitHubUrl(Boolean(ghUrl));
  resetters.setGitHubUrl(ghUrl);
  resetters.setGitHubBranches([]);
  resetters.setGitHubUrlError(null);
  resetters.setGitHubPrHeadBranch(iv?.checkoutBranch ?? null);
  resetters.setFreshBranchEnabled(false);
  resetters.setCurrentLocalBranch("");
}

/** Hook to manage draft persistence for task creation dialog */
function useDraftPersistence(
  open: boolean,
  workspaceId: string | null,
  initialValues: TaskCreateDialogInitialValues | undefined,
  taskName: string,
  descriptionInputRef: React.RefObject<{ getValue: () => string } | null>,
) {
  const wasOpenRef = useRef(false);
  const skipDraftSaveRef = useRef(false);

  // Save draft when dialog closes (only in create mode without initialValues)
  useEffect(() => {
    const wasOpen = wasOpenRef.current;
    wasOpenRef.current = open;

    if (!wasOpen || open || !workspaceId) return;
    // Skip if clearDraft was called (successful submission)
    if (skipDraftSaveRef.current) {
      skipDraftSaveRef.current = false;
      return;
    }
    const hasInitialValues = Boolean(
      initialValues?.title?.trim() || initialValues?.description?.trim(),
    );
    // Only save draft in create mode
    if (!hasInitialValues) {
      const currentDescription = descriptionInputRef.current?.getValue() ?? "";
      setTaskCreateDraft(workspaceId, { title: taskName, description: currentDescription });
    }
  }, [open, workspaceId, initialValues, taskName, descriptionInputRef]);

  // Clear draft (call on successful submission before closing dialog)
  const clearDraft = useCallback(() => {
    if (workspaceId) {
      removeTaskCreateDraft(workspaceId);
      skipDraftSaveRef.current = true;
    }
  }, [workspaceId]);

  return { clearDraft };
}

function useWorkflowAgentProfileState() {
  const [workflowAgentProfileId, setWorkflowAgentProfileId] = useState("");
  return { workflowAgentProfileId, setWorkflowAgentProfileId };
}

function useFreshBranchState() {
  const [freshBranchEnabled, setFreshBranchEnabled] = useState(false);
  const [currentLocalBranch, setCurrentLocalBranch] = useState("");
  return {
    freshBranchEnabled,
    setFreshBranchEnabled,
    currentLocalBranch,
    setCurrentLocalBranch,
  };
}

function useGitHubUrlState() {
  const [useGitHubUrl, setUseGitHubUrl] = useState(false);
  const [githubUrl, setGitHubUrl] = useState("");
  const [githubBranches, setGitHubBranches] = useState<Branch[]>([]);
  const [githubBranchesLoading, setGitHubBranchesLoading] = useState(false);
  const [githubUrlError, setGitHubUrlError] = useState<string | null>(null);
  const [githubPrHeadBranch, setGitHubPrHeadBranch] = useState<string | null>(null);
  return {
    useGitHubUrl,
    setUseGitHubUrl,
    githubUrl,
    setGitHubUrl,
    githubBranches,
    setGitHubBranches,
    githubBranchesLoading,
    setGitHubBranchesLoading,
    githubUrlError,
    setGitHubUrlError,
    githubPrHeadBranch,
    setGitHubPrHeadBranch,
  };
}

/** Core form state declarations */
function useFormStateValues(
  workflowId: string | null,
  workspaceId: string | null,
  open: boolean,
  initialValues?: TaskCreateDialogInitialValues,
) {
  // openCycle increments each time dialog opens - used in key to force TaskFormInputs remount
  const [openCycle, setOpenCycle] = useState(0);
  // Start as false so a fresh mount with open=true is detected as a rising edge
  // (callers like QuickTaskLauncher conditionally mount the dialog already-open).
  const prevOpenRef = useRef(false);

  // currentDefaults stores the loaded draft/initial values for this open cycle
  const [currentDefaults, setCurrentDefaults] = useState<{ name: string; description: string }>({
    name: "",
    description: "",
  });

  // These states are initialized with defaults and then managed by effects/handlers
  const [taskName, setTaskName] = useState("");
  const [hasTitle, setHasTitle] = useState(false);
  const [hasDescription, setHasDescription] = useState(false);
  const [draftDescription, setDraftDescription] = useState("");

  const descriptionInputRef = useRef<TaskFormInputsHandle | null>(null);
  const [repositoryId, setRepositoryId] = useState(initialValues?.repositoryId ?? "");
  const [branch, setBranch] = useState(initialValues?.branch ?? "");
  const [agentProfileId, setAgentProfileId] = useState("");
  const [executorId, setExecutorId] = useState("");
  const [executorProfileId, setExecutorProfileId] = useState("");
  const [selectedWorkflowId, setSelectedWorkflowId] = useState(workflowId);
  const [fetchedSteps, setFetchedSteps] = useState<StepType[] | null>(null);
  const [isCreatingSession, setIsCreatingSession] = useState(false);
  const [isCreatingTask, setIsCreatingTask] = useState(false);
  return {
    taskName,
    setTaskName,
    hasTitle,
    setHasTitle,
    hasDescription,
    setHasDescription,
    draftDescription,
    setDraftDescription,
    descriptionInputRef,
    repositoryId,
    setRepositoryId,
    branch,
    setBranch,
    agentProfileId,
    setAgentProfileId,
    executorId,
    setExecutorId,
    executorProfileId,
    setExecutorProfileId,
    selectedWorkflowId,
    setSelectedWorkflowId,
    fetchedSteps,
    setFetchedSteps,
    isCreatingSession,
    setIsCreatingSession,
    isCreatingTask,
    setIsCreatingTask,
    openCycle,
    setOpenCycle,
    currentDefaults,
    setCurrentDefaults,
    prevOpenRef,
  };
}

/** Repository discovery state */
function useDiscoveryState() {
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [discoveredRepoPath, setDiscoveredRepoPath] = useState("");
  const [selectedLocalRepo, setSelectedLocalRepo] = useState<LocalRepository | null>(null);
  const [localBranches, setLocalBranches] = useState<Branch[]>([]);
  const [localBranchesLoading, setLocalBranchesLoading] = useState(false);
  const [discoverReposLoading, setDiscoverReposLoading] = useState(false);
  const [discoverReposLoaded, setDiscoverReposLoaded] = useState(false);
  return {
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
  };
}

export function useDialogFormState(
  open: boolean,
  workspaceId: string | null,
  workflowId: string | null,
  initialValues?: TaskCreateDialogInitialValues,
) {
  const form = useFormStateValues(workflowId, workspaceId, open, initialValues);
  const discovery = useDiscoveryState();
  const ghUrl = useGitHubUrlState();
  const wfAgent = useWorkflowAgentProfileState();
  const freshBranch = useFreshBranchState();

  useFormResetEffects({
    open,
    workspaceId,
    workflowId,
    initialValues,
    setDraftDescription: form.setDraftDescription,
    setCurrentDefaults: form.setCurrentDefaults,
    setOpenCycle: form.setOpenCycle,
    prevOpenRef: form.prevOpenRef,
    resetters: {
      setTaskName: form.setTaskName,
      setHasTitle: form.setHasTitle,
      setHasDescription: form.setHasDescription,
      setRepositoryId: form.setRepositoryId,
      setBranch: form.setBranch,
      setAgentProfileId: form.setAgentProfileId,
      setExecutorId: form.setExecutorId,
      setExecutorProfileId: form.setExecutorProfileId,
      setSelectedWorkflowId: form.setSelectedWorkflowId,
      setFetchedSteps: form.setFetchedSteps,
      setDiscoveredRepositories: discovery.setDiscoveredRepositories,
      setDiscoveredRepoPath: discovery.setDiscoveredRepoPath,
      setSelectedLocalRepo: discovery.setSelectedLocalRepo,
      setLocalBranches: discovery.setLocalBranches,
      setDiscoverReposLoaded: discovery.setDiscoverReposLoaded,
      setUseGitHubUrl: ghUrl.setUseGitHubUrl,
      setGitHubUrl: ghUrl.setGitHubUrl,
      setGitHubBranches: ghUrl.setGitHubBranches,
      setGitHubUrlError: ghUrl.setGitHubUrlError,
      setGitHubPrHeadBranch: ghUrl.setGitHubPrHeadBranch,
      setFreshBranchEnabled: freshBranch.setFreshBranchEnabled,
      setCurrentLocalBranch: freshBranch.setCurrentLocalBranch,
    },
  });

  const { clearDraft } = useDraftPersistence(
    open,
    workspaceId,
    initialValues,
    form.taskName,
    form.descriptionInputRef,
  );

  return { ...form, ...discovery, ...ghUrl, ...wfAgent, ...freshBranch, clearDraft };
}

export type { DialogFormState } from "@/components/task-create-dialog-types";
export {
  computePassthroughProfile,
  computeEffectiveStepId,
  computeIsTaskStarted,
} from "@/components/task-create-dialog-helpers";
export { useTaskCreateDialogEffects } from "@/components/task-create-dialog-effects";

export { useDialogHandlers } from "@/components/task-create-dialog-handlers";

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
  workflows,
}: DialogComputedArgs): DialogComputedValues {
  const effectiveWorkflowId = fs.selectedWorkflowId ?? workflowId;
  // Compute workflow agent lock directly from data — avoids effect timing issues.
  const workflowAgentProfileId = (() => {
    const wfId = effectiveWorkflowId;
    if (!wfId) return "";
    const wf = workflows.find((w) => w.id === wfId);
    return wf?.agent_profile_id ?? "";
  })();
  const workflowAgentLocked = Boolean(workflowAgentProfileId);
  // fs.agentProfileId lags behind the workflow override on dialog re-open
  // (effect deps don't change), so fall back to the synchronous value.
  const effectiveAgentProfileId = fs.agentProfileId || workflowAgentProfileId;
  const isPassthroughProfile = useMemo(
    () => computePassthroughProfile(effectiveAgentProfileId, agentProfiles),
    [effectiveAgentProfileId, agentProfiles],
  );
  const effectiveDefaultStepId = computeEffectiveStepId(
    fs.selectedWorkflowId,
    workflowId,
    fs.fetchedSteps,
    defaultStepId,
  );
  const workspaceDefaults = workspaceId
    ? workspaces.find((ws: Workspace) => ws.id === workspaceId)
    : null;
  const hasRepositorySelection = Boolean(
    fs.repositoryId || fs.selectedLocalRepo || (fs.useGitHubUrl && fs.githubUrl.trim()),
  );
  const effectiveBranches = (() => {
    if (fs.useGitHubUrl) return fs.githubBranches;
    if (fs.repositoryId) return branches;
    return fs.localBranches;
  })();
  const branchOptions = useBranchOptions(effectiveBranches);
  const agentProfileOptions = useAgentProfileOptions(agentProfiles);
  const allExecutorProfiles = useMemo<ExecutorProfile[]>(() => {
    return executors.flatMap((executor) =>
      (executor.profiles ?? []).map((p) => ({
        ...p,
        executor_type: p.executor_type ?? executor.type,
        executor_name: p.executor_name ?? executor.name,
      })),
    );
  }, [executors]);
  const executorProfileOptions = useExecutorProfileOptions(allExecutorProfiles);
  const executorHint = useExecutorHint(executors, fs.executorId);
  const isLocalExecutor = useIsLocalExecutor(executors, fs.executorId);
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
    executorProfileOptions,
    executorHint,
    isLocalExecutor,
    headerRepositoryOptions,
    agentProfilesLoading,
    executorsLoading,
    workflowAgentLocked,
    workflowAgentProfileId,
    effectiveAgentProfileId,
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
    workflows,
  });
  return {
    workflows,
    workspaces,
    agentProfiles,
    executors,
    snapshots,
    repositories,
    repositoriesLoading,
    branches,
    branchesLoading,
    computed,
  };
}
