import type { LocalRepository, TaskPriority } from "@/lib/types/http";
import type {
  StepType,
  TaskCreateDialogInitialValues,
  TaskRemoteRepoRow,
  TaskRepoRow,
  TaskRepositorySelection,
} from "@/components/task-create-dialog-types";
import type { TaskRemoteProviderReadinessMap } from "@/components/task-create-dialog-remote-provider-readiness";

export type FormResetters = {
  setTaskName: (value: string) => void;
  setHasTitle: (value: boolean) => void;
  setHasDescription: (value: boolean) => void;
  setHasPendingAttachmentUploads: (value: boolean) => void;
  setRepositories: (value: TaskRepoRow[]) => void;
  setRepositoriesDirty: (value: boolean) => void;
  setRemoteRepos: (value: TaskRemoteRepoRow[]) => void;
  /** Canonical reset path for the ordered mixed selection state. */
  resetRepositorySelections?: (value: TaskRepositorySelection[]) => void;
  setAgentProfileId: (value: string) => void;
  setExecutorId: (value: string) => void;
  setExecutorProfileId: (value: string) => void;
  setWorkflowAgentOverrides?: (value: Record<string, string>) => void;
  setExecutorChoiceTouched?: (value: boolean) => void;
  setAutomaticExecutorRestore?: (
    value: { executorId: string; executorProfileId: string } | null,
  ) => void;
  setFolderOnlyExecutorNotice?: (value: boolean) => void;
  setSelectedWorkflowId: (value: string | null) => void;
  setFetchedSteps: (value: StepType[] | null) => void;
  setDiscoveredRepositories: (value: LocalRepository[]) => void;
  setDiscoverReposLoaded: (value: boolean) => void;
  setUseRemote: (value: boolean) => void;
  setNoRepository: (value: boolean) => void;
  setPreferLocalExecutor: (value: boolean) => void;
  setWorkspacePath: (value: string) => void;
  setAutopilot: (value: boolean) => void;
  setPriority: (value: TaskPriority) => void;
  setRemoteProviderReadiness?: (value: TaskRemoteProviderReadinessMap) => void;
  setGitHubUrlError: (value: string | null) => void;
  setFreshBranchEnabled: (value: boolean) => void;
  setCurrentLocalBranch: (value: string) => void;
  setBlockedBy: (value: string[]) => void;
};

export function resetTaskForm(
  resetters: FormResetters,
  name: string,
  description: string,
  workflowId: string | null,
  initialValues?: TaskCreateDialogInitialValues,
) {
  resetters.setTaskName(name);
  resetters.setHasTitle(name.trim().length > 0);
  resetters.setHasDescription(description.trim().length > 0);
  resetters.setHasPendingAttachmentUploads(false);
  resetRepositoryState(resetters, initialValues);
  resetters.setRepositoriesDirty(false);
  resetters.setAgentProfileId("");
  resetters.setExecutorId("");
  resetters.setExecutorProfileId("");
  resetters.setWorkflowAgentOverrides?.({});
  resetters.setExecutorChoiceTouched?.(false);
  resetters.setAutomaticExecutorRestore?.(null);
  resetters.setFolderOnlyExecutorNotice?.(false);
  resetters.setSelectedWorkflowId(workflowId);
  resetters.setFetchedSteps(null);
  resetters.setNoRepository(resolveInitialNoRepository(initialValues));
  resetters.setPreferLocalExecutor(initialValues?.preferLocalExecutor ?? false);
  resetters.setWorkspacePath("");
  resetters.setAutopilot(false);
  resetters.setPriority("medium");
  resetters.setRemoteProviderReadiness?.({});
}

function resetRepositoryState(
  resetters: FormResetters,
  initialValues: TaskCreateDialogInitialValues | undefined,
) {
  const restoredRepositories = restoreRepositories(initialValues);
  const initialSelections = repositorySelectionsFromInitialValues(
    initialValues,
    restoredRepositories,
  );
  if (resetters.resetRepositorySelections) {
    resetters.resetRepositorySelections(initialSelections);
    return;
  }
  resetters.setRepositories(restoredRepositories);
  resetters.setRemoteRepos(seededRemoteRepositories(initialValues));
}

