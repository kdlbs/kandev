"use client";

import { useMemo, type RefObject } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { TaskChangeRequestLinkForm } from "@/components/integrations/task-change-request-link-form";
import { createTaskPR } from "@/lib/api/domains/github-api";
import { createFocusReturnHandler } from "@/lib/dialog-focus-return";
import type { Repository } from "@/lib/types/http";
import {
  githubReposForTask,
  pullRequestPayload,
  type TaskPullRequestLinkTarget,
} from "./task-github-pr-url";
import { useTranslation } from "react-i18next";

/** URL shape the user types verbatim — protocol, not copy. Passed into the
 * placeholder message as an interpolation value so it survives translation. */
const GITHUB_PR_URL_EXAMPLE = "github.com/owner/repo/pull/1471";

type TaskGitHubPRDialogProps = {
  workspaceId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  task: TaskPullRequestLinkTarget;
  repositories: Repository[];
  /** Element to return keyboard focus to on close (AC-TASKS-TASK-ACTIONS-MENU-001.12).
   * Omitted callers keep Radix's default restore-to-previously-focused-element behavior. */
  focusReturnRef?: RefObject<HTMLElement | null>;
};

export function TaskGitHubPRDialog({
  workspaceId,
  open,
  onOpenChange,
  task,
  repositories,
  focusReturnRef,
}: TaskGitHubPRDialogProps) {
  const { t } = useTranslation();
  const githubRepos = useMemo(() => githubReposForTask(task, repositories), [task, repositories]);
  const inferredRepo = githubRepos.length === 1 ? githubRepos[0] : null;
  const placeholder = inferredRepo
    ? t("task:githubPrRefPlaceholder", { example: GITHUB_PR_URL_EXAMPLE })
    : GITHUB_PR_URL_EXAMPLE;

  const submit = async (reference: string) => {
    if (!workspaceId) {
      throw new Error(t("task:selectWorkspaceBeforeLinkingPr"));
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
      <DialogContent
        className="w-[calc(100vw-2rem)] sm:max-w-lg"
        onCloseAutoFocus={createFocusReturnHandler(focusReturnRef)}
      >
        <DialogHeader>
          <DialogTitle>{t("task:linkGithubPullRequest")}</DialogTitle>
          <DialogDescription>
            {inferredRepo
              ? t("task:useAFullPullRequestUrl", {
                  owner: inferredRepo.owner,
                  repo: inferredRepo.repo,
                })
              : t("task:useAFullGithubPullRequest")}
          </DialogDescription>
        </DialogHeader>
        <TaskChangeRequestLinkForm
          inputLabel={t("task:pullRequest")}
          placeholder={placeholder}
          emptyError={t("task:enterGithubPrUrlOrNumber")}
          failureMessage={t("task:failedToLinkGithubPullRequest")}
          successMessage={t("task:githubPullRequestLinked")}
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
