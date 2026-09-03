import type { LocalRepository } from "@/lib/types/http";
import type {
  StepType,
  TaskCreateDialogInitialValues,
  TaskRemoteRepoRow,
  TaskRepoRow,
} from "@/components/task-create-dialog-types";

export type FormResetters = {
  setTaskName: (value: string) => void;
  setHasTitle: (value: boolean) => void;
  setHasDescription: (value: boolean) => void;
  setHasPendingAttachmentUploads: (value: boolean) => void;
  setRepositories: (value: TaskRepoRow[]) => void;
  setRepositoriesDirty: (value: boolean) => void;
  setRemoteRepos: (value: TaskRemoteRepoRow[]) => void;
  setAgentProfileId: (value: string) => void;
  setExecutorId: (value: string) => void;
  setExecutorProfileId: (value: string) => void;
  setSelectedWorkflowId: (value: string | null) => void;
  setFetchedSteps: (value: StepType[] | null) => void;
  setDiscoveredRepositories: (value: LocalRepository[]) => void;
  setDiscoverReposLoaded: (value: boolean) => void;
  setUseRemote: (value: boolean) => void;
  setNoRepository: (value: boolean) => void;
  setWorkspacePath: (value: string) => void;
  setAutopilot: (value: boolean) => void;
  setGitHubUrlError: (value: string | null) => void;
  setFreshBranchEnabled: (value: boolean) => void;
  setCurrentLocalBranch: (value: string) => void;
  setBlockedBy: (value: string[]) => void;
  setMcpServerIds: (value: string[]) => void;
  setMcpServerIdsDirty: (value: boolean) => void;
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
  const savedRepositories = initialValues?.repositories ?? [];
  const restoredRepositories = savedRepositories.map((repository, index) => ({
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
  resetters.setRepositories(restoredRepositories);
  resetters.setRepositoriesDirty(false);
  resetters.setAgentProfileId("");
  resetters.setExecutorId("");
  resetters.setExecutorProfileId("");
  resetters.setSelectedWorkflowId(workflowId);
  resetters.setFetchedSteps(null);
  resetters.setAutopilot(false);
  resetters.setMcpServerIds([]);
  resetters.setMcpServerIdsDirty(false);
}
