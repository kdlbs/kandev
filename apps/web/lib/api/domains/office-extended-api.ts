import { fetchJson, fetchJsonWithRetry, type ApiRequestOptions } from "../client";
import type { DashboardData, OfficeTask } from "@/lib/state/slices/office/types";

const BASE = "/api/v1/office";

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

// --- Tasks ---

// listTasks (filter / sort / paginate) lives in office-tasks-api.ts so
// this file stays under the eslint max-lines budget. Re-exported here so
// existing imports (`from "./office-extended-api"`, `office-api`) keep
// working without a churn-y import update across the app.
export { listTasks, type ListTasksParams, type ListTasksResponse } from "./office-tasks-api";

export type TimelineEvent = {
  type: string;
  from?: string;
  to?: string;
  at: string;
};

export function getTask(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ task: OfficeTask; timeline?: TimelineEvent[] }>(
    `${BASE}/tasks/${taskId}`,
    options,
  );
}

// --- Task mutations (PATCH /tasks/:id) ---

export type UpdateTaskPayload = {
  status?: string;
  comment?: string;
  assignee_agent_profile_id?: string;
  priority?: string;
  project_id?: string;
  parent_id?: string;
  reopen?: boolean;
  resume?: boolean;
};

export function updateTask(
  taskId: string,
  payload: UpdateTaskPayload,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/tasks/${taskId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...options?.init },
  });
}

// --- Task blockers ---

export function addTaskBlocker(taskId: string, blockerTaskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ ok: boolean }>(`${BASE}/tasks/${taskId}/blockers`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ blocker_task_id: blockerTaskId }),
      ...options?.init,
    },
  });
}

export function removeTaskBlocker(taskId: string, blockerId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/tasks/${taskId}/blockers/${blockerId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Task participants (reviewers / approvers) ---

/**
 * Per ADR 0005 Wave C-backend, the underlying SQLite table for these
 * participants moves from `office_task_participants` to the dual-scoped
 * `workflow_step_participants`. The HTTP endpoints and `TaskParticipant`
 * wire shape stay the same — the migration is transparent to the client.
 * If the backend introduces step-scoped participants surfaced in the UI
 * (e.g. "approvers for the In-Review step only"), extend this DTO with
 * the new `workflow_step_id` field.
 */
export type TaskParticipant = {
  agent_profile_id: string;
  role: "reviewer" | "approver";
  created_at: string;
};

export function listTaskReviewers(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ participants: TaskParticipant[] }>(
    `${BASE}/tasks/${taskId}/reviewers`,
    options,
  );
}

export function addTaskReviewer(
  taskId: string,
  agentProfileId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/tasks/${taskId}/reviewers`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ agent_profile_id: agentProfileId }),
      ...options?.init,
    },
  });
}

export function removeTaskReviewer(taskId: string, agentId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/tasks/${taskId}/reviewers/${agentId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export function listTaskApprovers(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ participants: TaskParticipant[] }>(
    `${BASE}/tasks/${taskId}/approvers`,
    options,
  );
}

export function addTaskApprover(
  taskId: string,
  agentProfileId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/tasks/${taskId}/approvers`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ agent_profile_id: agentProfileId }),
      ...options?.init,
    },
  });
}

