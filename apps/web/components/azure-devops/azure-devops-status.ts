import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";

export type AzureDevOpsPullRequestPresentation = {
  provider: "azure_devops";
  labelKey: string;
  tone: "success" | "danger" | "warning" | "muted" | "info";
};

export function getAzureDevOpsPullRequestPresentation(
  pullRequest: AzureDevOpsTaskPullRequest,
): AzureDevOpsPullRequestPresentation {
  const status = pullRequest.status.toLowerCase();
  if (status === "completed") {
    return { provider: "azure_devops", labelKey: "azuredevops:prStatusCompleted", tone: "success" };
  }
  if (status === "abandoned") {
    return { provider: "azure_devops", labelKey: "azuredevops:prStatusAbandoned", tone: "muted" };
  }
  if (pullRequest.policyState === "failure") {
    return {
      provider: "azure_devops",
      labelKey: "azuredevops:prStatusPolicyFailed",
      tone: "danger",
    };
  }
  if (pullRequest.reviewState === "rejected") {
    return {
      provider: "azure_devops",
      labelKey: "azuredevops:prStatusChangesRequested",
      tone: "danger",
    };
  }
  if (pullRequest.isDraft) {
    return { provider: "azure_devops", labelKey: "azuredevops:prStatusDraft", tone: "muted" };
  }
  if (pullRequest.policyState === "pending") {
    return {
      provider: "azure_devops",
      labelKey: "azuredevops:prStatusPolicyRunning",
      tone: "warning",
    };
  }
  if (pullRequest.reviewState === "waiting") {
    return {
      provider: "azure_devops",
      labelKey: "azuredevops:prStatusWaitingForReview",
      tone: "warning",
    };
  }
  if (pullRequest.reviewState === "approved" && pullRequest.policyState === "success") {
    return { provider: "azure_devops", labelKey: "azuredevops:prStatusReady", tone: "success" };
  }
  return { provider: "azure_devops", labelKey: "azuredevops:prStatusActive", tone: "info" };
}
