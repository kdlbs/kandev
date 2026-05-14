"use client";

import { useEffect, useRef, useState, useMemo, useCallback } from "react";
import type { LocalRepository, Workspace, ExecutorProfile, Branch } from "@/lib/types/http";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
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

/**
 * Multi-repo tasks currently only run on the git-worktree executor —
 * Docker/Sprites/etc. don't yet know how to provision N sibling repos under
 * one task root. Returning a non-null reason marks the option as disabled in
 * the executor selector and surfaces this string as a tooltip. The dialog
 * only applies this when 2+ repos are selected (see isMultiRepoSelection).
 */
function nonWorktreeDisabledReason(profile: ExecutorProfile): string | null {
  if ((profile.executor_type ?? "") === "worktree") return null;
  return "Multi-repo tasks only support the git-worktree executor.";
}

/**
 * Worktree executor needs a repository to create the worktree from. Disable
 * it when the task is in no-repository mode so the picker doesn't offer an
 * unworkable choice (the backend would silently fall back to local).
 */
function worktreeDisabledReason(profile: ExecutorProfile): string | null {
  if ((profile.executor_type ?? "") !== "worktree") return null;
  return "Worktree executor requires a repository.";
}

/**
 * Combines the two executor-disable rules into a single resolver:
 *   - no-repository mode → disable worktree (it needs a repo)
 *   - multi-repo selection → disable everything except worktree
 *   - otherwise → no disabling
 * The two never co-occur (no-repository implies zero repos, so multi-repo
 * cannot be true at the same time), so a simple priority order is enough.
 */
function pickExecutorDisabledReason(
  noRepository: boolean,
  isMultiRepoSelection: boolean,
): ((profile: ExecutorProfile) => string | null) | undefined {
  if (noRepository) return worktreeDisabledReason;
  if (isMultiRepoSelection) return nonWorktreeDisabledReason;
  return undefined;
}
import type {
  StepType,
  TaskCreateDialogInitialValues,
  DialogFormState,
  DialogComputedValues,
  DialogComputedArgs,
  TaskRepoRow,
} from "@/components/task-create-dialog-types";
import { useRepositoriesState } from "@/components/task-create-dialog-repositories-state";
import { computePassthroughProfile } from "@/components/task-create-dialog-helpers";
import {
  computeDialogDefaultStepId,
  computeSingleWorkflowFallbackId,
} from "@/components/task-create-dialog-defaults";
import { useRemoteAuthSpecs } from "@/hooks/domains/settings/use-remote-auth-specs";
import { isAgentConfiguredOnExecutor } from "@/lib/agent-executor-compat";

export type {
  StepType,
  TaskCreateDialogInitialValues,
} from "@/components/task-create-dialog-types";
export { autoSelectBranch } from "@/components/task-create-dialog-helpers";
export { useLockedFieldSync } from "@/components/task-create-dialog-locked-fields";

