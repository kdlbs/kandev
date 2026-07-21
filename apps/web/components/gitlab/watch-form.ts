import type {
  CreateIssueWatchRequest,
  CreateReviewWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import type { IssueWatch, ReviewWatch } from "@/lib/types/gitlab";

export type GitLabWatchKind = "review" | "issue";

export type GitLabWatchForm = {
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  projectPaths: string;
  repositoryId: string;
  baseBranch: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  customQuery: string;
  reviewScope: "user" | "user_and_teams";
  labels: string;
  pollIntervalSeconds: string;
  maxInflightTasks: string;
  cleanupPolicy: "auto" | "always" | "never";
};

const DEFAULT_REVIEW_PROMPT =
  "Review GitLab merge request {{mr.url}}. Summarize risks and leave actionable feedback.";
const DEFAULT_ISSUE_PROMPT =
  "Investigate GitLab issue {{issue.url}} and implement the requested change.";

export function makeWatchForm(kind: GitLabWatchKind, workspaceId: string): GitLabWatchForm {
  return {
    workspaceId,
    workflowId: "",
    workflowStepId: "",
    projectPaths: "",
    repositoryId: "",
    baseBranch: "",
    agentProfileId: "",
    executorProfileId: "",
    prompt: kind === "review" ? DEFAULT_REVIEW_PROMPT : DEFAULT_ISSUE_PROMPT,
    customQuery: "",
    reviewScope: "user",
    labels: "",
    pollIntervalSeconds: "300",
    maxInflightTasks: "",
    cleanupPolicy: "auto",
  };
}

export function watchFormFromWatch(
  kind: GitLabWatchKind,
  watch: ReviewWatch | IssueWatch,
): GitLabWatchForm {
  const form = makeWatchForm(kind, watch.workspace_id);
  return {
    ...form,
    workflowId: watch.workflow_id,
    workflowStepId: watch.workflow_step_id,
    projectPaths: watch.projects.map((project) => project.path).join(", "),
    repositoryId: watch.repository_id ?? "",
    baseBranch: watch.base_branch ?? "",
    agentProfileId: watch.agent_profile_id,
    executorProfileId: watch.executor_profile_id,
    prompt: watch.prompt || form.prompt,
    customQuery: watch.custom_query,
    reviewScope:
      kind === "review" ? normalizeReviewScope((watch as ReviewWatch).review_scope) : "user",
    labels: kind === "issue" ? ((watch as IssueWatch).labels ?? []).join(", ") : "",
    pollIntervalSeconds: String(watch.poll_interval_seconds),
    maxInflightTasks: watch.max_inflight_tasks ? String(watch.max_inflight_tasks) : "",
    cleanupPolicy: normalizeCleanupPolicy(watch.cleanup_policy),
  };
}

export function buildWatchPayload(
  kind: "review",
  form: GitLabWatchForm,
  editing?: boolean,
): CreateReviewWatchRequest | null;
export function buildWatchPayload(
  kind: "issue",
  form: GitLabWatchForm,
  editing?: boolean,
): CreateIssueWatchRequest | null;
export function buildWatchPayload(
  kind: GitLabWatchKind,
  form: GitLabWatchForm,
  editing = false,
): CreateReviewWatchRequest | CreateIssueWatchRequest | null {
  const pollInterval = Number(form.pollIntervalSeconds);
  const hasInflightInput = Boolean(form.maxInflightTasks.trim());
  let inflight: number | undefined;
  if (hasInflightInput) inflight = Number(form.maxInflightTasks);
  else if (editing) inflight = 0;
  if (!isValidForm(form, pollInterval, inflight, hasInflightInput)) return null;
  const common = {
    workspace_id: form.workspaceId,
    workflow_id: form.workflowId,
    workflow_step_id: form.workflowStepId,
    projects: parseList(form.projectPaths).map((path) => ({ path })),
    repository_id: form.repositoryId,
    base_branch: form.baseBranch,
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    prompt: form.prompt.trim(),
    custom_query: form.customQuery.trim(),
    poll_interval_seconds: pollInterval,
    cleanup_policy: form.cleanupPolicy,
    max_inflight_tasks: inflight,
  };
  if (kind === "review") return { ...common, review_scope: form.reviewScope };
  return { ...common, labels: parseList(form.labels) };
}

function isValidForm(
  form: GitLabWatchForm,
  pollInterval: number,
  inflight: number | undefined,
  hasInflightInput: boolean,
): boolean {
  const dependenciesReady = Boolean(
    form.workspaceId && form.workflowId && form.workflowStepId && form.prompt.trim(),
  );
  const pollIntervalValid =
    Number.isInteger(pollInterval) && pollInterval >= 60 && pollInterval <= 3600;
  const inflightValid = !hasInflightInput || (Number.isInteger(inflight) && (inflight ?? 0) >= 1);
  return dependenciesReady && pollIntervalValid && inflightValid;
}

function parseList(value: string): string[] {
  return Array.from(
    new Set(
      value
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  );
}

function normalizeReviewScope(value: string): GitLabWatchForm["reviewScope"] {
  return value === "user_and_teams" ? value : "user";
}

function normalizeCleanupPolicy(value: string): GitLabWatchForm["cleanupPolicy"] {
  if (value === "always" || value === "never") return value;
  return "auto";
}
