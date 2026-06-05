"use client";

import { useEffect, useRef, useState, useMemo, useCallback } from "react";
import type { LocalRepository } from "@/lib/types/http";
import type {
  TaskFormInputsHandle,
  TaskRemoteRepoRow,
} from "@/components/task-create-dialog-types";
import { useBranchesByURL } from "@/hooks/domains/github/use-branches-by-url";
import { usePRInfoByURL } from "@/hooks/domains/github/use-pr-info-by-url";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { getTaskCreateDraft, setTaskCreateDraft, removeTaskCreateDraft } from "@/lib/local-storage";
import type {
  StepType,
  TaskCreateDialogInitialValues,
  DialogFormState,
  TaskRepoRow,
} from "@/components/task-create-dialog-types";
import {
  useRemoteReposSeedEffect,
  useRemoteReposState,
  useRepositoriesState,
} from "@/components/task-create-dialog-repositories-state";
import { useDialogComputed } from "@/components/task-create-dialog-computed";

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
  setRemoteRepos: (v: TaskRemoteRepoRow[]) => void;
  setAgentProfileId: (v: string) => void;
  setExecutorId: (v: string) => void;
  setExecutorProfileId: (v: string) => void;
  setSelectedWorkflowId: (v: string | null) => void;
  setFetchedSteps: (v: StepType[] | null) => void;
  setDiscoveredRepositories: (v: LocalRepository[]) => void;
  setDiscoverReposLoaded: (v: boolean) => void;
  setUseRemote: (v: boolean) => void;
  setNoRepository: (v: boolean) => void;
  setWorkspacePath: (v: string) => void;
  setGitHubUrlError: (v: string | null) => void;
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
  resetters.setUseRemote(Boolean(ghUrl));
  // Seed remoteRepos with a single paste row when the dialog opens with a
  // pre-filled URL (Quick-task launcher path). When `checkoutBranch` is set
  // (PR launch flow), seed the row's branch with it so the chip pill shows
  // the PR head immediately. Otherwise start empty — the seed effect creates
  // an empty row on mode toggle.
  if (ghUrl) {
    const seededBranch = iv?.checkoutBranch ?? iv?.branch ?? "";
    resetters.setRemoteRepos([
      { key: "remote-0", url: ghUrl, branch: seededBranch, source: "paste" },
    ]);
  } else {
    resetters.setRemoteRepos([]);
  }
  resetters.setGitHubUrlError(null);
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
  const [useRemote, setUseRemote] = useState(false);
  const [githubUrlError, setGitHubUrlError] = useState<string | null>(null);
  return {
    useRemote,
    setUseRemote,
    githubUrlError,
    setGitHubUrlError,
  };
}

/** Core form state declarations */
function useFormStateValues(workflowId: string | null) {
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
  const form = useFormStateValues(workflowId);
  const discovery = useDiscoveryState();
  const ghUrl = useGitHubUrlState();
  const wfAgent = useWorkflowAgentProfileState();
  const repos = useRepositoriesState();
  const remoteRepos = useRemoteReposState();
  const freshBranch = useFreshBranchState();
  const branchesByUrl = useBranchesByURL();
  const prInfoByUrl = usePRInfoByURL();

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
      setRemoteRepos: remoteRepos.setRemoteRepos,
      setAgentProfileId: form.setAgentProfileId,
      setExecutorId: form.setExecutorId,
      setExecutorProfileId: form.setExecutorProfileId,
      setSelectedWorkflowId: form.setSelectedWorkflowId,
      setFetchedSteps: form.setFetchedSteps,
      setDiscoveredRepositories: discovery.setDiscoveredRepositories,
      setDiscoverReposLoaded: discovery.setDiscoverReposLoaded,
      setUseRemote: ghUrl.setUseRemote,
      setGitHubUrlError: ghUrl.setGitHubUrlError,
      setFreshBranchEnabled: freshBranch.setFreshBranchEnabled,
      setCurrentLocalBranch: freshBranch.setCurrentLocalBranch,
      setNoRepository: form.setNoRepository,
      setWorkspacePath: form.setWorkspacePath,
    },
  });

  useRemoteReposSeedEffect(ghUrl.useRemote, remoteRepos.remoteRepos, remoteRepos.setRemoteRepos);

  // Title autofill: the first remote-repo row's PR info seeds the task title
  // when the user hasn't typed one. Keyed only off row 0 — adding more PR
  // rows later never overwrites the title.
  useTitleAutofillFromFirstRowPRInfo({
    open: open && ghUrl.useRemote,
    firstRowUrl: remoteRepos.remoteRepos[0]?.url ?? "",
    prInfoByUrl,
    taskName: form.taskName,
    setTaskName: form.setTaskName,
    setHasTitle: form.setHasTitle,
  });

  const { clearDraft } = useDraftPersistence(
    open,
    workspaceId,
    initialValues,
    form.taskName,
    form.descriptionInputRef,
  );

  return {
    ...form,
    ...discovery,
    ...ghUrl,
    ...wfAgent,
    ...repos,
    ...remoteRepos,
    ...freshBranch,
    branchesByUrl,
    prInfoByUrl,
    clearDraft,
  };
}