type FormResetters = {
  setTaskName: (v: string) => void;
  setHasTitle: (v: boolean) => void;
  setHasDescription: (v: boolean) => void;
  setRepositories: (v: TaskRepoRow[]) => void;
  setGitHubBranch: (v: string) => void;
  setAgentProfileId: (v: string) => void;
  setExecutorId: (v: string) => void;
  setExecutorProfileId: (v: string) => void;
  setSelectedWorkflowId: (v: string | null) => void;
  setFetchedSteps: (v: StepType[] | null) => void;
  setDiscoveredRepositories: (v: LocalRepository[]) => void;
  setDiscoverReposLoaded: (v: boolean) => void;
  setUseGitHubUrl: (v: boolean) => void;
  setGitHubUrl: (v: string) => void;
  setNoRepository: (v: boolean) => void;
  setWorkspacePath: (v: string) => void;
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
  // Seed the unified repos list from initialValues. A repo + branch pre-fill
  // becomes a single row; nothing seeds an empty list (the auto-select
  // effect later picks the user's last-used repo or the first workspace one).
  if (initialValues?.repositoryId) {
    resetters.setRepositories([
      {
        key: "row-0",
        repositoryId: initialValues.repositoryId,
        branch: initialValues.branch ?? "",
      },
    ]);
  } else {
    resetters.setRepositories([]);
  }
  resetters.setGitHubBranch(initialValues?.branch ?? "");
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
  resetters.setDiscoverReposLoaded(false);
  resetters.setUseGitHubUrl(Boolean(ghUrl));
  resetters.setGitHubUrl(ghUrl);
  resetters.setGitHubBranches([]);
  resetters.setGitHubUrlError(null);
  resetters.setGitHubPrHeadBranch(iv?.checkoutBranch ?? null);
  resetters.setFreshBranchEnabled(false);
  resetters.setCurrentLocalBranch("");
  // Source-mode toggle resets — without these, opening the dialog in "None"
  // mode and reopening for a different task would land in None mode again.
  resetters.setNoRepository(false);
  resetters.setWorkspacePath("");
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
  const [currentLocalBranchLoading, setCurrentLocalBranchLoading] = useState(false);
  return {
    freshBranchEnabled,
    setFreshBranchEnabled,
    currentLocalBranch,
    setCurrentLocalBranch,
    currentLocalBranchLoading,
    setCurrentLocalBranchLoading,
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
  // GitHub URL flow has its own branch field (the per-repo branch lives on
  // each row in `repositories`). Seed from initialValues for the URL flow only.
  const [githubBranch, setGitHubBranch] = useState(initialValues?.branch ?? "");
  const [agentProfileId, setAgentProfileId] = useState("");
  const [executorId, setExecutorId] = useState("");
  const [executorProfileId, setExecutorProfileId] = useState("");
  const [selectedWorkflowId, setSelectedWorkflowId] = useState(workflowId);
  const [fetchedSteps, setFetchedSteps] = useState<StepType[] | null>(null);
  const [isCreatingSession, setIsCreatingSession] = useState(false);
  const [isCreatingTask, setIsCreatingTask] = useState(false);
  // No-repo mode: when true, the task is created with no repositories. The
  // optional workspacePath points the agent at an existing host folder; empty
  // means kandev creates a scratch workspace.
  const [noRepository, setNoRepository] = useState(false);
  const [workspacePath, setWorkspacePath] = useState("");
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
    githubBranch,
    setGitHubBranch,
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
    noRepository,
    setNoRepository,
    workspacePath,
    setWorkspacePath,
  };
}

/** Repository discovery state — just the discovered list. The previous
 *  per-form `selectedLocalRepo` / `discoveredRepoPath` / `localBranches`
 *  primary-only fields are gone; discovered repos now live as ordinary rows
 *  in `fs.repositories` with `localPath` set. */
function useDiscoveryState() {
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [discoverReposLoading, setDiscoverReposLoading] = useState(false);
  const [discoverReposLoaded, setDiscoverReposLoaded] = useState(false);
  return {
    discoveredRepositories,
    setDiscoveredRepositories,
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
  const repos = useRepositoriesState();
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
      setRepositories: repos.setRepositories,
      setGitHubBranch: form.setGitHubBranch,
      setAgentProfileId: form.setAgentProfileId,
      setExecutorId: form.setExecutorId,
      setExecutorProfileId: form.setExecutorProfileId,
      setSelectedWorkflowId: form.setSelectedWorkflowId,
      setFetchedSteps: form.setFetchedSteps,
      setDiscoveredRepositories: discovery.setDiscoveredRepositories,
      setDiscoverReposLoaded: discovery.setDiscoverReposLoaded,
      setUseGitHubUrl: ghUrl.setUseGitHubUrl,
      setGitHubUrl: ghUrl.setGitHubUrl,
      setGitHubBranches: ghUrl.setGitHubBranches,
      setGitHubUrlError: ghUrl.setGitHubUrlError,
      setGitHubPrHeadBranch: ghUrl.setGitHubPrHeadBranch,
      setFreshBranchEnabled: freshBranch.setFreshBranchEnabled,
      setCurrentLocalBranch: freshBranch.setCurrentLocalBranch,
      setNoRepository: form.setNoRepository,
      setWorkspacePath: form.setWorkspacePath,
    },
  });

  const { clearDraft } = useDraftPersistence(
    open,
    workspaceId,
    initialValues,
    form.taskName,
    form.descriptionInputRef,
  );

  return { ...form, ...discovery, ...ghUrl, ...wfAgent, ...repos, ...freshBranch, clearDraft };
}

