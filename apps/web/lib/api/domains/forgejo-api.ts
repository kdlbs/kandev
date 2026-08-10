import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  ForgejoActionPreset,
  ForgejoConfig,
  ForgejoIssue,
  ForgejoIssueWatch,
  ForgejoPullRequest,
  ForgejoPullRequestComment,
  ForgejoPullRequestDetails,
  ForgejoPullRequestReview,
  ForgejoRepository,
  ForgejoReviewWatch,
  ForgejoTaskIssue,
  ForgejoTaskPR,
  SetForgejoConfigRequest,
  TestForgejoConnectionResult,
} from "@/lib/types/forgejo";

type WorkspaceOptions = ApiRequestOptions & { workspaceId: string };
type PaginationOptions = WorkspaceOptions & { page?: number; limit?: number };
const path = (route: string, workspaceId: string) =>
  `${route}${route.includes("?") ? "&" : "?"}workspace_id=${encodeURIComponent(workspaceId)}`;
const withoutWorkspace = ({ workspaceId: _workspaceId, ...options }: WorkspaceOptions) => options;
const paginatedPath = (route: string, options: PaginationOptions) => {
  const params = new URLSearchParams();
  if (options.page) params.set("page", String(options.page));
  if (options.limit) params.set("limit", String(options.limit));
  const suffix = params.toString();
  return path(`${route}${route.includes("?") ? "&" : "?"}${suffix}`, options.workspaceId);
};

export const getForgejoConfig = async (options: WorkspaceOptions) =>
  (await fetchJson<ForgejoConfig | undefined>(
    path("/api/v1/forgejo/config", options.workspaceId),
    withoutWorkspace(options),
  )) ?? null;
export const setForgejoConfig = (payload: SetForgejoConfigRequest, options: WorkspaceOptions) =>
  fetchJson<ForgejoConfig>(path("/api/v1/forgejo/config", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "PUT", body: JSON.stringify(payload) },
  });
export const deleteForgejoConfig = (options: WorkspaceOptions) =>
  fetchJson<{ deleted: boolean }>(path("/api/v1/forgejo/config", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "DELETE" },
  });
export const testForgejoConfig = (payload: SetForgejoConfigRequest, options: WorkspaceOptions) =>
  fetchJson<TestForgejoConnectionResult>(path("/api/v1/forgejo/config/test", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "POST", body: JSON.stringify(payload) },
  });
export const refreshForgejoConnection = (options: WorkspaceOptions) =>
  fetchJson<ForgejoConfig>(path("/api/v1/forgejo/connection/refresh", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "POST" },
  });
export const listForgejoRepositories = (options: PaginationOptions) =>
  fetchJson<{ repositories: ForgejoRepository[]; total_count: number }>(
    paginatedPath("/api/v1/forgejo/repositories", options),
    withoutWorkspace(options),
  );
export const listForgejoQueue = (options: WorkspaceOptions) =>
  fetchJson<{
    issues: { repository: ForgejoRepository; issue: ForgejoIssue }[];
    pull_requests: { repository: ForgejoRepository; pull_request: ForgejoPullRequest }[];
  }>(path("/api/v1/forgejo/queue", options.workspaceId), withoutWorkspace(options));
export const listForgejoIssues = (owner: string, repo: string, options: PaginationOptions) =>
  fetchJson<{ issues: ForgejoIssue[]; total_count: number }>(
    paginatedPath(
      `/api/v1/forgejo/issues?owner=${encodeURIComponent(owner)}&repo=${encodeURIComponent(repo)}`,
      options,
    ),
    withoutWorkspace(options),
  );
export const getForgejoPullRequestDetails = (
  owner: string,
  repo: string,
  number: number,
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoPullRequestDetails>(
    path(
      `/api/v1/forgejo/pull-request-details?owner=${encodeURIComponent(owner)}&repo=${encodeURIComponent(repo)}&number=${number}`,
      options.workspaceId,
    ),
    withoutWorkspace(options),
  );
export const createForgejoPullRequestComment = (
  body: { owner: string; repo: string; number: number; body: string },
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoPullRequestComment>(
    path("/api/v1/forgejo/pull-request-comments", options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "POST", body: JSON.stringify(body) } },
  );
export const submitForgejoPullRequestReview = (
  body: {
    owner: string;
    repo: string;
    number: number;
    body?: string;
    event: "APPROVE" | "REQUEST_CHANGES" | "COMMENT";
  },
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoPullRequestReview>(
    path("/api/v1/forgejo/pull-request-reviews", options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "POST", body: JSON.stringify(body) } },
  );
export const listForgejoTaskIssues = (taskId: string, options: WorkspaceOptions) =>
  fetchJson<{ issues: ForgejoTaskIssue[] }>(
    path(`/api/v1/forgejo/tasks/${encodeURIComponent(taskId)}/issues`, options.workspaceId),
    withoutWorkspace(options),
  );
export const listForgejoTaskPRs = (taskId: string, options: WorkspaceOptions) =>
  fetchJson<{ pull_requests: ForgejoTaskPR[] }>(
    path(`/api/v1/forgejo/tasks/${encodeURIComponent(taskId)}/pull-requests`, options.workspaceId),
    withoutWorkspace(options),
  );
export const linkForgejoIssue = (
  body: { task_id: string; repository_id?: string; owner: string; repo: string; number: number },
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoTaskIssue>(path("/api/v1/forgejo/task-issues", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "POST", body: JSON.stringify(body) },
  });
export const linkForgejoPullRequest = (
  body: { task_id: string; repository_id?: string; owner: string; repo: string; number: number },
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoTaskPR>(path("/api/v1/forgejo/task-pull-requests", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "POST", body: JSON.stringify(body) },
  });
export const createForgejoTaskPullRequest = (
  body: {
    task_id: string;
    repository_id?: string;
    owner: string;
    repo: string;
    title: string;
    body?: string;
    head: string;
    base: string;
    draft?: boolean;
  },
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoTaskPR>(path("/api/v1/forgejo/task-pull-requests/create", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "POST", body: JSON.stringify(body) },
  });
export const unlinkForgejoIssue = (linkId: string, options: WorkspaceOptions) =>
  fetchJson<{ deleted: boolean }>(
    path(`/api/v1/forgejo/task-issues/${encodeURIComponent(linkId)}`, options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "DELETE" } },
  );
