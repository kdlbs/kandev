import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  WakeupEntry,
  DashboardData,
  OrchestrateIssue,
} from "@/lib/state/slices/orchestrate/types";

const BASE = "/api/v1/orchestrate";

// --- Channels ---

export function listChannels(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<{
    channels: Array<{
      id: string;
      platform: string;
      config: string;
      status: string;
      task_id: string;
      created_at: string;
    }>;
  }>(`${BASE}/agents/${agentId}/channels`, options);
}

export function setupChannel(
  agentId: string,
  data: { workspace_id: string; platform: string; config: string; status: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{
    channel: {
      id: string;
      platform: string;
      config: string;
      status: string;
      task_id: string;
      created_at: string;
    };
  }>(`${BASE}/agents/${agentId}/channels`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function deleteChannel(agentId: string, channelId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/agents/${agentId}/channels/${channelId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Config Export/Import ---

export function exportConfig(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ bundle: Record<string, unknown> }>(
    `${BASE}/workspaces/${workspaceId}/config/export`,
    options,
  );
}

export const exportConfigZipUrl = (workspaceId: string) =>
  `${BASE}/workspaces/${workspaceId}/config/export/zip`;

export function previewImport(
  workspaceId: string,
  bundle: Record<string, unknown>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{
    preview: {
      agents: { created: string[]; updated: string[]; deleted: string[] };
      skills: { created: string[]; updated: string[]; deleted: string[] };
      routines: { created: string[]; updated: string[]; deleted: string[] };
      projects: { created: string[]; updated: string[]; deleted: string[] };
    };
  }>(`${BASE}/workspaces/${workspaceId}/config/preview`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(bundle), ...options?.init },
  });
}

export function applyImport(
  workspaceId: string,
  bundle: Record<string, unknown>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ result: { created_count: number; updated_count: number } }>(
    `${BASE}/workspaces/${workspaceId}/config/import`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify(bundle), ...options?.init },
    },
  );
}

// --- Config Sync (FS <-> DB) ---

export type ImportDiff = {
  created: string[];
  updated: string[];
  deleted: string[];
};

export type ImportPreview = {
  agents: ImportDiff;
  skills: ImportDiff;
  routines: ImportDiff;
  projects: ImportDiff;
};

export type ParseError = {
  workspace_id: string;
  file_path: string;
  error: string;
};

export type SyncDiff = {
  direction: "incoming" | "outgoing";
  preview: ImportPreview;
  errors?: ParseError[];
};

export function getIncomingDiff(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ diff: SyncDiff }>(
    `${BASE}/workspaces/${workspaceId}/config/sync/incoming`,
    options,
  );
}

export function getOutgoingDiff(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ diff: SyncDiff }>(
    `${BASE}/workspaces/${workspaceId}/config/sync/outgoing`,
    options,
  );
}

export function applyIncomingSync(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ result: { created_count: number; updated_count: number } }>(
    `${BASE}/workspaces/${workspaceId}/config/sync/import-fs`,
    { ...options, init: { method: "POST", ...options?.init } },
  );
}

export function applyOutgoingSync(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ status: string }>(`${BASE}/workspaces/${workspaceId}/config/sync/export-fs`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

// --- Issues ---

export function listIssues(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ issues: OrchestrateIssue[] }>(
    `${BASE}/workspaces/${workspaceId}/issues`,
    options,
  );
}

export function getIssue(issueId: string, options?: ApiRequestOptions) {
  return fetchJson<{ issue: OrchestrateIssue }>(`${BASE}/issues/${issueId}`, options);
}

// --- Comments ---

export type TaskCommentResponse = {
  id: string;
  taskId: string;
  authorType: string;
  authorId: string;
  body: string;
  source: string;
  createdAt: string;
};

export function listComments(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ comments: TaskCommentResponse[] }>(
    `${BASE}/tasks/${taskId}/comments`,
    options,
  );
}

export function createComment(
  taskId: string,
  data: { body: string; author_type?: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ comment: TaskCommentResponse }>(`${BASE}/tasks/${taskId}/comments`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function searchTasks(
  workspaceId: string,
  query: string,
  limit = 50,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ q: query, limit: String(limit) });
  return fetchJson<{ tasks: OrchestrateIssue[] }>(
    `${BASE}/workspaces/${workspaceId}/tasks/search?${params.toString()}`,
    options,
  );
}

// --- Instructions ---

export function listInstructions(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<{
    files: Array<{
      id: string;
      filename: string;
      content: string;
      is_entry: boolean;
      created_at: string;
      updated_at: string;
    }>;
  }>(`${BASE}/agents/${agentId}/instructions`, options);
}

export function getInstruction(agentId: string, filename: string, options?: ApiRequestOptions) {
  return fetchJson<{
    file: {
      id: string;
      filename: string;
      content: string;
      is_entry: boolean;
      created_at: string;
      updated_at: string;
    };
  }>(`${BASE}/agents/${agentId}/instructions/${encodeURIComponent(filename)}`, options);
}

export function upsertInstruction(
  agentId: string,
  filename: string,
  content: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{
    file: {
      id: string;
      filename: string;
      content: string;
      is_entry: boolean;
      created_at: string;
      updated_at: string;
    };
  }>(`${BASE}/agents/${agentId}/instructions/${encodeURIComponent(filename)}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify({ content }), ...options?.init },
  });
}

export function deleteInstruction(agentId: string, filename: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/agents/${agentId}/instructions/${encodeURIComponent(filename)}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Onboarding ---

export type OnboardingStateData = {
  completed: boolean;
  workspaceId?: string;
  ceoAgentId?: string;
};

export function getOnboardingState(options?: ApiRequestOptions) {
  return fetchJson<OnboardingStateData>(`${BASE}/onboarding-state`, options);
}

export type OnboardingCompletePayload = {
  workspaceName: string;
  taskPrefix: string;
  agentName: string;
  agentProfileId: string;
  executorPreference: string;
  taskTitle?: string;
  taskDescription?: string;
};

export type OnboardingCompleteResult = {
  workspaceId: string;
  agentId: string;
  projectId?: string;
  taskId?: string;
};

export function completeOnboarding(data: OnboardingCompletePayload, options?: ApiRequestOptions) {
  return fetchJson<OnboardingCompleteResult>(`${BASE}/onboarding/complete`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

// --- Dashboard ---

export function getDashboard(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<DashboardData>(`${BASE}/workspaces/${workspaceId}/dashboard`, options);
}

// --- Wakeups ---

export function listWakeups(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ wakeups: WakeupEntry[] }>(
    `${BASE}/workspaces/${workspaceId}/wakeups`,
    options,
  );
}

// --- Workspace Settings ---

export function updateWorkspaceSettings(
  workspaceId: string,
  data: {
    name?: string;
    description?: string;
    require_approval_for_new_agents?: boolean;
    require_approval_for_task_completion?: boolean;
    require_approval_for_skill_changes?: boolean;
  },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/settings`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
}

// --- Git ---

export type GitStatusData = {
  is_git: boolean;
  branch?: string;
  is_dirty: boolean;
  has_remote: boolean;
  ahead: number;
  behind: number;
  commit_count: number;
};

export function getGitStatus(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<GitStatusData>(`${BASE}/workspaces/${workspaceId}/git/status`, options);
}

export function gitClone(
  workspaceId: string,
  data: { repoUrl: string; branch: string; workspaceName: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/git/clone`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function gitPull(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/git/pull`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

export function gitPush(
  workspaceId: string,
  data: { message: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/git/push`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}
