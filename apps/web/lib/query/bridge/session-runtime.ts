import type { QueryClient } from "@tanstack/react-query";
import { invalidateCumulativeDiffCache } from "@/hooks/domains/session/use-cumulative-diff";
import { hasGitStatusChanged } from "@/lib/state/slices/session-runtime/session-runtime-slice";
import type {
  GitStatusEntry,
  ProcessStatusEntry,
  SessionCommit,
  SessionPollMode,
  SessionPrepareState,
} from "@/lib/state/slices/session-runtime/types";
import type { BackendMessageMap } from "@/lib/types/backend";
import type {
  GitBranchSwitchedEvent,
  GitCommitCreatedEvent,
  GitCommitsResetEvent,
  GitEventPayload,
  GitStatusUpdateEvent,
} from "@/lib/types/git-events";
import { qk } from "../keys";
import type {
  GitStatusQueryData,
  SessionModelsQueryData,
  SessionProcessesQueryData,
} from "../query-options/session-runtime";
import { registerBridgeHandlers, type QueryBridgeRegistration } from "./registrar";

const VALID_POLL_MODES = new Set<SessionPollMode>(["fast", "slow", "paused"]);

export function registerSessionRuntimeBridge(
  ws: Parameters<typeof registerBridgeHandlers>[0],
  queryClient: QueryClient,
): QueryBridgeRegistration {
  return registerBridgeHandlers(ws, queryClient, {
    "session.git.event": (message) => patchGitEvent(queryClient, message.payload),
    "executor.prepare.progress": (message) => patchPrepareProgress(queryClient, message.payload),
    "executor.prepare.completed": (message) => patchPrepareCompleted(queryClient, message.payload),
    "session.state_changed": (message) => patchContextWindowFromState(queryClient, message),
    "session.agentctl_starting": (message) =>
      patchAgentctl(queryClient, message.payload, "starting", message.timestamp),
    "session.agentctl_ready": (message) =>
      patchAgentctl(queryClient, message.payload, "ready", message.timestamp),
    "session.agentctl_error": (message) =>
      patchAgentctl(queryClient, message.payload, "error", message.timestamp),
    "session.available_commands": (message) =>
      queryClient.setQueryData(
        qk.sessionRuntime.availableCommands(message.payload.session_id),
        message.payload.available_commands ?? [],
      ),
    "session.mode_changed": (message) => patchSessionMode(queryClient, message),
    "session.agent_capabilities": (message) => patchAgentCapabilities(queryClient, message),
    "session.models_updated": (message) => patchSessionModels(queryClient, message),
    "session.info_updated": (message) => patchSessionInfo(queryClient, message),
    "session.prompt_usage": (message) => patchPromptUsage(queryClient, message),
    "session.todos_updated": (message) => patchTodos(queryClient, message),
    "session.poll_mode_changed": (message) => patchPollMode(queryClient, message),
    "session.process.status": (message) => patchProcessStatus(queryClient, message),
  });
}

function patchGitEvent(queryClient: QueryClient, payload: GitEventPayload): void {
  if (!payload.session_id || !payload.type) return;
  switch (payload.type) {
    case "status_update":
      patchGitStatus(queryClient, payload);
      return;
    case "commit_created":
      patchCommitCreated(queryClient, payload);
      return;
    case "commits_reset":
    case "branch_switched":
      invalidateCommits(queryClient, payload);
      return;
  }
}

function patchGitStatus(queryClient: QueryClient, event: GitStatusUpdateEvent): void {
  const envKey = resolveEnvKey(queryClient, event.session_id);
  const row: GitStatusEntry = {
    branch: event.status.branch,
    remote_branch: event.status.remote_branch,
    modified: event.status.modified,
    added: event.status.added,
    deleted: event.status.deleted,
    untracked: event.status.untracked,
    renamed: event.status.renamed,
    ahead: event.status.ahead,
    behind: event.status.behind,
    files: event.status.files,
    timestamp: event.timestamp,
    branch_additions: event.status.branch_additions,
    branch_deletions: event.status.branch_deletions,
    repository_name: event.status.repository_name,
  };
  const repoName = row.repository_name ?? "";
  let changed = false;
  queryClient.setQueryData(qk.sessionRuntime.gitStatus(envKey), (current: unknown) => {
    const currentData = readGitStatusData(current);
    const existing = currentData.byRepo[repoName];
    changed = !existing || hasGitStatusChanged(existing, row);
    if (!changed) return currentData;
    return {
      latest: row,
      byRepo: { ...currentData.byRepo, [repoName]: row },
    };
  });
  if (changed) invalidateCumulativeDiffCache(envKey);
}

