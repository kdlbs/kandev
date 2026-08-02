"use client";

import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { TaskGitCredentialsState } from "@/hooks/domains/github/use-task-git-credentials";
import type { TaskGitCredentialsMode } from "@/lib/types/github";
import { GitHubAccessHelp } from "./github-access-help";
import { useTranslation } from "react-i18next";

// Keyed by the wire mode, which is never translated. Catalog keys rather than
// `t()` calls, because this is module scope (see docs/i18n.md).
const taskAccessLabelKeys: Record<TaskGitCredentialsMode, string> = {
  managed: "github:managedWorkspaceCredentials",
  executor: "github:inheritExecutorGitCredentials",
};

export function GitHubTaskAccessSummary({
  mode,
  loading,
  error,
}: Omit<TaskGitCredentialsState, "save">) {
  const { t } = useTranslation();
  let value = t(taskAccessLabelKeys[mode]);
  if (loading) value = t("github:loadingTaskAccess");
  if (error) value = t("github:taskAccessUnavailable");
  return (
    <div
      className="flex items-center gap-1 text-xs text-muted-foreground"
      data-testid="github-task-access-summary"
    >
      <GitHubAccessHelp
        label={t("github:explainTaskGitAccess")}
        title={t("github:taskGitAccess")}
        description={t("github:controlsHowNewlyLaunchedTasksAuthenticate")}
      />
      <span>
        <span className="font-medium text-foreground">{t("github:taskAccess")} </span>
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
  const { t } = useTranslation();
  return (
    <section className="space-y-4 border-t pt-5" data-testid="github-task-access-settings">
      <div className="space-y-1">
        <div className="flex items-center gap-1">
          <h3 className="text-sm font-medium">{t("github:taskGitAccess")}</h3>
          <GitHubAccessHelp
            label={t("github:explainHowManagedTaskCredentialsWork")}
            title={t("github:howManagedTaskCredentialsWork")}
            description={t("github:withManagedWorkspaceCredentialsKandevConfigures")}
          />
        </div>
        <p className="text-xs leading-5 text-muted-foreground">
          {t("github:chooseHowNewlyLaunchedTasksAuthenticate")}
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
            <span className="font-medium">{t("github:managedWorkspaceCredentials")}</span>
            <span className="mt-1 block text-xs leading-5 text-muted-foreground">
              {t("github:kandevBrokersTheWorkspacePatNamed")}
            </span>
          </span>
        </label>
        <label
          className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3"
          data-testid="github-task-access-option-executor"
        >
          <RadioGroupItem value="executor" className="mt-0.5" />
          <span>
            <span className="font-medium">{t("github:inheritExecutorGitCredentials")}</span>
            <span className="mt-1 block text-xs leading-5 text-muted-foreground">
              {t("github:localAndWorktreeTasksUseHost")}
            </span>
          </span>
        </label>
      </RadioGroup>
      <p className="text-xs leading-5 text-muted-foreground">
        {/* Env var names are identifiers the user must type exactly, so they are
            interpolated rather than left for the pseudo-locale to mangle. */}
        {t("github:anExecutorProfileGhTokenOr", {
          ghToken: "GH_TOKEN",
          githubToken: "GITHUB_TOKEN",
        })}
      </p>
      {taskAccess.error && (
        <p className="text-sm text-destructive">{t("github:unableToLoadTheCurrentTask")}</p>
      )}
    </section>
  );
}