/**
 * Seeds the task title from the first Remote row's PR info (when present)
 * the first time it arrives, and only when the user hasn't typed a title.
 * Subsequent PR-info changes for the same URL don't overwrite — the
 * lastAutoFilledRef tracks our own writes so re-pasting a different PR URL
 * still works while a user-edited title is preserved.
 */
/** Sentinel stored in `lastAutoFilledRef` once the user clears an
 * auto-filled title. Distinguishable from any real suggested title (which
 * always begins with `"PR #"`) so the effect can refuse to re-apply autofill
 * for the current URL until the URL itself changes. */
const USER_CLEARED_SENTINEL = "\0cleared";

function useTitleAutofillFromFirstRowPRInfo(args: {
  open: boolean;
  firstRowUrl: string;
  prInfoByUrl: ReturnType<typeof usePRInfoByURL>;
  taskName: string;
  setTaskName: (v: string) => void;
  setHasTitle: (v: boolean) => void;
}) {
  const { open, firstRowUrl, prInfoByUrl, taskName, setTaskName, setHasTitle } = args;
  const lastAutoFilledRef = useRef("");
  const lastUrlRef = useRef("");
  // Re-read latest info on every render; cheap because the cache is memoized
  // by URL inside the hook.
  const suggested = firstRowUrl ? prInfoByUrl.info(firstRowUrl)?.suggestedTitle : undefined;

  // Reset ownership-tracking when the URL changes — switching to a different
  // PR URL grants a fresh autofill opportunity even if the user previously
  // cleared an autofill on the prior URL.
  useEffect(() => {
    if (firstRowUrl !== lastUrlRef.current) {
      lastUrlRef.current = firstRowUrl;
      lastAutoFilledRef.current = "";
    }
  }, [firstRowUrl]);

  useEffect(() => {
    // Detect the "user cleared an auto-filled title" transition. Once we see
    // it, lock further autofill for the current URL (until the URL changes,
    // which is handled by the effect above). Without this, the effect below
    // would see `trimmed === ""` again and dutifully re-apply the suggested
    // title that the user just removed.
    if (
      !taskName.trim() &&
      lastAutoFilledRef.current &&
      lastAutoFilledRef.current !== USER_CLEARED_SENTINEL
    ) {
      lastAutoFilledRef.current = USER_CLEARED_SENTINEL;
    }
  }, [taskName]);

  useEffect(() => {
    if (!open) return;
    if (!suggested) return;
    if (lastAutoFilledRef.current === USER_CLEARED_SENTINEL) return;
    const trimmed = taskName.trim();
    // Two writeable states: title is empty AND we haven't auto-filled yet, or
    // title equals our last auto-fill (so a fresh PR URL replaces a previous
    // PR's auto-filled title).
    if (trimmed && taskName !== lastAutoFilledRef.current) return;
    if (taskName === suggested) return;
    lastAutoFilledRef.current = suggested;
    setTaskName(suggested);
    setHasTitle(true);
  }, [open, suggested, taskName, setTaskName, setHasTitle]);
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

export { useDialogComputed } from "@/components/task-create-dialog-computed";

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