function patchCommitCreated(queryClient: QueryClient, event: GitCommitCreatedEvent): void {
  const envKey = resolveEnvKey(queryClient, event.session_id);
  const commit: SessionCommit = {
    id: event.commit.id,
    session_id: event.session_id,
    commit_sha: event.commit.commit_sha,
    parent_sha: event.commit.parent_sha,
    commit_message: event.commit.commit_message,
    author_name: event.commit.author_name,
    author_email: event.commit.author_email,
    files_changed: event.commit.files_changed,
    insertions: event.commit.insertions,
    deletions: event.commit.deletions,
    committed_at: event.commit.committed_at,
    created_at: event.commit.created_at ?? event.timestamp,
    repository_name: event.commit.repository_name,
  };
  queryClient.setQueryData(qk.sessionRuntime.commits(envKey), (current: unknown) => {
    const existing = Array.isArray(current) ? (current as SessionCommit[]) : [];
    if (existing.length > 0 && existing[0]?.parent_sha === commit.parent_sha) {
      return [commit, ...existing.slice(1)];
    }
    return [commit, ...existing];
  });
  invalidateCumulativeDiffCache(envKey);
}

function invalidateCommits(
  queryClient: QueryClient,
  event: GitCommitsResetEvent | GitBranchSwitchedEvent,
): void {
  const envKey = resolveEnvKey(queryClient, event.session_id);
  queryClient.invalidateQueries({ exact: true, queryKey: qk.sessionRuntime.commits(envKey) });
  invalidateCumulativeDiffCache(envKey);
}

function patchPrepareProgress(
  queryClient: QueryClient,
  payload: BackendMessageMap["executor.prepare.progress"]["payload"],
): void {
  if (!payload.session_id) return;
  queryClient.setQueryData(qk.sessionRuntime.prepare(payload.session_id), (current: unknown) => {
    const existing = isRecord(current) ? (current as Partial<SessionPrepareState>) : {};
    const steps = [...(Array.isArray(existing.steps) ? existing.steps : [])];
    while (steps.length <= payload.step_index) steps.push({ name: "", status: "pending" });
    steps[payload.step_index] = {
      name: payload.step_name,
      command: payload.step_command,
      status: payload.status,
      output: payload.output,
      error: payload.error,
      warning: payload.warning,
      warningDetail: payload.warning_detail,
      startedAt: payload.started_at,
      endedAt: payload.ended_at,
    };
    return { ...existing, sessionId: payload.session_id, status: "preparing", steps };
  });
}

function patchPrepareCompleted(
  queryClient: QueryClient,
  payload: BackendMessageMap["executor.prepare.completed"]["payload"],
): void {
  if (!payload.session_id) return;
  queryClient.setQueryData(qk.sessionRuntime.prepare(payload.session_id), (current: unknown) => {
    const existing = isRecord(current) ? (current as Partial<SessionPrepareState>) : {};
    const steps = payload.steps?.length
      ? payload.steps.map((step) => ({
          name: step.name,
          command: step.command,
          status: step.status,
          output: step.output,
          error: step.error,
          warning: step.warning,
          warningDetail: step.warning_detail,
          startedAt: step.started_at,
          endedAt: step.ended_at,
        }))
      : existing.steps;
    return {
      ...existing,
      sessionId: payload.session_id,
      status: payload.success ? "completed" : "failed",
      steps: steps ?? [],
      errorMessage: payload.error_message,
      durationMs: payload.duration_ms,
    };
  });
}

function patchContextWindowFromState(
  queryClient: QueryClient,
  message: BackendMessageMap["session.state_changed"],
): void {
  const { session_id: sessionId } = message.payload;
  if (!sessionId) return;
  const metadata = message.payload.metadata ?? message.payload.session_metadata;
  const contextWindow = isRecord(metadata) ? metadata.context_window : null;
  if (!isRecord(contextWindow)) return;
  queryClient.setQueryData(qk.sessionRuntime.contextWindow(sessionId), {
    size: Number(contextWindow.size ?? 0),
    used: Number(contextWindow.used ?? 0),
    remaining: Number(contextWindow.remaining ?? 0),
    efficiency: Number(contextWindow.efficiency ?? 0),
    timestamp:
      typeof contextWindow.timestamp === "string"
        ? contextWindow.timestamp
        : new Date().toISOString(),
  });
}

function patchAgentctl(
  queryClient: QueryClient,
  payload: BackendMessageMap["session.agentctl_ready"]["payload"],
  status: "starting" | "ready" | "error",
  timestamp: string | undefined,
): void {
  if (!payload.session_id) return;
  queryClient.setQueryData(qk.sessionRuntime.agentctl(payload.session_id), {
    status,
    agentExecutionId: payload.agent_execution_id,
    errorMessage: payload.error_message,
    updatedAt: timestamp,
  });
  patchSessionEnvironment(queryClient, payload);
}