export type { DialogFormState } from "@/components/task-create-dialog-types";
export {
  computePassthroughProfile,
  computeEffectiveStepId,
  computeIsTaskStarted,
} from "@/components/task-create-dialog-helpers";
export { useTaskCreateDialogEffects } from "@/components/task-create-dialog-effects";

// useDialogHandlers lives in ./task-create-dialog-handlers.ts
export { useDialogHandlers } from "@/components/task-create-dialog-handlers";

export function useDialogComputed({
  fs,
  open,
  workspaceId,
  workflowId,
  defaultStepId,
  settingsData,
  agentProfiles,
  workspaces,
  executors,
  repositories,
  workflows,
  snapshots,
}: DialogComputedArgs): DialogComputedValues {
  const singleWorkflowId = computeSingleWorkflowFallbackId(
    fs.selectedWorkflowId,
    workflowId,
    workflows,
  );
  const effectiveWorkflowId = fs.selectedWorkflowId ?? workflowId ?? singleWorkflowId;
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
  const effectiveDefaultStepId = computeDialogDefaultStepId({
    selectedWorkflowId: fs.selectedWorkflowId,
    workflowId,
    fetchedSteps: fs.fetchedSteps,
    defaultStepId,
    effectiveWorkflowId,
    snapshots,
  });
  const workspaceDefaults = workspaceId
    ? workspaces.find((ws: Workspace) => ws.id === workspaceId)
    : null;
  // The form has a repo selection when either: (a) any chip in the unified
  // list has a repo set, (b) URL mode has a non-empty URL, or (c) the task
  // is intentionally repo-less (noRepository toggle on).
  const hasRepositorySelection = Boolean(
    fs.noRepository ||
    fs.repositories.some((r) => r.repositoryId || r.localPath) ||
    (fs.useGitHubUrl && fs.githubUrl.trim()),
  );
  // Branch options are only used by the URL-mode flow now (the chip's branch
  // pill loads branches per-repo). Keep the computed value but always feed it
  // the URL branches when in URL mode.
  const branchOptions = useBranchOptions(fs.useGitHubUrl ? fs.githubBranches : []);
  const allExecutorProfiles = useMemo<ExecutorProfile[]>(() => {
    return executors.flatMap((executor) =>
      (executor.profiles ?? []).map((p) => ({
        ...p,
        executor_type: p.executor_type ?? executor.type,
        executor_name: p.executor_name ?? executor.name,
      })),
    );
  }, [executors]);
  // Multi-repo tasks only run on the git-worktree executor today — Docker /
  // Sprites / standalone don't yet know how to provision N sibling repos. Gate
  // non-worktree options only when 2+ repos are selected; single-repo tasks
  // keep the full executor catalogue.
  const selectedRepoCount = fs.repositories.filter((r) => r.repositoryId || r.localPath).length;
  const isMultiRepoSelection = selectedRepoCount > 1;
  // `pickExecutorDisabledReason` came from main — folds two disable rules
  // (no-repository disables worktree; multi-repo disables non-worktree) into
  // one resolver. Plug it into our existing useExecutorProfileCompat wrapper
  // so the downstream consumer still gets selectedExecutorProfile /
  // noCompatibleAgent metadata.
  // Use the effective agent ID (form value OR the workflow-locked override)
  // so the compatibility gate catches the override case too — passing the
  // raw fs.agentProfileId would let workflow-locked sessions slip past with
  // an empty selection.
  const exec = useExecutorProfileCompat(
    allExecutorProfiles,
    fs.executorProfileId,
    effectiveAgentProfileId,
    agentProfiles,
    pickExecutorDisabledReason(fs.noRepository, isMultiRepoSelection),
  );
  const agentProfileOptions = useAgentProfileOptions(exec.compatibleAgentProfiles);
  const executorHint = useExecutorHint(executors, fs.executorId, selectedRepoCount);
  const isLocalExecutor = useIsLocalExecutor(executors, fs.executorId);
  const { headerRepositoryOptions } = useRepositoryOptions(repositories, fs.discoveredRepositories);
  // Treat the dialog as still loading agents until BOTH the agent profiles
  // (DB rows) AND the host-utility capability probe have resolved. The
  // backend reconciler renames profiles ("Claude" → "Claude Sonnet 4.6") only
  // after the probe lands, so showing the selector before then surfaces stale
  // labels missing the model badge.
  const agentProfilesLoading =
    open && (!settingsData.agentsLoaded || !settingsData.capabilitiesLoaded);
  const executorsLoading = open && !settingsData.executorsLoaded;
  return {
    isPassthroughProfile,
    effectiveWorkflowId,
    effectiveDefaultStepId,
    workspaceDefaults,
    hasRepositorySelection,
    branchOptions,
    agentProfileOptions,
    executorProfileOptions: exec.executorProfileOptions,
    executorHint,
    isLocalExecutor,
    headerRepositoryOptions,
    agentProfilesLoading,
    executorsLoading,
    workflowAgentLocked,
    workflowAgentProfileId,
    effectiveAgentProfileId,
    selectedExecutorProfileName: exec.selectedExecutorProfile?.name ?? null,
    noCompatibleAgent: exec.noCompatibleAgent,
  };
}

