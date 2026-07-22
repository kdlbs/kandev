"use client";

import { useMemo } from "react";
import { useRouter } from "@/lib/routing/client-router";
import type { Icon } from "@tabler/icons-react";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { useAppStore } from "@/components/state-provider";
import { createTaskPR, linkTaskIssue } from "@/lib/api/domains/github-api";
import type { Repository, Task, TaskRepository, Workflow, WorkflowStep } from "@/lib/types/http";
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
  prNumber?: number;
  prBaseBranch?: string;
};

const LOCAL_SOURCE_TYPE = "local";
const REJECT_REPO_MATCH = Number.POSITIVE_INFINITY;

function matchRepo(repos: Repository[], owner: string, name: string): Repository | undefined {
  let best: Repository | undefined;
  let bestRank = REJECT_REPO_MATCH;
  for (const repo of repos) {
    if (!matchesProviderIdentity(repo, owner, name)) continue;
    const rank = repositoryLaunchRank(repo);
    if (rank < bestRank) {
      best = repo;
      bestRank = rank;
    }
  }
  return bestRank === REJECT_REPO_MATCH ? undefined : best;
}

function matchesProviderIdentity(repo: Repository, owner: string, name: string): boolean {
  return (
    (repo.provider_owner || "").toLowerCase() === owner.toLowerCase() &&
    (repo.provider_name || "").toLowerCase() === name.toLowerCase()
  );
}

function repositoryLaunchRank(repo: Repository): number {
  if (!isGitHubRepository(repo)) return REJECT_REPO_MATCH;
  if (isTaskWorktreePath(repo.local_path)) return REJECT_REPO_MATCH;
  if ((repo.source_type || LOCAL_SOURCE_TYPE) !== LOCAL_SOURCE_TYPE) return 0;
  return 1;
}

function isGitHubRepository(repo: Repository): boolean {
  const provider = (repo.provider || "").toLowerCase();
  return provider === "" || provider === "github";
}

function isTaskWorktreePath(path: string | undefined): boolean {
  const normalized = (path || "").replaceAll("\\", "/");
  return normalized.includes("/.kandev/tasks/") || normalized.endsWith("/.kandev/tasks");
}

function emptyToUndefined(value: string | undefined): string | undefined {
  return value ? value : undefined;
}

function prURL(pr: GitHubPR): string {
  return (
    pr.html_url || pr.url || `https://github.com/${pr.repo_owner}/${pr.repo_name}/pull/${pr.number}`
  );
}

// Multi-repo tasks have one task_repository row per repo; pick the one that
// matches the PR's owner/repo. Preference order:
//   1. preferredRepositoryId (the id captured at dialog-time by repo matching)
//   2. checkout_branch matching the PR head — handles cases where dialog
//      didn't set a repositoryId (legacy github_url flow)
//   3. first row (safest fallback for single-repo tasks)
// Branch-only matching can mis-select when two task repos share branch names,
// so the dialog-time hint wins when present.
function pickRepositoryIdForPR(
  taskRepos: TaskRepository[] | undefined,
  pr: GitHubPR,
  preferredRepositoryId?: string,
): string | undefined {
  if (!taskRepos || taskRepos.length === 0) return preferredRepositoryId;
  if (preferredRepositoryId) {
    const preferred = taskRepos.find((r) => r.repository_id === preferredRepositoryId);
    if (preferred) return preferred.repository_id;
  }
  const byBranch = taskRepos.find((r) => r.checkout_branch === pr.head_branch);
  return (byBranch ?? taskRepos[0]).repository_id;
}

function extractPayload(payload: LaunchPayload) {
  if (payload.kind === "pr") {
    return {
      url: prURL(payload.pr),
      title: payload.pr.title,
      owner: payload.pr.repo_owner,
      name: payload.pr.repo_name,
      branch: emptyToUndefined(payload.pr.head_branch),
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
  const description = payload.preset.prompt({ url: data.url, title: data.title });
  const title = `${payload.preset.label}: ${data.title}`;
  // For a PR launch we want the dialog to display and check out the PR's head
  // branch — matching the GitHub-URL-paste flow, where the branch selector
  // auto-resolves to the PR head. Same branch for both: the chip shows it and
  // the worktree checks it out.
  const checkoutBranch = payload.kind === "pr" ? data.branch : undefined;
  if (payload.kind === "pr") {
    return {
      title,
      description,
      githubUrl: data.url,
      branch: data.branch,
      checkoutBranch,
      prNumber: payload.pr.number,
      prBaseBranch: payload.pr.base_branch,
    };
  }
  const repo = matchRepo(repositories, data.owner, data.name);
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
  const upsertTaskIssue = useAppStore((state) => state.upsertTaskIssue);

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
    if (payload?.kind === "pr" && workspaceId) {
      const repositoryId = pickRepositoryIdForPR(
        task.repositories,
        payload.pr,
        dialog?.repositoryId,
      );
      // Fire-and-forget: associating the PR is best-effort. A failure (network,
      // missing GH client) shouldn't block navigation — the existing
      // branch-based poller will still try once the agent starts.
      void createTaskPR({
        workspace_id: workspaceId,
        task_id: task.id,
        repository_id: repositoryId,
        pr_url: prURL(payload.pr),
      }).catch(() => {
        // Silently ignore — the indicator will populate via the poller path
        // (legacy behavior) if branch matching succeeds.
      });
    }
    if (payload?.kind === "issue") {
      void linkTaskIssue(task.id, { issue: payload.issue.html_url })
        .then((issue) => {
          if (workspaceId) upsertTaskIssue(workspaceId, issue);
        })
        .catch(() => {
          // Task creation succeeded; keep navigation available if GitHub linking fails.
        });
    }
    onClose();
    router.push(`/tasks/${task.id}`);
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
        prNumber: dialog.prNumber,
        prBaseBranch: dialog.prBaseBranch,
      }}
      onSuccess={handleSuccess}
    />
  );
}