function patchSessionEnvironment(
  queryClient: QueryClient,
  payload: BackendMessageMap["session.agentctl_ready"]["payload"],
): void {
  if (!payload.session_id) return;
  queryClient.setQueryData(qk.taskSession.byId(payload.session_id), (current: unknown) => {
    const existing = isRecord(current) ? current : {};
    return {
      ...existing,
      id: payload.session_id,
      task_id: payload.task_id ?? existing.task_id,
      task_environment_id: payload.task_environment_id ?? existing.task_environment_id,
      worktree_id: payload.worktree_id ?? existing.worktree_id,
      worktree_path: payload.task_workspace_path ?? payload.worktree_path ?? existing.worktree_path,
      worktree_branch: payload.worktree_branch ?? existing.worktree_branch,
    };
  });
}

function patchSessionMode(
  queryClient: QueryClient,
  message: BackendMessageMap["session.mode_changed"],
): void {
  const payload = message.payload;
  if (!payload.session_id) return;
  queryClient.setQueryData(qk.sessionRuntime.mode(payload.session_id), {
    currentModeId: payload.current_mode_id || "",
    availableModes: (payload.available_modes ?? []).map((mode) => ({
      id: mode.id,
      name: mode.name,
      description: mode.description,
    })),
  });
}

function patchAgentCapabilities(
  queryClient: QueryClient,
  message: BackendMessageMap["session.agent_capabilities"],
): void {
  const payload = message.payload;
  if (!payload.session_id) return;
  queryClient.setQueryData(qk.sessionRuntime.agentCapabilities(payload.session_id), {
    supportsImage: payload.supports_image,
    supportsAudio: payload.supports_audio,
    supportsEmbeddedContext: payload.supports_embedded_context,
    authMethods: (payload.auth_methods ?? []).map((method) => ({
      id: method.id,
      name: method.name,
      description: method.description,
      terminalAuth: method.terminal_auth
        ? {
            command: method.terminal_auth.command,
            args: method.terminal_auth.args,
            label: method.terminal_auth.label,
          }
        : undefined,
      meta: method.meta,
    })),
  });
}

function patchSessionModels(
  queryClient: QueryClient,
  message: BackendMessageMap["session.models_updated"],
): void {
  const payload = message.payload;
  if (!payload.session_id) return;
  const currentModelId = resolveCurrentModelId(payload);
  let previousModelId = "";
  const next: SessionModelsQueryData = {
    currentModelId,
    models: (payload.models ?? []).map((model) => ({
      modelId: model.model_id,
      name: model.name,
      description: model.description,
      usageMultiplier: model.usage_multiplier,
      meta: model.meta,
    })),
    configOptions: (payload.config_options ?? []).map((option) => ({
      type: option.type,
      id: option.id,
      name: option.name,
      currentValue: option.current_value,
      category: option.category,
      options: option.options,
    })),
  };
  queryClient.setQueryData(qk.sessionRuntime.models(payload.session_id), (current: unknown) => {
    previousModelId = isRecord(current) ? String(current.currentModelId ?? "") : "";
    return next;
  });
  if (previousModelId && currentModelId && previousModelId !== currentModelId) {
    queryClient.setQueryData(qk.sessionRuntime.contextWindow(payload.session_id), null);
  }
}

function patchSessionInfo(
  queryClient: QueryClient,
  message: BackendMessageMap["session.info_updated"],
): void {
  const payload = message.payload;
  if (!payload.session_id) return;
  queryClient.setQueryData(qk.taskSession.byId(payload.session_id), (current: unknown) => {
    if (!isRecord(current)) return current;
    const currentMetadata = isRecord(current.metadata) ? current.metadata : {};
    const existingAcp = readExistingAcp(currentMetadata.acp);
    if (isStale(payload.session_updated_at, existingAcp.updated_at)) return current;
    return {
      ...current,
      metadata: {
        ...currentMetadata,
        acp: {
          session_id: payload.acp_session_id || existingAcp.session_id,
          title: payload.session_title || existingAcp.title,
          updated_at: payload.session_updated_at || existingAcp.updated_at,
          meta: payload.session_meta ?? existingAcp.meta,
        },
      },
    };
  });
}

