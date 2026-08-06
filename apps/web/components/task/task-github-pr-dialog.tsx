"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useToast } from "@/components/toast-provider";
import { createTaskPR } from "@/lib/api/domains/github-api";
import type { Repository } from "@/lib/types/http";
import {
  githubReposForTask,
  pullRequestPayload,
  type TaskPullRequestLinkTarget,
} from "./task-github-pr-url";
import { useTranslation } from "react-i18next";

type TaskGitHubPRDialogProps = {
  workspaceId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  task: TaskPullRequestLinkTarget;
  repositories: Repository[];
};

function PRDialogFooter({
  submitting,
  onCancel,
  onSubmit,
}: {
  submitting: boolean;
  onCancel: () => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DialogFooter className="gap-2">
      <Button
        type="button"
        variant="outline"
        className="cursor-pointer"
        onClick={onCancel}
        disabled={submitting}
      >
        {t("common:cancel")}
      </Button>
      <Button
        type="button"
        className="cursor-pointer"
        onClick={onSubmit}
        disabled={submitting}
        data-testid="task-github-pr-submit"
      >
        {submitting ? t("task:saving2") : t("common:save")}
      </Button>
    </DialogFooter>
  );
}

export function TaskGitHubPRDialog({
  workspaceId,
  open,
  onOpenChange,
  task,
  repositories,
}: TaskGitHubPRDialogProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [input, setInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const githubRepos = useMemo(() => githubReposForTask(task, repositories), [task, repositories]);
  const inferredRepo = githubRepos.length === 1 ? githubRepos[0] : null;
  const placeholder = inferredRepo
    ? t("task:githubPrRefPlaceholder")
    : "github.com/owner/repo/pull/1471";

  useEffect(() => {
    if (open) {
      setInput("");
      setError(null);
    }
  }, [open]);

  const submit = async () => {
    if (!workspaceId) {
      setError("Select a workspace before linking a GitHub pull request.");
      return;
    }
    if (!input.trim()) {
      setError("Enter a GitHub pull request URL or number.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const payload = pullRequestPayload(input, githubRepos);
      await createTaskPR({
        workspace_id: workspaceId,
        task_id: task.id,
        pr_url: payload.pr_url,
        ...(payload.repository_id ? { repository_id: payload.repository_id } : {}),
      });
      toast({ description: t("task:githubPullRequestLinked"), variant: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to link GitHub pull request.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] sm:max-w-lg">
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
        <div className="space-y-2">
          <Label htmlFor="task-github-pr-input">{t("task:pullRequest")}</Label>
          <Input
            id="task-github-pr-input"
            data-testid="task-github-pr-input"
            value={input}
            onChange={(event) => setInput(event.target.value)}
            placeholder={placeholder}
            disabled={submitting}
          />
          {error && (
            <p className="text-xs text-destructive" data-testid="task-github-pr-error">
              {error}
            </p>
          )}
        </div>
        <PRDialogFooter
          submitting={submitting}
          onCancel={() => onOpenChange(false)}
          onSubmit={submit}
        />
      </DialogContent>
    </Dialog>
  );
}
