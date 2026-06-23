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

type TaskPullRequest = {
  id: string;
  title: string;
  repositoryId?: string;
  repositories?: Array<{ id?: string; repository_id: string }>;
};

type GitHubRepoRef = {
  owner: string;
  repo: string;
  taskRepositoryId?: string;
};

type TaskGitHubPRDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  task: TaskPullRequest;
  repositories: Repository[];
};

type ParsedPullRequestURL = {
  owner: string;
  repo: string;
  number: number;
  url: string;
};

function githubReposForTask(task: TaskPullRequest, repositories: Repository[]): GitHubRepoRef[] {
  const taskRepoIdsByRepoId = new Map<string, string | undefined>();
  for (const repo of task.repositories ?? []) {
    taskRepoIdsByRepoId.set(repo.repository_id, repo.id);
  }
  if (task.repositoryId && !taskRepoIdsByRepoId.has(task.repositoryId)) {
    taskRepoIdsByRepoId.set(task.repositoryId, undefined);
  }

  return repositories
    .filter((repo) => taskRepoIdsByRepoId.has(repo.id) && repo.provider === "github")
    .map((repo) => ({
      owner: repo.provider_owner,
      repo: repo.provider_name,
      taskRepositoryId: taskRepoIdsByRepoId.get(repo.id),
    }))
    .filter((repo) => repo.owner && repo.repo);
}

function parseGitHubPullRequestURL(input: string): ParsedPullRequestURL | null {
  const match = input
    .trim()
    .match(/^(?:https?:\/\/)?github\.com\/([^/\s]+)\/([^/\s]+)\/pull\/(\d+)(?:[/?#].*)?$/i);
  if (!match) return null;
  const [, owner, repo, number] = match;
  return {
    owner,
    repo,
    number: Number(number),
    url: `https://github.com/${owner}/${repo}/pull/${number}`,
  };
}

function pullRequestPayload(input: string, githubRepos: GitHubRepoRef[]) {
  const trimmed = input.trim();
  const inferredRepo = /^#?\d+$/.test(trimmed) && githubRepos.length === 1 ? githubRepos[0] : null;
  if (inferredRepo) {
    const number = trimmed.replace(/^#/, "");
    return {
      pr_url: `https://github.com/${inferredRepo.owner}/${inferredRepo.repo}/pull/${number}`,
      repository_id: inferredRepo.taskRepositoryId,
    };
  }

  const parsed = parseGitHubPullRequestURL(trimmed);
  const matchingRepo = parsed
    ? githubRepos.find(
        (repo) =>
          repo.owner.toLowerCase() === parsed.owner.toLowerCase() &&
          repo.repo.toLowerCase() === parsed.repo.toLowerCase(),
      )
    : null;

  return {
    pr_url: parsed?.url ?? trimmed,
    repository_id: matchingRepo?.taskRepositoryId,
  };
}

export function TaskGitHubPRDialog({
  open,
  onOpenChange,
  task,
  repositories,
}: TaskGitHubPRDialogProps) {
  const { toast } = useToast();
  const [input, setInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const githubRepos = useMemo(() => githubReposForTask(task, repositories), [task, repositories]);
  const inferredRepo = githubRepos.length === 1 ? githubRepos[0] : null;
  const placeholder = inferredRepo
    ? "#1471 or github.com/owner/repo/pull/1471"
    : "github.com/owner/repo/pull/1471";

  useEffect(() => {
    if (open) {
      setInput("");
      setError(null);
    }
  }, [open]);

  const submit = async () => {
    if (!input.trim()) {
      setError("Enter a GitHub pull request URL or number.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const payload = pullRequestPayload(input, githubRepos);
      await createTaskPR({
        task_id: task.id,
        pr_url: payload.pr_url,
        ...(payload.repository_id ? { repository_id: payload.repository_id } : {}),
      });
      toast({ description: "GitHub pull request linked", variant: "success" });
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
          <DialogTitle>Link GitHub pull request</DialogTitle>
          <DialogDescription>
            {inferredRepo
              ? `Use a full pull request URL or number for ${inferredRepo.owner}/${inferredRepo.repo}.`
              : "Use a full GitHub pull request URL for this task."}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="task-github-pr-input">Pull request</Label>
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
        <DialogFooter className="gap-2">
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            Cancel
          </Button>
          <Button
            type="button"
            className="cursor-pointer"
            onClick={submit}
            disabled={submitting}
            data-testid="task-github-pr-submit"
          >
            {submitting ? "Saving" : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
