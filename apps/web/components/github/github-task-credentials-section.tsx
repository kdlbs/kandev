"use client";

import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { TaskGitCredentialsState } from "@/hooks/domains/github/use-task-git-credentials";
import type { TaskGitCredentialsMode } from "@/lib/types/github";
import { GitHubAccessHelp } from "./github-access-help";

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
    <div
      className="flex items-center gap-1 text-xs text-muted-foreground"
      data-testid="github-task-access-summary"
    >
      <GitHubAccessHelp
        label="Explain task Git access"
        title="Task Git access"
        description="Controls how newly launched tasks authenticate to GitHub for Git HTTPS and gh commands. Managed workspace credentials use Kandev's workspace connection; executor credentials use the selected executor's Git or SSH setup."
      />
      <span>
        <span className="font-medium text-foreground">Task access: </span>
        {value}
      </span>
    </div>
  );
}

export function GitHubTaskAccessForm({
  taskAccess,
  value,
  onChange,
  disabled,
}: {
  taskAccess: TaskGitCredentialsState;
  value: TaskGitCredentialsMode;
  onChange: (value: TaskGitCredentialsMode) => void;
  disabled?: boolean;
}) {
  return (
    <section className="space-y-4 border-t pt-5" data-testid="github-task-access-settings">
      <div className="space-y-1">
        <div className="flex items-center gap-1">
          <h3 className="text-sm font-medium">Task Git access</h3>
          <GitHubAccessHelp
            label="Explain how managed task credentials work"
            title="How managed task credentials work"
            description="With Managed workspace credentials, Kandev configures agentctl as Git's credential helper inside newly launched task processes. When Git requests credentials for an attached GitHub HTTPS repository, the helper redeems a task- and repository-scoped broker lease. The credential is returned to Git on demand and is not written to the repository or Git config. The gh command uses Kandev's broker-aware shim. Executor inheritance skips both and uses credentials visible in the selected executor. Changes apply to new task launches and the next resume, not already-running processes."
          />
        </div>
        <p className="text-xs leading-5 text-muted-foreground">
          Choose how newly launched tasks authenticate to GitHub. This does not change the workspace
          automation connection above.
        </p>
      </div>
      <RadioGroup
        value={value}
        onValueChange={(nextValue) => onChange(nextValue as TaskGitCredentialsMode)}
        disabled={taskAccess.loading || taskAccess.error || disabled}
        className="gap-2"
      >
        <label
          className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3"
          data-testid="github-task-access-option-managed"
        >
          <RadioGroupItem value="managed" className="mt-0.5" />
          <span>
            <span className="font-medium">Managed workspace credentials</span>
            <span className="mt-1 block text-xs leading-5 text-muted-foreground">
              Kandev brokers the workspace PAT, named GitHub CLI account, or App identity to this
              task for GitHub HTTPS and gh.
            </span>
          </span>
        </label>
        <label
          className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3"
          data-testid="github-task-access-option-executor"
        >
          <RadioGroupItem value="executor" className="mt-0.5" />
          <span>
            <span className="font-medium">Inherit executor Git credentials</span>
            <span className="mt-1 block text-xs leading-5 text-muted-foreground">
              Local and Worktree tasks use host-visible Git or SSH credentials. Docker, SSH, and
              cloud tasks use credentials configured in that executor.
            </span>
          </span>
        </label>
      </RadioGroup>
      <p className="text-xs leading-5 text-muted-foreground">
        An executor-profile GH_TOKEN or GITHUB_TOKEN overrides managed workspace credentials for
        that task.
      </p>
      {taskAccess.error && (
        <p className="text-sm text-destructive">Unable to load the current task access setting.</p>
      )}
    </section>
  );
}
