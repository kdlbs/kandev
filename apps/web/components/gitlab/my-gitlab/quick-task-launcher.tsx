"use client";

import { useMemo } from "react";
import type { Icon } from "@tabler/icons-react";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { createTaskMR } from "@/lib/api/domains/gitlab-api";
import { linkToTask } from "@/lib/links";
import { useRouter } from "@/lib/routing/client-router";
import type { Issue, MR } from "@/lib/types/gitlab";
import type { Repository, Task, Workflow, WorkflowStep } from "@/lib/types/http";

export type GitLabTaskPreset = {
  id: string;
  label: string;
  hint: string;
  icon: Icon;
  prompt: (input: { url: string; title: string }) => string;
};

export type GitLabLaunchPayload =
  | { kind: "mr"; mr: MR; preset: GitLabTaskPreset }
  | { kind: "issue"; issue: Issue; preset: GitLabTaskPreset };

type QuickTaskLauncherProps = {
  workspaceId: string | null;
  configuredHost: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  payload: GitLabLaunchPayload | null;
  onClose: () => void;
};

function normalizedOrigin(rawHost: string | undefined): string {
  if (!rawHost) return "";
  try {
    const withScheme = rawHost.includes("://") ? rawHost : `https://${rawHost}`;
    const parsed = new URL(withScheme);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return "";
    return `${parsed.protocol}//${parsed.host}`.toLowerCase();
  } catch {
    return "";
  }
}

function projectRepository(
  projectPath: string,
  configuredHost: string,
  repositories: Repository[],
): Repository | undefined {
  const normalizedProject = projectPath.toLowerCase();
  const configuredOrigin = normalizedOrigin(configuredHost);
  return repositories.find((repository) => {
    if (repository.provider.toLowerCase() !== "gitlab") return false;
    if (!configuredOrigin || normalizedOrigin(repository.provider_host) !== configuredOrigin)
      return false;
    const providerPath = `${repository.provider_owner}/${repository.provider_name}`.toLowerCase();
    return providerPath === normalizedProject;
  });
}

function linkedRepositoryID(task: Task, preferredRepositoryID?: string): string | undefined {
  const taskRepositories = task.repositories ?? [];
  if (preferredRepositoryID) {
    const preferred = taskRepositories.find(
      (repository) => repository.repository_id === preferredRepositoryID,
    );
    if (preferred) return preferred.repository_id;
  }
  return taskRepositories[0]?.repository_id;
}

function buildInitialValues(
  payload: GitLabLaunchPayload,
  configuredHost: string,
  repositories: Repository[],
) {
  const item = payload.kind === "mr" ? payload.mr : payload.issue;
  const repository = projectRepository(item.project_path, configuredHost, repositories);
  const branch = payload.kind === "mr" && repository ? payload.mr.head_branch : undefined;
  return {
    title: `${payload.preset.label}: ${item.title}`,
    description: payload.preset.prompt({ url: item.web_url, title: item.title }),
    repositoryId: repository?.id,
    branch,
    checkoutBranch: branch,
  };
}

export function QuickTaskLauncher({
  workspaceId,
  configuredHost,
  workflows,
  steps,
  repositories,
  payload,
  onClose,
}: QuickTaskLauncherProps) {
  const router = useRouter();
  const { toast } = useToast();
  const setTaskMR = useAppStore((state) => state.setTaskMR);
  const workflow = workflows[0];
  const workflowSteps = useMemo(
    () =>
      steps
        .filter((step) => step.workflow_id === workflow?.id)
        .sort((left, right) => left.position - right.position),
    [steps, workflow?.id],
  );
  const initialValues = useMemo(
    () => (payload ? buildInitialValues(payload, configuredHost, repositories) : null),
    [payload, configuredHost, repositories],
  );
  const firstStep = workflowSteps[0];
  if (!workspaceId || !workflow || !firstStep || !payload || !initialValues) return null;

  const handleSuccess = async (task: Task) => {
    if (payload.kind === "mr") {
      const repositoryID = linkedRepositoryID(task, initialValues.repositoryId);
      try {
        const association = await createTaskMR(
          {
            task_id: task.id,
            repository_id: repositoryID,
            mr_url: payload.mr.web_url,
          },
          workspaceId,
        );
        setTaskMR(workspaceId, task.id, association);
      } catch (error) {
        toast({
          title: "Task created, but merge request was not linked",
          description: `Open the task and use Link GitLab merge request to retry. ${
            error instanceof Error ? error.message : "GitLab rejected the association."
          }`,
          variant: "error",
          duration: 8000,
        });
      }
    }
    onClose();
    router.push(linkToTask(task.id));
  };

  return (
    <TaskCreateDialog
      open={true}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      mode="create"
      workspaceId={workspaceId}
      workflowId={workflow.id}
      defaultStepId={firstStep.id}
      steps={workflowSteps.map((step) => ({
        id: step.id,
        title: step.name,
        events: step.events,
      }))}
      initialValues={initialValues}
      onSuccess={handleSuccess}
    />
  );
}
