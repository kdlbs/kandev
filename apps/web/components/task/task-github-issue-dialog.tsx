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
import { linkTaskIssue, unlinkTaskIssue } from "@/lib/api/domains/github-api";
import type { Repository } from "@/lib/types/http";

type TaskIssue = {
  id: string;
  title: string;
  repositoryId?: string;
  issueUrl?: string;
  issueNumber?: number;
  repositories?: Array<{ repository_id: string }>;
};

type GitHubRepoRef = {
  owner: string;
  repo: string;
};

type TaskGitHubIssueDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  task: TaskIssue;
  repositories: Repository[];
};

type DialogFieldsProps = {
  input: string;
  setInput: (input: string) => void;
  inferredRepo: GitHubRepoRef | null;
  submitting: boolean;
  error: string | null;
};

type DialogActionsProps = {
  currentLabel: string | null;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: () => void;
  onUnlink: () => void;
};

function githubReposForTask(task: TaskIssue, repositories: Repository[]): GitHubRepoRef[] {
  const ids = new Set((task.repositories ?? []).map((repo) => repo.repository_id));
  if (task.repositoryId) ids.add(task.repositoryId);
  return repositories
    .filter((repo) => ids.has(repo.id) && repo.provider === "github")
    .map((repo) => ({ owner: repo.provider_owner, repo: repo.provider_name }))
    .filter((repo) => repo.owner && repo.repo);
}

function issuePayload(input: string, inferredRepo: GitHubRepoRef | null) {
  const trimmed = input.trim();
  if (/^#?\d+$/.test(trimmed) && inferredRepo) {
    return {
      issue: trimmed,
      owner: inferredRepo.owner,
      repo: inferredRepo.repo,
      number: Number(trimmed.replace(/^#/, "")),
    };
  }
  return { issue: trimmed };
}

function TaskGitHubIssueFields({
  input,
  setInput,
  inferredRepo,
  submitting,
  error,
}: DialogFieldsProps) {
  const placeholder = inferredRepo
    ? "#1470 or github.com/owner/repo/issues/1470"
    : "github.com/owner/repo/issues/1470";
  return (
    <div className="space-y-2">
      <Label htmlFor="task-github-issue-input">Issue</Label>
      <Input
        id="task-github-issue-input"
        data-testid="task-github-issue-input"
        value={input}
        onChange={(event) => setInput(event.target.value)}
        placeholder={placeholder}
        disabled={submitting}
      />
      {error && (
        <p className="text-xs text-destructive" data-testid="task-github-issue-error">
          {error}
        </p>
      )}
    </div>
  );
}

function TaskGitHubIssueActions({
  currentLabel,
  submitting,
  onCancel,
  onSubmit,
  onUnlink,
}: DialogActionsProps) {
  return (
    <DialogFooter className="gap-2 sm:justify-between">
      {currentLabel ? (
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer"
          onClick={onUnlink}
          disabled={submitting}
        >
          Unlink
        </Button>
      ) : (
        <span />
      )}
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </Button>
        <Button
          type="button"
          className="cursor-pointer"
          onClick={onSubmit}
          disabled={submitting}
          data-testid="task-github-issue-submit"
        >
          {submitting ? "Saving" : "Save"}
        </Button>
      </div>
    </DialogFooter>
  );
}

export function TaskGitHubIssueDialog({
  open,
  onOpenChange,
  task,
  repositories,
}: TaskGitHubIssueDialogProps) {
  const { toast } = useToast();
  const [input, setInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  let currentLabel: string | null = null;
  if (task.issueNumber) {
    currentLabel = `#${task.issueNumber}`;
  } else if (task.issueUrl) {
    currentLabel = "Linked issue";
  }
  const githubRepos = useMemo(() => githubReposForTask(task, repositories), [task, repositories]);
  const inferredRepo = githubRepos.length === 1 ? githubRepos[0] : null;

  useEffect(() => {
    if (open) {
      setInput(task.issueUrl ?? "");
      setError(null);
    }
  }, [open, task.issueUrl]);

  const submit = async () => {
    if (!input.trim()) {
      setError("Enter a GitHub issue URL or number.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await linkTaskIssue(task.id, issuePayload(input, inferredRepo));
      toast({ description: "GitHub issue linked", variant: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to link GitHub issue.");
    } finally {
      setSubmitting(false);
    }
  };

  const unlink = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await unlinkTaskIssue(task.id);
      toast({ description: "GitHub issue unlinked", variant: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to unlink GitHub issue.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{currentLabel ? "Change GitHub issue" : "Link GitHub issue"}</DialogTitle>
          <DialogDescription>
            {inferredRepo
              ? `Use a full issue URL or number for ${inferredRepo.owner}/${inferredRepo.repo}.`
              : "Use a full GitHub issue URL for this task."}
          </DialogDescription>
        </DialogHeader>
        <TaskGitHubIssueFields
          input={input}
          setInput={setInput}
          inferredRepo={inferredRepo}
          submitting={submitting}
          error={error}
        />
        <TaskGitHubIssueActions
          currentLabel={currentLabel}
          submitting={submitting}
          onCancel={() => onOpenChange(false)}
          onSubmit={submit}
          onUnlink={unlink}
        />
      </DialogContent>
    </Dialog>
  );
}
