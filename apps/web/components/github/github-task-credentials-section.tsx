"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { useToast } from "@/components/toast-provider";
import type { TaskGitCredentialsState } from "@/hooks/domains/github/use-task-git-credentials";
import type { TaskGitCredentialsMode } from "@/lib/types/github";

const taskAccessLabels: Record<TaskGitCredentialsMode, string> = {
  managed: "Managed workspace credentials",
  executor: "Inherit executor Git credentials",
};

export function GitHubTaskAccessSummary({
  mode,
  loading,
  error,
}: Omit<TaskGitCredentialsState, "save">) {
  let value = taskAccessLabels[mode];
  if (loading) value = "Loading task access…";
  if (error) value = "Task access unavailable";
  return (
    <div className="pt-1 text-xs text-muted-foreground" data-testid="github-task-access-summary">
      <span className="font-medium text-foreground">Task access: </span>
      {value}
    </div>
  );
}

export function GitHubTaskAccessForm({
  open,
  taskAccess,
  onSaved,
  onDraftChange,
}: {
  open: boolean;
  taskAccess: TaskGitCredentialsState;
  onSaved: () => void;
  onDraftChange: (dirty: boolean) => void;
}) {
  const { toast } = useToast();
  const [draft, setDraft] = useState(taskAccess.mode);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) setDraft(taskAccess.mode);
  }, [open, taskAccess.mode]);

  useEffect(() => {
    onDraftChange(draft !== taskAccess.mode);
  }, [draft, onDraftChange, taskAccess.mode]);

  const save = async () => {
    setSaving(true);
    try {
      await taskAccess.save(draft);
      toast({ description: "Task Git access saved", variant: "success" });
      onSaved();
    } catch {
      toast({ description: "Failed to save task Git access", variant: "error" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="space-y-4 border-t pt-5" data-testid="github-task-access-settings">
      <div className="space-y-1">
        <h3 className="font-medium">Task Git access</h3>
        <p className="text-sm text-muted-foreground">
          Choose how newly launched tasks authenticate to GitHub. This does not change the workspace
          automation connection above.
        </p>
      </div>
      <RadioGroup
        value={draft}
        onValueChange={(value) => setDraft(value as TaskGitCredentialsMode)}
        disabled={taskAccess.loading || taskAccess.error || saving}
      >
        <label
          className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3"
          data-testid="github-task-access-option-managed"
        >
          <RadioGroupItem value="managed" className="mt-0.5" />
          <span>
            <span className="font-medium">Managed workspace credentials</span>
            <span className="mt-1 block text-sm text-muted-foreground">
              Kandev brokers the workspace PAT, named GitHub CLI account, or App identity to this
              task for GitHub HTTPS and gh.
            </span>
          </span>
        </label>
        <label
          className="mt-3 flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3"
          data-testid="github-task-access-option-executor"
        >
          <RadioGroupItem value="executor" className="mt-0.5" />
          <span>
            <span className="font-medium">Inherit executor Git credentials</span>
            <span className="mt-1 block text-sm text-muted-foreground">
              Local and Worktree tasks use host-visible Git or SSH credentials. Docker, SSH, and
              cloud tasks use credentials configured in that executor.
            </span>
          </span>
        </label>
      </RadioGroup>
      <p className="text-xs text-muted-foreground">
        An executor-profile GH_TOKEN or GITHUB_TOKEN overrides managed workspace credentials for
        that task.
      </p>
      {taskAccess.error && (
        <p className="text-sm text-destructive">Unable to load the current task access setting.</p>
      )}
      <Button
        type="button"
        disabled={taskAccess.loading || taskAccess.error || saving}
        onClick={save}
        className="h-11 cursor-pointer"
      >
        {saving ? "Saving task access…" : "Save task access"}
      </Button>
    </section>
  );
}
