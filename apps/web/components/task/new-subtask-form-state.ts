"use client";

import { useMemo, useRef, useState } from "react";
import type { Branch, LocalRepository } from "@/lib/types/http";
import type {
  DialogFormState,
  StepType,
  TaskFormInputsHandle,
} from "@/components/task-create-dialog-types";
import { useRepositoriesState } from "@/components/task-create-dialog-repositories-state";

/**
 * Returns a `DialogFormState`-shaped object for the New Subtask dialog so it
 * can reuse the create-task dialog's `RepoChipsRow`, `useDialogHandlers`, and
 * `useGitHubUrlBranchesEffect` without any forking of those components.
 *
 * The subtask flow only exercises a slice of the full state: repo rows,
 * GitHub URL mode, agent/executor profiles. The remaining fields (title,
 * workflow, draft, fresh-branch, discovered repos) are kept as inert stubs
 * because the subtask dialog renders its own title input and inherits the
 * parent's workflow.
 */
export function useSubtaskFormState(): DialogFormState {
  const repos = useRepositoriesState();
  const [agentProfileId, setAgentProfileId] = useState("");
  const [executorProfileId, setExecutorProfileId] = useState("");
  const [useGitHubUrl, setUseGitHubUrl] = useState(false);
  const [githubUrl, setGitHubUrl] = useState("");
  const [githubBranch, setGitHubBranch] = useState("");
  const [githubBranches, setGitHubBranches] = useState<Branch[]>([]);
  const [githubBranchesLoading, setGitHubBranchesLoading] = useState(false);
  const [githubUrlError, setGitHubUrlError] = useState<string | null>(null);
  const [githubPrHeadBranch, setGitHubPrHeadBranch] = useState<string | null>(null);
  // Discovered (on-disk) repos — populated by useDiscoverReposEffect when the
  // dialog opens, same as the create-task flow. This lets users target
  // not-yet-imported on-machine git folders for the subtask.
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [discoverReposLoading, setDiscoverReposLoading] = useState(false);
  const [discoverReposLoaded, setDiscoverReposLoaded] = useState(false);
  const descriptionInputRef = useRef<TaskFormInputsHandle | null>(null);

  return useMemo<DialogFormState>(
    () => ({
      // Title is owned by the subtask dialog directly — these are inert.
      taskName: "",
      setTaskName: NOOP,
      hasTitle: false,
      setHasTitle: NOOP,
      hasDescription: false,
      setHasDescription: NOOP,
      draftDescription: "",
      openCycle: 0,
      currentDefaults: EMPTY_DEFAULTS,
      descriptionInputRef,
      // Repo chip row — what RepoChipsRow + useDialogHandlers actually drive.
      repositories: repos.repositories,
      setRepositories: repos.setRepositories,
      addRepository: repos.addRepository,
      removeRepository: repos.removeRepository,
      updateRepository: repos.updateRepository,
      githubBranch,
      setGitHubBranch,
      agentProfileId,
      setAgentProfileId,
      executorId: "",
      setExecutorId: NOOP,
      executorProfileId,
      setExecutorProfileId,
      discoveredRepositories,
      setDiscoveredRepositories,
      discoverReposLoading,
      setDiscoverReposLoading,
      discoverReposLoaded,
      setDiscoverReposLoaded,
      // Subtasks inherit the parent's workflow; no selector is rendered.
      selectedWorkflowId: null,
      setSelectedWorkflowId: NOOP,
      fetchedSteps: EMPTY_STEPS,
      setFetchedSteps: NOOP,
      isCreatingSession: false,
      setIsCreatingSession: NOOP,
      isCreatingTask: false,
      setIsCreatingTask: NOOP,
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
      workflowAgentProfileId: "",
      setWorkflowAgentProfileId: NOOP,
      clearDraft: NOOP,
      // Fresh-branch flow is local-executor-only, single-row, and not
      // surfaced here — keep the toggles wired but always inert.
      freshBranchEnabled: false,
      setFreshBranchEnabled: NOOP,
      currentLocalBranch: "",
      setCurrentLocalBranch: NOOP,
      currentLocalBranchLoading: false,
      setCurrentLocalBranchLoading: NOOP,
    }),
    [
      repos.repositories,
      repos.setRepositories,
      repos.addRepository,
      repos.removeRepository,
      repos.updateRepository,
      githubBranch,
      agentProfileId,
      executorProfileId,
      useGitHubUrl,
      githubUrl,
      githubBranches,
      githubBranchesLoading,
      githubUrlError,
      githubPrHeadBranch,
      discoveredRepositories,
      discoverReposLoading,
      discoverReposLoaded,
    ],
  );
}

const NOOP = () => undefined;
const EMPTY_DEFAULTS = { name: "", description: "" };
const EMPTY_STEPS: StepType[] | null = null;