function restoreRepositories(initialValues?: TaskCreateDialogInitialValues): TaskRepoRow[] {
  const restoredRepositories = (initialValues?.repositories ?? []).map((repository, index) => ({
    key: `row-${index}`,
    repositoryId: repository.repository_id,
    branch:
      repository.branch_policy_base_branch ??
      repository.base_branch ??
      repository.checkout_branch ??
      "",
    branchPolicyId: repository.branch_policy_id,
  }));
  if (restoredRepositories.length === 0 && initialValues?.repositoryId) {
    restoredRepositories.push({
      key: "row-0",
      repositoryId: initialValues.repositoryId,
      branch: initialValues.branch ?? "",
      branchPolicyId: undefined,
    });
  }
  return restoredRepositories;
}

function resolveInitialNoRepository(initialValues?: TaskCreateDialogInitialValues): boolean {
  if (initialValues?.noRepository !== undefined) return initialValues.noRepository;
  return initialValues?.repositorySelections?.length === 0;
}

/** Builds the ordered source rows used when a dialog opens. */
export function repositorySelectionsFromInitialValues(
  initialValues: TaskCreateDialogInitialValues | undefined,
  restoredRepositories?: TaskRepoRow[],
): TaskRepositorySelection[] {
  if (initialValues?.repositorySelections) return initialValues.repositorySelections;
  const localRows =
    restoredRepositories ??
    (initialValues?.repositories ?? []).map((repository, index) => ({
      key: `row-${index}`,
      repositoryId: repository.repository_id,
      branch:
        repository.branch_policy_base_branch ??
        repository.base_branch ??
        repository.checkout_branch ??
        "",
      branchPolicyId: repository.branch_policy_id,
    }));
  const selections = [
    ...localRows.map((row) => ({ kind: "local" as const, ...row })),
    ...remoteSelectionsFromInitialValues(initialValues),
  ];
  return selections;
}

/** Converts a legacy URL preset into the remote row shape used by the picker. */
export function seededRemoteRepositories(iv?: TaskCreateDialogInitialValues): TaskRemoteRepoRow[] {
  const inspection = iv?.remoteRepository;
  const remoteUrl = remotePresetUrl(iv, inspection);
  if (!remoteUrl) return [];
  return [buildSeededRemoteRepository(iv, inspection, remoteUrl)];
}

function remotePresetUrl(
  initialValues: TaskCreateDialogInitialValues | undefined,
  inspection: TaskCreateDialogInitialValues["remoteRepository"],
): string {
  return initialValues?.remoteUrl ?? initialValues?.githubUrl ?? inspection?.cloneUrl ?? "";
}

function buildSeededRemoteRepository(
  initialValues: TaskCreateDialogInitialValues | undefined,
  inspection: TaskCreateDialogInitialValues["remoteRepository"],
  remoteUrl: string,
): TaskRemoteRepoRow {
  return {
    key: "remote-0",
    url: remoteUrl,
    branch: remotePresetBranch(initialValues, inspection),
    source: "paste",
    ...seededRemotePullRequestFields(initialValues, inspection),
    ...seededRemoteProviderFields(inspection),
  };
}

function seededRemotePullRequestFields(
  initialValues: TaskCreateDialogInitialValues | undefined,
  inspection: TaskCreateDialogInitialValues["remoteRepository"],
) {
  return {
    prNumber: initialValues?.prNumber ?? inspection?.pullRequest?.number,
    prBaseBranch: initialValues?.prBaseBranch ?? inspection?.baseBranch,
    prHeadBranch: initialValues?.checkoutBranch ?? inspection?.headBranch,
  };
}

function seededRemoteProviderFields(inspection: TaskCreateDialogInitialValues["remoteRepository"]) {
  return {
    remoteUrl: inspection?.cloneUrl,
    provider: inspection?.providerId,
    providerHost: inspection?.providerHost,
    providerScope: inspection?.providerScope,
    providerRepoId: inspection?.repositoryId,
    providerOwner: inspection?.ownerOrProject,
    providerName: inspection?.repositoryName,
    fullName: inspection ? `${inspection.ownerOrProject}/${inspection.repositoryName}` : undefined,
  };
}

function remotePresetBranch(
  initialValues: TaskCreateDialogInitialValues | undefined,
  inspection: TaskCreateDialogInitialValues["remoteRepository"],
): string {
  return (
    initialValues?.checkoutBranch ??
    initialValues?.branch ??
    inspection?.headBranch ??
    inspection?.defaultBranch ??
    ""
  );
}

export function remoteSelectionsFromInitialValues(
  initialValues?: TaskCreateDialogInitialValues,
): TaskRepositorySelection[] {
  return seededRemoteRepositories(initialValues).map((row) => ({
    kind: "remote" as const,
    ...row,
  }));
}
