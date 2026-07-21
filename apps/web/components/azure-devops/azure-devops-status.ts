import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";

export type AzureDevOpsPullRequestPresentation = {
  provider: "azure_devops";
  label: string;
  tone: "success" | "danger" | "warning" | "muted" | "info";
};

export function getAzureDevOpsPullRequestPresentation(
  pullRequest: AzureDevOpsTaskPullRequest,
): AzureDevOpsPullRequestPresentation {
  const status = pullRequest.status.toLowerCase();
  if (status === "completed") {
    return { provider: "azure_devops", label: "Completed", tone: "success" };
  }
  if (status === "abandoned") {
    return { provider: "azure_devops", label: "Abandoned", tone: "muted" };
  }
  if (pullRequest.policyState === "failure") {
    return { provider: "azure_devops", label: "Policy failed", tone: "danger" };
  }
  if (pullRequest.reviewState === "rejected") {
    return { provider: "azure_devops", label: "Changes requested", tone: "danger" };
  }
  if (pullRequest.isDraft) {
    return { provider: "azure_devops", label: "Draft", tone: "muted" };
  }
  if (pullRequest.policyState === "pending") {
    return { provider: "azure_devops", label: "Policy running", tone: "warning" };
  }
  if (pullRequest.reviewState === "waiting") {
    return { provider: "azure_devops", label: "Waiting for review", tone: "warning" };
  }
  if (pullRequest.reviewState === "approved" && pullRequest.policyState === "success") {
    return { provider: "azure_devops", label: "Ready", tone: "success" };
  }
  return { provider: "azure_devops", label: "Active", tone: "info" };
}
