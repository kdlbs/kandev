"use client";

import { useMemo } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { TaskChangeRequestLinkForm } from "@/components/integrations/task-change-request-link-form";
import { createTaskPR } from "@/lib/api/domains/github-api";
import type { Repository } from "@/lib/types/http";
import {
  githubReposForTask,
  pullRequestPayload,
  type TaskPullRequestLinkTarget,
} from "./task-github-pr-url";

type TaskGitHubPRDialogProps = {
  workspaceId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  task: TaskPullRequestLinkTarget;
  repositories: Repository[];
};

export function TaskGitHubPRDialog({
  workspaceId,
  open,
  onOpenChange,
  task,
  repositories,
}: TaskGitHubPRDialogProps) {
  const githubRepos = useMemo(() => githubReposForTask(task, repositories), [task, repositories]);
  const inferredRepo = githubRepos.length === 1 ? githubRepos[0] : null;
  const placeholder = inferredRepo
    ? "#1471 or github.com/owner/repo/pull/1471"
    : "github.com/owner/repo/pull/1471";

  const submit = async (reference: string) => {
    if (!workspaceId) {
      throw new Error("Select a workspace before linking a GitHub pull request.");
    }
    const payload = pullRequestPayload(reference, githubRepos);
    await createTaskPR({
      workspace_id: workspaceId,
      task_id: task.id,
      pr_url: payload.pr_url,
      ...(payload.repository_id ? { repository_id: payload.repository_id } : {}),
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Link GitHub pull request</DialogTitle>
          <DialogDescription>
            {inferredRepo
              ? `Use a full pull request URL or number for ${inferredRepo.owner}/${inferredRepo.repo}.`
              : "Use a full GitHub pull request URL for this task."}
          </DialogDescription>
        </DialogHeader>
        <TaskChangeRequestLinkForm
          inputLabel="Pull request"
          placeholder={placeholder}
          emptyError="Enter a GitHub pull request URL or number."
          failureMessage="Failed to link GitHub pull request."
          successMessage="GitHub pull request linked"
          inputTestId="task-github-pr-input"
          errorTestId="task-github-pr-error"
          submitTestId="task-github-pr-submit"
          resetKey={open}
          onSubmit={submit}
          onCancel={() => onOpenChange(false)}
          onSuccess={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
