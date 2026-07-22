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
