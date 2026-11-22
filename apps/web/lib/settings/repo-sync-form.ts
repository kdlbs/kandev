import type { WorkflowSyncProvider } from "@/lib/types/workflow-sync";
import { PUBLIC_GITHUB_HOST } from "@/lib/github/host";
import { buildGitHubRepoUrl, parseGitHubRepoUrl } from "@/lib/utils/github-repo-url";
import { parseGitLabProjectUrl } from "@/lib/utils/gitlab-repo-url";

// The default Prompt Sync and Workflow Sync directories.
export const DEFAULT_PROMPTS_PATH = ".kandev/prompts"; // i18n-exempt: repository path
export const DEFAULT_WORKFLOWS_PATH = ".kandev/workflows"; // i18n-exempt: repository path

/** Editable repository source shared by Workflow Sync and Prompt Sync. */
export type RepoSyncFormState = {
  github_host: string;
  provider: WorkflowSyncProvider;
  repo_owner: string;
  repo_name: string;
  project_path: string;
  branch: string;
  path: string;
  interval_seconds: number;
  poll_enabled: boolean;
};

/** The repository coordinates a saved sync configuration carries. */
export type RepoSyncTarget = Pick<
  RepoSyncFormState,
  "provider" | "repo_owner" | "repo_name" | "project_path" | "branch" | "path"
> & { github_host?: string };

/** The editable repository source of a saved sync configuration. */
export function repoFormFromConfig(
  cfg: RepoSyncTarget & Pick<RepoSyncFormState, "interval_seconds" | "poll_enabled">,
): RepoSyncFormState {
  return {
    provider: cfg.provider,
    github_host: cfg.provider === "gitlab" ? "" : (cfg.github_host ?? PUBLIC_GITHUB_HOST),
    repo_owner: cfg.repo_owner,
    repo_name: cfg.repo_name,
    project_path: cfg.project_path,
    branch: cfg.branch,
    path: cfg.path,
    interval_seconds: cfg.interval_seconds,
    poll_enabled: cfg.poll_enabled,
  };
}

/** Request payload for the repository part of a sync configuration. */
type RepoSyncPayload = {
  provider: WorkflowSyncProvider;
  github_host?: string;
  repo_owner?: string;
  repo_name?: string;
  project_path?: string;
  branch: string;
  path: string;
  interval_seconds: number;
  poll_enabled: boolean;
};

// displayUrl redisplays only the repo identity (owner/repo, or GitLab
// project_path), never branch or directory: those are their own editable
// fields because a branch name can contain slashes, which makes splitting a
// combined string ambiguous.
export function displayUrl(
  form: Pick<
    RepoSyncFormState,
    "provider" | "repo_owner" | "repo_name" | "project_path" | "github_host"
  >,
): string {
  if (form.provider === "gitlab") {
    return form.project_path;
  }
  return form.repo_owner
    ? buildGitHubRepoUrl({ owner: form.repo_owner, repo: form.repo_name, host: form.github_host })
    : "";
}

function parseRepoUrl(provider: WorkflowSyncProvider, value: string, host?: string) {
  return provider === "gitlab" ? parseGitLabProjectUrl(value) : parseGitHubRepoUrl(value, host);
}

/** Reports whether a non-empty link is not recognized for the provider. */
export function repoUrlInvalid(
  form: Pick<RepoSyncFormState, "provider" | "github_host">,
  url: string,
  configuredHost?: string,
): boolean {
  return (
    !!url.trim() &&
    !parseRepoUrl(form.provider, url, configuredHost) &&
    !parseRepoUrl(form.provider, url, form.github_host)
  );
}

// formFromUrl applies a pasted link to the form. Branch and directory change
// only when the link carried them, so a bare repository keeps their values.
export function formFromUrl<T extends RepoSyncFormState>(
  form: T,
  url: string,
  configuredHost?: string,
): T {
  const parsed =
    parseRepoUrl(form.provider, url, configuredHost) ??
    parseRepoUrl(form.provider, url, form.github_host);
  if (!parsed) return form;
  if (form.provider === "gitlab" && "projectPath" in parsed) {
    return {
      ...form,
      project_path: parsed.projectPath,
      branch: parsed.branch ?? form.branch,
      path: parsed.path ?? form.path,
    };
  }
  if (form.provider === "github" && "owner" in parsed) {
    return {
      ...form,
      github_host: parsed.host ?? PUBLIC_GITHUB_HOST,
      repo_owner: parsed.owner,
      repo_name: parsed.repo,
      branch: parsed.branch ?? form.branch,
      path: parsed.path ?? form.path,
    };
  }
  return form;
}

/** Clears the other provider's coordinates when the provider changes. */
export function formForProvider<T extends RepoSyncFormState>(
  form: T,
  provider: WorkflowSyncProvider,
  configuredHost?: string,
): T {
  return {
    ...form,
    provider,
    github_host: provider === "gitlab" ? "" : (configuredHost ?? PUBLIC_GITHUB_HOST),
    repo_owner: "",
    repo_name: "",
    project_path: "",
  };
}

/** Builds the provider-specific repository payload from a form. */
export function repoSyncPayload(form: RepoSyncFormState): RepoSyncPayload {
  const common = {
    branch: form.branch.trim(),
    path: form.path.trim(),
    interval_seconds: form.interval_seconds,
    poll_enabled: form.poll_enabled,
  };
  return form.provider === "gitlab"
    ? { provider: "gitlab", project_path: form.project_path.trim(), ...common }
    : {
        provider: "github",
        github_host: form.github_host,
        repo_owner: form.repo_owner.trim(),
        repo_name: form.repo_name.trim(),
        ...common,
      };
}

/** Reports whether the form lacks the provider's required repository. */
export function repoTargetMissing(form: RepoSyncFormState): boolean {
  return form.provider === "gitlab"
    ? !form.project_path.trim()
    : !form.repo_owner.trim() || !form.repo_name.trim();
}

/** Reports whether polling is on with an interval below the 60s minimum. */
export function intervalInvalid(form: RepoSyncFormState): boolean {
  return (
    form.poll_enabled && (!Number.isInteger(form.interval_seconds) || form.interval_seconds < 60)
  );
}