function useExecutorProfileCompat(
  allExecutorProfiles: ExecutorProfile[],
  selectedProfileId: string,
  selectedAgentProfileId: string,
  agentProfiles: DialogComputedArgs["agentProfiles"],
  disabledReasonFor?: (profile: ExecutorProfile) => string | null,
) {
  const executorProfileOptions = useExecutorProfileOptions(allExecutorProfiles, {
    disabledReasonFor,
  });
  const selectedExecutorProfile = useMemo(
    () => allExecutorProfiles.find((p) => p.id === selectedProfileId) ?? null,
    [allExecutorProfiles, selectedProfileId],
  );
  const { specs: authSpecs, loaded: authLoaded } = useRemoteAuthSpecs();
  const compatibleAgentProfiles = useMemo(() => {
    if (!selectedExecutorProfile || !authLoaded) return agentProfiles;
    return agentProfiles.filter((ap) =>
      isAgentConfiguredOnExecutor(ap, selectedExecutorProfile, authSpecs),
    );
  }, [agentProfiles, selectedExecutorProfile, authSpecs, authLoaded]);
  // `noCompatibleAgent` gates the submit button. It must catch BOTH cases:
  //   1. The selected executor has no compatible agents at all.
  //   2. The user picked an agent that isn't compatible with the executor
  //      (e.g. switched executor after the agent was chosen).
  // Previously this only checked case 1, so case 2 silently let the user
  // submit with a known-incompatible combination.
  const noCompatibleAgent = useMemo(() => {
    if (!selectedExecutorProfile) return false;
    if (compatibleAgentProfiles.length === 0) return true;
    if (!selectedAgentProfileId) return false;
    return !compatibleAgentProfiles.some((ap) => ap.id === selectedAgentProfileId);
  }, [selectedExecutorProfile, compatibleAgentProfiles, selectedAgentProfileId]);
  return {
    selectedExecutorProfile,
    compatibleAgentProfiles,
    executorProfileOptions,
    noCompatibleAgent,
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
  const availableAgentsLoaded = useAppStore((state) => state.availableAgents.loaded);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);

  useSettingsData(open);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, open);
  // Per-repo branch loading lives in each chip now (RepoChipsRow). No
  // global branch query is needed here — the chip uses useRepositoryBranches
  // for its own row, and the store dedupes by repositoryId.
  const branchesLoading = false;
  const computed = useDialogComputed({
    fs,
    open,
    workspaceId,
    workflowId,
    defaultStepId,
    settingsData: {
      agentsLoaded: settingsData.agentsLoaded,
      executorsLoaded: settingsData.executorsLoaded,
      capabilitiesLoaded: availableAgentsLoaded,
    },
    agentProfiles,
    workspaces,
    executors,
    repositories,
    workflows,
    snapshots,
  });
  return {
    workflows,
    workspaces,
    agentProfiles,
    executors,
    snapshots,
    repositories,
    repositoriesLoading,
    branchesLoading,
    computed,
  };
}
