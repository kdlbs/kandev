"use client";

import { useMemo } from "react";
import { useRouter } from "next/navigation";
import type { Icon } from "@tabler/icons-react";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { Repository, Task, Workflow, WorkflowStep } from "@/lib/types/http";
import type { GitHubPR, GitHubIssue } from "@/lib/types/github";

export type TaskPreset = {
  id: string;
  label: string;
  hint: string;
  icon: Icon;
  prompt: (opts: { url: string; title: string }) => string;
};

export type LaunchPayload =
  | { kind: "pr"; pr: GitHubPR; preset: TaskPreset }
  | { kind: "issue"; issue: GitHubIssue; preset: TaskPreset };

type DialogState = {
  title: string;
  description: string;
  repositoryId?: string;
  branch?: string;
  checkoutBranch?: string;
  githubUrl?: string;
};

function matchRepo(repos: Repository[], owner: string, name: string): Repository | undefined {
  return repos.find(
    (r) =>
      (r.provider_owner || "").toLowerCase() === owner.toLowerCase() &&
      (r.provider_name || "").toLowerCase() === name.toLowerCase(),
  );
}

function extractPayload(payload: LaunchPayload) {
  if (payload.kind === "pr") {
    return {
      url: payload.pr.html_url,
      title: payload.pr.title,
      owner: payload.pr.repo_owner,
      name: payload.pr.repo_name,
      branch: payload.pr.head_branch,
    };
  }
  return {
    url: payload.issue.html_url,
    title: payload.issue.title,
    owner: payload.issue.repo_owner,
    name: payload.issue.repo_name,
    branch: undefined as string | undefined,
  };
}

function buildDialogState(payload: LaunchPayload, repositories: Repository[]): DialogState {
  const data = extractPayload(payload);
  const repo = matchRepo(repositories, data.owner, data.name);
  const description = payload.preset.prompt({ url: data.url, title: data.title });
  const title = `${payload.preset.label}: ${data.title}`;
  const checkoutBranch = payload.kind === "pr" ? data.branch : undefined;
  if (repo) {
    return {
      title,
      description,
      repositoryId: repo.id,
      branch: data.branch,
      checkoutBranch,
    };
  }
  return {
    title,
    description,
    githubUrl: `github.com/${data.owner}/${data.name}`,
    branch: data.branch,
    checkoutBranch,
  };
}

type QuickTaskLauncherProps = {
  workspaceId: string | null;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  payload: LaunchPayload | null;
  onClose: () => void;
};

export function QuickTaskLauncher({
  workspaceId,
  workflows,
  steps,
  repositories,
  payload,
  onClose,
}: QuickTaskLauncherProps) {
  const router = useRouter();

  const defaultWorkflow = workflows[0];
  const sortedStepsForWorkflow = useMemo(
    () =>
      steps
        .filter((s) => s.workflow_id === defaultWorkflow?.id)
        .sort((a, b) => a.position - b.position),
    [steps, defaultWorkflow],
  );
  const defaultStep = sortedStepsForWorkflow[0];
  const stepsForWorkflow = useMemo(
    () => sortedStepsForWorkflow.map((s) => ({ id: s.id, title: s.name, events: s.events })),
    [sortedStepsForWorkflow],
  );

  const dialog = useMemo(
    () => (payload ? buildDialogState(payload, repositories) : null),
    [payload, repositories],
  );

  const handleOpenChange = (open: boolean) => {
    if (!open) onClose();
  };
  const handleSuccess = (task: Task) => {
    onClose();
    router.push(`/t/${task.id}`);
  };

  if (!workspaceId || !defaultWorkflow || !defaultStep || !dialog) return null;

  return (
    <TaskCreateDialog
      open={true}
      onOpenChange={handleOpenChange}
      mode="create"
      workspaceId={workspaceId}
      workflowId={defaultWorkflow.id}
      defaultStepId={defaultStep.id}
      steps={stepsForWorkflow}
      initialValues={{
        title: dialog.title,
        description: dialog.description,
        repositoryId: dialog.repositoryId,
        branch: dialog.branch,
        checkoutBranch: dialog.checkoutBranch,
        githubUrl: dialog.githubUrl,
      }}
      onSuccess={handleSuccess}
    />
  );
}
