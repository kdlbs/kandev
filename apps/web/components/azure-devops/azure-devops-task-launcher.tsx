"use client";

import { useMemo } from "react";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { associateAzureDevOpsPullRequest } from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsPullRequest, AzureDevOpsWorkItem } from "@/lib/types/azure-devops";
import type { Repository, Task, Workflow, WorkflowStep } from "@/lib/types/http";

export type AzureDevOpsLaunchPayload =
  | { kind: "work-item"; item: AzureDevOpsWorkItem }
  | { kind: "pull-request"; pullRequest: AzureDevOpsPullRequest };

function plainText(value: string | undefined): string {
  if (!value) return "(no description)";
  const document = new DOMParser().parseFromString(value, "text/html");
  return document.body.textContent?.trim() || "(no description)";
}

function launchText(payload: AzureDevOpsLaunchPayload) {
  if (payload.kind === "work-item") {
    const item = payload.item;
    return {
      title: `${item.type} ${item.id}: ${item.title}`,
      description: [
        `Azure DevOps work item: ${item.id}`,
        `URL: ${item.webUrl ?? item.apiUrl ?? ""}`,
        "",
        item.title,
        "",
        plainText(item.description),
      ].join("\n"),
    };
  }
  const pullRequest = payload.pullRequest;
  return {
    title: `Review PR ${pullRequest.id}: ${pullRequest.title}`,
    description: [
      `Azure DevOps pull request: ${pullRequest.webUrl}`,
      "",
      pullRequest.title,
      "",
      plainText(pullRequest.description),
    ].join("\n"),
  };
}

function matchingRepository(
  payload: AzureDevOpsLaunchPayload,
  repositories: Repository[],
): Repository | undefined {
  if (payload.kind !== "pull-request") return undefined;
  return repositories.find(
    (repository) =>
      repository.provider === "azure_devops" &&
      repository.provider_repo_id === payload.pullRequest.repositoryId &&
      repository.provider_owner === payload.pullRequest.projectId,
  );
}

export function AzureDevOpsTaskLauncher({
  workspaceId,
  workflows,
  steps,
  repositories,
  payload,
  onClose,
}: {
  workspaceId?: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  payload: AzureDevOpsLaunchPayload | null;
  onClose: () => void;
}) {
  const router = useRouter();
  const setTaskPullRequest = useAppStore((state) => state.setAzureDevOpsTaskPullRequest);
  const launch = useMemo(() => {
    if (!payload) return null;
    const workflow = workflows.find((candidate) =>
      steps.some((step) => step.workflow_id === candidate.id),
    );
    const workflowSteps = steps
      .filter((step) => step.workflow_id === workflow?.id)
      .sort((left, right) => left.position - right.position);
    return {
      workflow,
      workflowSteps,
      repository: matchingRepository(payload, repositories),
      text: launchText(payload),
    };
  }, [payload, repositories, steps, workflows]);

  const onSuccess = (task: Task) => {
    if (payload?.kind === "pull-request" && workspaceId && launch?.repository) {
      void associateAzureDevOpsPullRequest(workspaceId, task.id, {
        repositoryId: launch.repository.id,
        pullRequestId: payload.pullRequest.id,
      })
        .then((linked) => setTaskPullRequest(task.id, linked))
        .catch(() => undefined);
    }
    onClose();
    router.push(`/tasks/${task.id}`);
  };

  if (!workspaceId || !payload || !launch?.workflow || !launch.workflowSteps[0]) return null;
  return (
    <TaskCreateDialog
      open
      onOpenChange={(open) => !open && onClose()}
      mode="create"
      workspaceId={workspaceId}
      workflowId={launch.workflow.id}
      defaultStepId={launch.workflowSteps[0].id}
      steps={launch.workflowSteps.map((step) => ({
        id: step.id,
        title: step.name,
        events: step.events,
      }))}
      initialValues={{
        title: launch.text.title,
        description: launch.text.description,
        repositoryId: launch.repository?.id,
        branch:
          payload.kind === "pull-request"
            ? payload.pullRequest.sourceBranch.replace(/^refs\/heads\//, "")
            : undefined,
      }}
      onSuccess={onSuccess}
    />
  );
}
