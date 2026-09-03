"use client";

import { useMemo, useRef, useState } from "react";
import type { LocalRepository } from "@/lib/types/http";
import type {
  DialogFormState,
  StepType,
  TaskFormInputsHandle,
} from "@/components/task-create-dialog-types";
import {
  useRemoteReposSeedEffect,
  useRemoteReposState,
  useRepositoriesState,
} from "@/components/task-create-dialog-repositories-state";
import { useBranchesByURL } from "@/hooks/domains/github/use-branches-by-url";
import { usePRInfoByURL } from "@/hooks/domains/github/use-pr-info-by-url";

/**
 * Workspace mode the New Subtask dialog supports today. The shared_group
 * mode is part of the office task-handoffs spec but only office agents
 * surface it via MCP; the Kanban dialog covers the inherit_parent /
 * new_workspace toggle (handoffs phase 5).
 */
export type SubtaskWorkspaceMode = "inherit_parent" | "new_workspace";

/**
 * Default workspace mode for the New Subtask dialog. When the parent
 * task has an active worktree we default to inherit_parent so the
 * subtask runs in the same materialized environment without forcing
 * the user through the repo picker; otherwise (parent has no
 * materialized workspace yet) we default to new_workspace.
 */
export function defaultSubtaskWorkspaceMode(
  parentWorktreeBranch: string | null,
): SubtaskWorkspaceMode {
  return parentWorktreeBranch ? "inherit_parent" : "new_workspace";
}

/**
 * Returns a `DialogFormState`-shaped object for the New Subtask dialog so it
 * can reuse the create-task dialog's `RepoChipsRow`, `useDialogHandlers`, and
 * `useGitHubUrlBranchesEffect` without any forking of those components.
 *
 * The subtask flow only exercises a slice of the full state: repo rows,
 * GitHub URL mode, agent/executor profiles, and fresh-branch selection for a
 * local executor in a new workspace. The remaining fields (title, workflow,
 * draft, and discovered repos) are kept as inert stubs because the subtask
 * dialog renders its own title input and inherits the parent's workflow.
 */
export function useSubtaskFormState(workspaceId: string | null): DialogFormState {
  const repos = useRepositoriesState();
  const remoteRepos = useRemoteReposState();
  const branchesByUrl = useBranchesByURL(workspaceId);
  const prInfoByUrl = usePRInfoByURL(workspaceId);
  const [agentProfileId, setAgentProfileId] = useState("");
  const [executorProfileId, setExecutorProfileId] = useState("");
  const [autopilot, setAutopilot] = useState(false);
  const [freshBranchEnabled, setFreshBranchEnabled] = useState(false);
  const [useRemote, setUseRemote] = useState(false);
  const [githubUrlError, setGitHubUrlError] = useState<string | null>(null);
  // Discovered (on-disk) repos — populated by useDiscoverReposEffect when the
  // dialog opens, same as the create-task flow. This lets users target
  // not-yet-imported on-machine git folders for the subtask.
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [discoverReposLoading, setDiscoverReposLoading] = useState(false);
  const [discoverReposLoaded, setDiscoverReposLoaded] = useState(false);
  const descriptionInputRef = useRef<TaskFormInputsHandle | null>(null);

  // Mirror the create-task dialog: when the user flips Remote mode on and
  // the chip list is empty, seed a single empty paste row so the URL input
  // has somewhere to land. Non-destructive on toggle-off.
  useRemoteReposSeedEffect(useRemote, remoteRepos.remoteRepos, remoteRepos.setRemoteRepos);

  return useMemo<DialogFormState>(
    () => ({
      ...INERT_TITLE_DRAFT,
      hasPendingAttachmentUploads: false,
      setHasPendingAttachmentUploads: NOOP,
      currentDefaults: EMPTY_DEFAULTS,
      descriptionInputRef,
      // Repo chip row — what RepoChipsRow + useDialogHandlers actually drive.
      repositories: repos.repositories,
      repositoriesDirty: repos.repositoriesDirty,
      setRepositories: repos.setRepositories,
      setRepositoriesDirty: repos.setRepositoriesDirty,
      addRepository: repos.addRepository,
      removeRepository: repos.removeRepository,
      updateRepository: repos.updateRepository,
      remoteRepos: remoteRepos.remoteRepos,
      setRemoteRepos: remoteRepos.setRemoteRepos,
      addRemoteRepo: remoteRepos.addRemoteRepo,
      removeRemoteRepo: remoteRepos.removeRemoteRepo,
      updateRemoteRepo: remoteRepos.updateRemoteRepo,
      branchesByUrl,
      prInfoByUrl,
      agentProfileId,
      setAgentProfileId,
      executorId: "",
      setExecutorId: NOOP,
      executorProfileId,
      setExecutorProfileId,
      autopilot,
      setAutopilot,
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
      useRemote,
      setUseRemote,
      githubUrlError,
      setGitHubUrlError,
      workflowAgentProfileId: "",
      setWorkflowAgentProfileId: NOOP,
      clearDraft: NOOP,
      ...INERT_FRESH_BRANCH_AND_NOREPO,
      freshBranchEnabled,
      setFreshBranchEnabled,
    }),
    [
      repos.repositories,
      repos.repositoriesDirty,
      repos.setRepositories,
      repos.setRepositoriesDirty,
      repos.addRepository,
      repos.removeRepository,
      repos.updateRepository,
      remoteRepos.remoteRepos,
      remoteRepos.setRemoteRepos,
      remoteRepos.addRemoteRepo,
      remoteRepos.removeRemoteRepo,
      remoteRepos.updateRemoteRepo,
      branchesByUrl,
      prInfoByUrl,
      agentProfileId,
      executorProfileId,
      autopilot,
      freshBranchEnabled,
      useRemote,
      githubUrlError,
      discoveredRepositories,
      discoverReposLoading,
      discoverReposLoaded,
    ],
  );
}

const NOOP = () => undefined;
const EMPTY_DEFAULTS = { name: "", description: "" };
const EMPTY_STEPS: StepType[] | null = null;

// Title / draft / openCycle are inert in the subtask flow — the dialog renders
// its own title input directly and doesn't restore drafts. Extracted so the
// useMemo body stays under the function-length lint cap.
const INERT_TITLE_DRAFT = {
  blockedBy: [] as string[],
  setBlockedBy: () => undefined,
  mcpServerIds: [] as string[],
  setMcpServerIds: () => undefined,
  mcpServerIdsDirty: false,
  setMcpServerIdsDirty: () => undefined,
  taskName: "",
  setTaskName: NOOP,
  hasTitle: false,
  setHasTitle: NOOP,
  hasDescription: false,
  setHasDescription: NOOP,
  draftDescription: "",
  openCycle: 0,
  autopilot: false,
  setAutopilot: NOOP,
} as const;

// No-repo / scratch workspace mode is a top-level create-task feature. The
// fresh-branch fields are real state above because new-workspace subtasks can
// create a policy branch with a local executor.
const INERT_FRESH_BRANCH_AND_NOREPO = {
  currentLocalBranch: "",
  setCurrentLocalBranch: NOOP,
  currentLocalBranchLoading: false,
  setCurrentLocalBranchLoading: NOOP,
  noRepository: false,
  setNoRepository: NOOP,
  workspacePath: "",
  setWorkspacePath: NOOP,
} as const;