export function removeTaskApprover(taskId: string, agentId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/tasks/${taskId}/approvers/${agentId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Task decisions (approval flow) ---

export type TaskDecisionDTO = {
  id: string;
  task_id: string;
  decider_type: "user" | "agent";
  decider_id: string;
  decider_name?: string;
  role: "reviewer" | "approver";
  decision: "approved" | "changes_requested";
  comment?: string;
  created_at: string;
  superseded_at?: string;
};

const USER_CALLER_HEADER: Record<string, string> = {
  "X-Office-User-Caller": "user",
};

export function approveTask(taskId: string, comment?: string, options?: ApiRequestOptions) {
  return fetchJson<{ decision: TaskDecisionDTO }>(`${BASE}/tasks/${taskId}/approve`, {
    ...options,
    init: {
      method: "POST",
      headers: USER_CALLER_HEADER,
      body: JSON.stringify({ comment: comment ?? "" }),
      ...options?.init,
    },
  }).then((res) => res.decision);
}

export function requestTaskChanges(taskId: string, comment: string, options?: ApiRequestOptions) {
  return fetchJson<{ decision: TaskDecisionDTO }>(`${BASE}/tasks/${taskId}/request-changes`, {
    ...options,
    init: {
      method: "POST",
      headers: USER_CALLER_HEADER,
      body: JSON.stringify({ comment }),
      ...options?.init,
    },
  }).then((res) => res.decision);
}

export function listTaskDecisions(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ decisions: TaskDecisionDTO[] }>(
    `${BASE}/tasks/${taskId}/decisions`,
    options,
  ).then((res) => res.decisions ?? []);
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
  // Optional — populated by the comments API for user comments whose
  // comment_created subscriber queued a task_comment run.
  runId?: string;
  runStatus?: "queued" | "claimed" | "finished" | "failed" | "cancelled";
  runError?: string;
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
  return fetchJson<{ tasks: OfficeTask[] }>(
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

export type OnboardingFSWorkspace = {
  name: string;
};

export type OnboardingStateData = {
  completed: boolean;
  workspaceId?: string;
  ceoAgentId?: string;
  fsWorkspaces: OnboardingFSWorkspace[];
};

export function getOnboardingState(options?: ApiRequestOptions) {
  return fetchJsonWithRetry<OnboardingStateData>(`${BASE}/onboarding-state`, options);
}

export type OnboardingCompletePayload = {
  workspaceName: string;
  taskPrefix: string;
  agentName: string;
  agentProfileId: string;
  executorPreference: string;
  taskTitle?: string;
  taskDescription?: string;
  /**
   * Workspace routing default tier captured by the wizard. The backend
   * normalises empty / unknown values to "balanced", so it is safe to
   * omit when the wizard step is skipped.
   */
  default_tier?: "frontier" | "balanced" | "economy";
};

export type OnboardingCompleteResult = {
  workspaceId: string;
  agentId: string;
  taskId?: string;
};

export function completeOnboarding(data: OnboardingCompletePayload, options?: ApiRequestOptions) {
  return fetchJson<OnboardingCompleteResult>(`${BASE}/onboarding/complete`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export type ImportFromFSResult = {
  workspaceIds: string[];
  importedCount: number;
};

export function importFromFS(options?: ApiRequestOptions) {
  return fetchJson<ImportFromFSResult>(`${BASE}/onboarding/import-fs`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

// --- Task Documents ---

export type TaskDocument = {
  key: string;
  taskId: string;
  type: string;
  title: string;
  content: string;
  revision: number;
  filename?: string;
  size?: number;
  updatedAt: string;
  createdAt: string;
};

export function listDocuments(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ documents: TaskDocument[] }>(`${BASE}/tasks/${taskId}/documents`, options);
}

export function getDocument(taskId: string, key: string, options?: ApiRequestOptions) {
  return fetchJson<{ document: TaskDocument }>(
    `${BASE}/tasks/${taskId}/documents/${encodeURIComponent(key)}`,
    options,
  );
}

export function createOrUpdateDocument(
  taskId: string,
  key: string,
  data: { type?: string; title?: string; content: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ document: TaskDocument }>(
    `${BASE}/tasks/${taskId}/documents/${encodeURIComponent(key)}`,
    {
      ...options,
      init: { method: "PUT", body: JSON.stringify(data), ...options?.init },
    },
  );
}

export function deleteDocument(taskId: string, key: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/tasks/${taskId}/documents/${encodeURIComponent(key)}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Task Labels ---

export function addLabel(
  workspaceId: string,
  taskId: string,
  data: { name: string; color?: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<void>(`${BASE}/workspaces/${workspaceId}/tasks/${taskId}/labels`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function removeLabel(
  workspaceId: string,
  taskId: string,
  labelName: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<void>(
    `${BASE}/workspaces/${workspaceId}/tasks/${taskId}/labels/${encodeURIComponent(labelName)}`,
    {
      ...options,
      init: { method: "DELETE", ...options?.init },
    },
  );
}

// --- Dashboard ---

export function getDashboard(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<DashboardData>(`${BASE}/workspaces/${workspaceId}/dashboard`, options);
}

export * from "./office-runs-api";

// --- Workspace Settings ---

export type WorkspaceSettingsData = {
  name: string;
  description?: string;
  permission_handling_mode: string;
  recovery_lookback_hours: number;
  require_approval_for_new_agents?: boolean;
  require_approval_for_task_completion?: boolean;
  require_approval_for_skill_changes?: boolean;
};

export function getWorkspaceSettings(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ settings: WorkspaceSettingsData }>(
    `${BASE}/workspaces/${workspaceId}/settings`,
    options,
  );
}

export function updateWorkspaceSettings(
  workspaceId: string,
  data: {
    name?: string;
    description?: string;
    require_approval_for_new_agents?: boolean;
    require_approval_for_task_completion?: boolean;
    require_approval_for_skill_changes?: boolean;
    recovery_lookback_hours?: number;
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

// Provider routing endpoints live in office-routing-api.ts; re-export here
// so existing call sites can import from one place if convenient.
export * from "./office-routing-api";

// --- Inbox dismissal ---

/**
 * Mark an inbox item as fixed. For `agent_run_failed`, dismisses the
 * per-task entry and re-queues a run for that (task, agent). For
 * `agent_paused_after_failures`, unpauses the agent, resets the
 * counter, and re-queues runs for all affected tasks whose current
 * assignee is still this agent.
 */
export function dismissInboxItem(
  kind: "agent_run_failed" | "agent_paused_after_failures",
  itemId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/inbox/dismiss`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ kind, item_id: itemId }),
      ...options?.init,
    },
  });
}