export const unlinkForgejoPullRequest = (linkId: string, options: WorkspaceOptions) =>
  fetchJson<{ deleted: boolean }>(
    path(`/api/v1/forgejo/task-pull-requests/${encodeURIComponent(linkId)}`, options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "DELETE" } },
  );
export const refreshForgejoTaskIssue = (linkId: string, options: WorkspaceOptions) =>
  fetchJson<ForgejoTaskIssue>(
    path(`/api/v1/forgejo/task-issues/${encodeURIComponent(linkId)}/refresh`, options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "POST" } },
  );
export const refreshForgejoTaskPullRequest = (linkId: string, options: WorkspaceOptions) =>
  fetchJson<ForgejoTaskPR>(
    path(
      `/api/v1/forgejo/task-pull-requests/${encodeURIComponent(linkId)}/refresh`,
      options.workspaceId,
    ),
    { ...withoutWorkspace(options), init: { method: "POST" } },
  );
export const listForgejoIssueWatches = (options: WorkspaceOptions) =>
  fetchJson<{ watches: ForgejoIssueWatch[] }>(
    path("/api/v1/forgejo/issue-watches", options.workspaceId),
    withoutWorkspace(options),
  );
export const saveForgejoIssueWatch = (
  watch: Partial<ForgejoIssueWatch> & { owner: string; repo: string },
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoIssueWatch>(path("/api/v1/forgejo/issue-watches", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "PUT", body: JSON.stringify(watch) },
  });
export const deleteForgejoIssueWatch = (watchId: string, options: WorkspaceOptions) =>
  fetchJson<{ deleted: boolean }>(
    path(`/api/v1/forgejo/issue-watches/${encodeURIComponent(watchId)}`, options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "DELETE" } },
  );
export const pollForgejoIssueWatch = (watchId: string, options: WorkspaceOptions) =>
  fetchJson<{ issues: ForgejoIssue[] }>(
    path(`/api/v1/forgejo/issue-watches/${encodeURIComponent(watchId)}/poll`, options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "POST" } },
  );
export const listForgejoReviewWatches = (options: WorkspaceOptions) =>
  fetchJson<{ watches: ForgejoReviewWatch[] }>(
    path("/api/v1/forgejo/review-watches", options.workspaceId),
    withoutWorkspace(options),
  );
export const saveForgejoReviewWatch = (
  watch: Partial<ForgejoReviewWatch> & { owner: string; repo: string },
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoReviewWatch>(path("/api/v1/forgejo/review-watches", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "PUT", body: JSON.stringify(watch) },
  });
export const deleteForgejoReviewWatch = (watchId: string, options: WorkspaceOptions) =>
  fetchJson<{ deleted: boolean }>(
    path(`/api/v1/forgejo/review-watches/${encodeURIComponent(watchId)}`, options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "DELETE" } },
  );
export const pollForgejoReviewWatch = (watchId: string, options: WorkspaceOptions) =>
  fetchJson<{ pull_requests: ForgejoPullRequest[] }>(
    path(`/api/v1/forgejo/review-watches/${encodeURIComponent(watchId)}/poll`, options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "POST" } },
  );
export const listForgejoActionPresets = (options: WorkspaceOptions) =>
  fetchJson<{ presets: ForgejoActionPreset[] }>(
    path("/api/v1/forgejo/action-presets", options.workspaceId),
    withoutWorkspace(options),
  );
export const saveForgejoActionPreset = (
  preset: Partial<ForgejoActionPreset> & { kind: string; name: string },
  options: WorkspaceOptions,
) =>
  fetchJson<ForgejoActionPreset>(path("/api/v1/forgejo/action-presets", options.workspaceId), {
    ...withoutWorkspace(options),
    init: { method: "PUT", body: JSON.stringify(preset) },
  });
export const deleteForgejoActionPreset = (presetId: string, options: WorkspaceOptions) =>
  fetchJson<{ deleted: boolean }>(
    path(`/api/v1/forgejo/action-presets/${encodeURIComponent(presetId)}`, options.workspaceId),
    { ...withoutWorkspace(options), init: { method: "DELETE" } },
  );