function patchPromptUsage(
  queryClient: QueryClient,
  message: BackendMessageMap["session.prompt_usage"],
): void {
  const payload = message.payload;
  if (!payload.session_id || !payload.usage) return;
  queryClient.setQueryData(qk.sessionRuntime.promptUsage(payload.session_id), {
    inputTokens: payload.usage.input_tokens,
    outputTokens: payload.usage.output_tokens,
    cachedReadTokens: payload.usage.cached_read_tokens,
    cachedWriteTokens: payload.usage.cached_write_tokens,
    totalTokens: payload.usage.total_tokens,
  });
}

function patchTodos(queryClient: QueryClient, message: BackendMessageMap["session.todos_updated"]) {
  const payload = message.payload;
  if (!payload.session_id) return;
  queryClient.setQueryData(
    qk.sessionRuntime.todos(payload.session_id),
    (payload.entries ?? []).map((entry) => ({
      description: entry.description,
      status: entry.status,
      priority: entry.priority,
    })),
  );
}

function patchPollMode(
  queryClient: QueryClient,
  message: BackendMessageMap["session.poll_mode_changed"],
): void {
  const { session_id: sessionId, poll_mode: pollMode } = message.payload;
  if (!sessionId || !VALID_POLL_MODES.has(pollMode as SessionPollMode)) return;
  queryClient.setQueryData(qk.sessionRuntime.pollMode(sessionId), pollMode as SessionPollMode);
}

function patchProcessStatus(
  queryClient: QueryClient,
  message: BackendMessageMap["session.process.status"],
): void {
  const payload = message.payload;
  if (!payload.session_id || !payload.process_id || !payload.status) return;
  const row: ProcessStatusEntry = {
    processId: payload.process_id,
    sessionId: payload.session_id,
    kind: payload.kind,
    scriptName: payload.script_name,
    status: payload.status,
    command: payload.command,
    workingDir: payload.working_dir,
    exitCode: payload.exit_code,
    updatedAt: payload.timestamp,
  };
  queryClient.setQueryData(qk.sessionRuntime.processes(payload.session_id), (current: unknown) => {
    const existing = readProcessesData(current);
    return {
      ...existing,
      processesById: { ...existing.processesById, [row.processId]: row },
      processIds: existing.processIds.includes(row.processId)
        ? existing.processIds
        : [...existing.processIds, row.processId],
      devProcessId: row.kind === "dev" ? row.processId : existing.devProcessId,
    };
  });
}

function resolveCurrentModelId(payload: BackendMessageMap["session.models_updated"]["payload"]) {
  if (payload.current_model_id) return payload.current_model_id;
  const modelOption = (payload.config_options ?? []).find(
    (option) => option.id === "model" || option.category === "model",
  );
  return modelOption?.current_value ?? "";
}

function resolveEnvKey(queryClient: QueryClient, sessionId: string): string {
  const session = queryClient.getQueryData(qk.taskSession.byId(sessionId));
  if (isRecord(session) && typeof session.task_environment_id === "string") {
    return session.task_environment_id;
  }
  return sessionId;
}

function readGitStatusData(current: unknown): GitStatusQueryData {
  if (!isRecord(current)) return { byRepo: {} };
  return {
    latest: isRecord(current.latest) ? (current.latest as GitStatusEntry) : undefined,
    byRepo: isRecord(current.byRepo) ? (current.byRepo as Record<string, GitStatusEntry>) : {},
  };
}

function readProcessesData(current: unknown): SessionProcessesQueryData {
  if (!isRecord(current)) return { processesById: {}, processIds: [] };
  return {
    processesById: isRecord(current.processesById)
      ? (current.processesById as Record<string, ProcessStatusEntry>)
      : {},
    processIds: Array.isArray(current.processIds) ? (current.processIds as string[]) : [],
    activeProcessId:
      typeof current.activeProcessId === "string" ? current.activeProcessId : undefined,
    devProcessId: typeof current.devProcessId === "string" ? current.devProcessId : undefined,
  };
}

function isStale(incomingUpdatedAt: string | undefined, existingUpdatedAt: string): boolean {
  if (!incomingUpdatedAt || !existingUpdatedAt) return false;
  const incoming = Date.parse(incomingUpdatedAt);
  const existing = Date.parse(existingUpdatedAt);
  if (Number.isNaN(incoming) || Number.isNaN(existing)) return false;
  return incoming < existing;
}

function readExistingAcp(value: unknown) {
  if (!isRecord(value)) return { session_id: "", title: "", updated_at: "", meta: {} };
  return {
    session_id: typeof value.session_id === "string" ? value.session_id : "",
    title: typeof value.title === "string" ? value.title : "",
    updated_at: typeof value.updated_at === "string" ? value.updated_at : "",
    meta: isRecord(value.meta) ? value.meta : {},
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
