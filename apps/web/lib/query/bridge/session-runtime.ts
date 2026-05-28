/**
 * Session-runtime data → TanStack Query bridge (Wave 5b).
 *
 * Mirrors the following WS handlers (1:1 event coverage):
 *   lib/ws/handlers/git-status.ts         → git status + commits
 *   lib/ws/handlers/session-mode.ts       → mode_changed
 *   lib/ws/handlers/session-models.ts     → models_updated
 *   lib/ws/handlers/session-poll-mode.ts  → poll_mode_changed
 *   lib/ws/handlers/session-todos.ts      → todos_updated
 *   lib/ws/handlers/prompt-usage.ts       → prompt_usage
 *   lib/ws/handlers/available-commands.ts → available_commands
 *   lib/ws/handlers/agent-capabilities.ts → agent_capabilities
 *   lib/ws/handlers/executor-prepare.ts   → prepare.progress + prepare.completed
 *
 * IMPORTANT: `environmentIdBySessionId` stays in Zustand (client-side index,
 * not server state). The bridge reads it via the `getEnvKey` resolver passed
 * in by the registrar caller.
 *
 * Sub-registrar breakdown:
 *   registerGitHandlers          — git status + commits events (~50 LOC)
 *   registerSessionDataHandlers  — mode, models, todos, prompts, capabilities (~60 LOC)
 *   registerPrepareHandlers      — executor prepare progress + completed (~40 LOC)
 *   registerSessionRuntimeBridge — top-level aggregator
 */

import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import type { GitEventPayload } from "@/lib/types/git-events";
import type {
  SessionModeChangedPayload,
  SessionModelsPayload,
  AgentCapabilitiesPayload,
  PrepareProgressPayload,
  PrepareCompletedPayload,
} from "@/lib/types/backend";
import type {
  SessionTodosPayload,
  SessionPromptUsagePayload,
} from "@/lib/types/session-runtime-payloads";
import type {
  GitStatusEntry,
  SessionCommit,
  TodoEntry,
} from "@/lib/state/slices/session-runtime/types";
import { qk } from "@/lib/query/keys";
import type {
  GitStatusData,
  SessionModeData,
  SessionModelsData,
} from "@/lib/query/query-options/session-runtime";
import { invalidateCumulativeDiffCache } from "@/hooks/domains/session/use-cumulative-diff";

// ---------------------------------------------------------------------------
// environmentIdBySessionId resolver type
// ---------------------------------------------------------------------------

/** Resolves sessionId → environmentId for cache key routing. */
export type EnvKeyResolver = (sessionId: string) => string;

// ---------------------------------------------------------------------------
// Git status helpers (mirrors session-runtime-slice hasGitStatusChanged)
// ---------------------------------------------------------------------------

function hasGitStatusChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  if (existing.timestamp !== incoming.timestamp) return true;
  if (existing.branch !== incoming.branch || existing.remote_branch !== incoming.remote_branch)
    return true;
  if (existing.ahead !== incoming.ahead || existing.behind !== incoming.behind) return true;
  const existingFileKeys = existing.files ? Object.keys(existing.files).sort().join(",") : "";
  const newFileKeys = incoming.files ? Object.keys(incoming.files).sort().join(",") : "";
  if (existingFileKeys !== newFileKeys) return true;
  return false;
}

function updateGitStatusData(
  prev: GitStatusData | undefined,
  incoming: GitStatusEntry,
): GitStatusData {
  const repoName = incoming.repository_name ?? "";
  const existing = prev?.byEnvironmentId;
  const existingRepo = prev?.byEnvironmentRepo[repoName];

  const shouldUpdate = !existing || hasGitStatusChanged(existing, incoming);
  const shouldUpdateRepo = !existingRepo || hasGitStatusChanged(existingRepo, incoming);

  if (!shouldUpdate && !shouldUpdateRepo) {
    return prev ?? { byEnvironmentId: incoming, byEnvironmentRepo: { [repoName]: incoming } };
  }

  const prevRepos = prev ? prev.byEnvironmentRepo : {};
  const prevById = prev ? prev.byEnvironmentId : incoming;

  return {
    byEnvironmentId: shouldUpdate ? incoming : prevById,
    byEnvironmentRepo: shouldUpdateRepo ? { ...prevRepos, [repoName]: incoming } : prevRepos,
  };
}

// ---------------------------------------------------------------------------
// Git handlers sub-registrar
// ---------------------------------------------------------------------------

function registerGitHandlers(
  ws: WebSocketClient,
  qc: QueryClient,
  getEnvKey: EnvKeyResolver,
): Array<() => void> {
  const unsubGit = ws.on("session.git.event", (message) => {
    const payload = message.payload as GitEventPayload;
    if (!payload.session_id || !payload.type) return;

    const sid = payload.session_id;
    const envKey = getEnvKey(sid);

    switch (payload.type) {
      case "status_update": {
        const incoming: GitStatusEntry = {
          branch: payload.status.branch,
          remote_branch: payload.status.remote_branch,
          modified: payload.status.modified,
          added: payload.status.added,
          deleted: payload.status.deleted,
          untracked: payload.status.untracked,
          renamed: payload.status.renamed,
          ahead: payload.status.ahead,
          behind: payload.status.behind,
          files: payload.status.files,
          timestamp: message.timestamp ?? null,
          branch_additions: payload.status.branch_additions,
          branch_deletions: payload.status.branch_deletions,
          repository_name: payload.status.repository_name,
        };
        qc.setQueryData<GitStatusData>(qk.session.git(envKey), (prev) =>
          updateGitStatusData(prev, incoming),
        );
        invalidateCumulativeDiffCache(envKey);
        break;
      }

      case "commit_created": {
        const commit: SessionCommit = {
          id: payload.commit.id,
          session_id: sid,
          commit_sha: payload.commit.commit_sha,
          parent_sha: payload.commit.parent_sha,
          commit_message: payload.commit.commit_message,
          author_name: payload.commit.author_name,
          author_email: payload.commit.author_email,
          files_changed: payload.commit.files_changed,
          insertions: payload.commit.insertions,
          deletions: payload.commit.deletions,
          committed_at: payload.commit.committed_at,
          created_at: payload.commit.created_at ?? message.timestamp ?? "",
          repository_name: payload.commit.repository_name,
        };
        qc.setQueryData<SessionCommit[]>(qk.session.commits(envKey), (prev) => {
          const existing = prev ?? [];
          // For amend: replace HEAD (index 0) when parent SHA matches
          if (existing.length > 0 && existing[0].parent_sha === commit.parent_sha) {
            return [commit, ...existing.slice(1)];
          }
          return [commit, ...existing];
        });
        invalidateCumulativeDiffCache(envKey);
        break;
      }

      case "commits_reset": {
        // Clear commits → triggers refetch in consumers
        qc.removeQueries({ queryKey: qk.session.commits(envKey) });
        invalidateCumulativeDiffCache(envKey);
        break;
      }

      case "branch_switched": {
        // Clear commits → triggers refetch with new base
        qc.removeQueries({ queryKey: qk.session.commits(envKey) });
        invalidateCumulativeDiffCache(envKey);
        break;
      }
    }
  });

  return [unsubGit];
}

// ---------------------------------------------------------------------------
// Session data handlers sub-registrar (mode, models, todos, usage, capabilities)
// ---------------------------------------------------------------------------

function registerSessionDataHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  const unsubMode = ws.on("session.mode_changed", (message) => {
    const payload = message.payload as SessionModeChangedPayload | undefined;
    if (!payload?.session_id) return;
    const modeId = payload.current_mode_id || "";
    const availableModes = (payload.available_modes ?? []).map((m) => ({
      id: m.id,
      name: m.name,
      description: m.description,
    }));
    qc.setQueryData<SessionModeData | null>(qk.session.mode(payload.session_id), (prev) => ({
      currentModeId: modeId,
      availableModes: availableModes.length > 0 ? availableModes : (prev?.availableModes ?? []),
    }));
  });

  const unsubModels = ws.on("session.models_updated", (message) => {
    const payload = message.payload as SessionModelsPayload | undefined;
    if (!payload?.session_id) return;
    const acpModels = payload.models ?? [];
    let currentModelId = payload.current_model_id || "";
    if (!currentModelId) {
      const modelOpt = (payload.config_options ?? []).find(
        (o) => o.id === "model" || o.category === "model",
      );
      if (modelOpt?.current_value) currentModelId = modelOpt.current_value;
    }
    qc.setQueryData<SessionModelsData | null>(qk.session.models(payload.session_id), () => ({
      currentModelId,
      models: acpModels.map((m) => ({
        modelId: m.model_id,
        name: m.name,
        description: m.description,
        usageMultiplier: m.usage_multiplier,
        meta: m.meta,
      })),
      configOptions: (payload.config_options ?? []).map((o) => ({
        type: o.type,
        id: o.id,
        name: o.name,
        currentValue: o.current_value,
        category: o.category,
        options: o.options,
      })),
    }));
  });

  const unsubTodos = ws.on("session.todos_updated", (message) => {
    const payload = message.payload as SessionTodosPayload | undefined;
    if (!payload?.session_id) return;
    const entries: TodoEntry[] = (payload.entries ?? []).map((e) => ({
      description: e.description,
      status: e.status as TodoEntry["status"],
      priority: e.priority,
    }));
    qc.setQueryData<TodoEntry[]>(qk.session.todos(payload.session_id), () => entries);
  });

  const unsubPromptUsage = ws.on("session.prompt_usage", (message) => {
    const payload = message.payload as SessionPromptUsagePayload | undefined;
    if (!payload?.session_id || !payload.usage) return;
    qc.setQueryData(qk.session.promptUsage(payload.session_id), () => ({
      inputTokens: payload.usage.input_tokens,
      outputTokens: payload.usage.output_tokens,
      cachedReadTokens: payload.usage.cached_read_tokens,
      cachedWriteTokens: payload.usage.cached_write_tokens,
      totalTokens: payload.usage.total_tokens,
    }));
  });

  const unsubCapabilities = ws.on("session.agent_capabilities", (message) => {
    const payload = message.payload as AgentCapabilitiesPayload | undefined;
    if (!payload?.session_id) return;
    qc.setQueryData(qk.session.agentCapabilities(payload.session_id), () => ({
      supportsImage: payload.supports_image,
      supportsAudio: payload.supports_audio,
      supportsEmbeddedContext: payload.supports_embedded_context,
      authMethods: (payload.auth_methods ?? []).map((m) => ({
        id: m.id,
        name: m.name,
        description: m.description,
        terminalAuth: m.terminal_auth
          ? {
              command: m.terminal_auth.command,
              args: m.terminal_auth.args,
              label: m.terminal_auth.label,
            }
          : undefined,
        meta: m.meta,
      })),
    }));
  });

  const unsubPollMode = ws.on("session.poll_mode_changed", (message) => {
    const { session_id, poll_mode } = message.payload;
    if (!session_id || !poll_mode) return;
    const VALID = new Set(["fast", "slow", "paused"]);
    if (!VALID.has(poll_mode as string)) return;
    qc.setQueryData(qk.session.pollMode(session_id as string), () => poll_mode);
  });

  return [unsubMode, unsubModels, unsubTodos, unsubPromptUsage, unsubCapabilities, unsubPollMode];
}

// ---------------------------------------------------------------------------
// Prepare progress handlers sub-registrar
// ---------------------------------------------------------------------------

type PrepareStep = NonNullable<
  import("@/lib/state/slices/session-runtime/types").PrepareProgressState["bySessionId"][string]
>["steps"][number];

function updatePrepareSteps(
  existing: PrepareStep[],
  payload: PrepareProgressPayload,
): PrepareStep[] {
  const steps = [...existing];
  while (steps.length <= payload.step_index) {
    steps.push({ name: "", status: "pending" });
  }
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
  return steps;
}

function registerPrepareHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  type PrepareState = NonNullable<
    import("@/lib/state/slices/session-runtime/types").PrepareProgressState["bySessionId"][string]
  >;

  const unsubProgress = ws.on("executor.prepare.progress", (message) => {
    const payload = message.payload as PrepareProgressPayload;
    if (!payload.session_id) return;
    qc.setQueryData<PrepareState | null>(
      qk.session.prepareProgress(payload.session_id),
      (prev) => ({
        sessionId: payload.session_id,
        status: "preparing",
        steps: updatePrepareSteps(prev?.steps ?? [], payload),
      }),
    );
  });

  const unsubCompleted = ws.on("executor.prepare.completed", (message) => {
    const payload = message.payload as PrepareCompletedPayload;
    if (!payload.session_id) return;
    qc.setQueryData<PrepareState | null>(qk.session.prepareProgress(payload.session_id), (prev) => {
      const steps = payload.steps?.length
        ? payload.steps.map((s) => ({
            name: s.name,
            command: s.command,
            status: s.status,
            output: s.output,
            error: s.error,
            warning: s.warning,
            warningDetail: s.warning_detail,
            startedAt: s.started_at,
            endedAt: s.ended_at,
          }))
        : (prev?.steps ?? []);
      return {
        sessionId: payload.session_id,
        status: payload.success ? "completed" : "failed",
        steps,
        errorMessage: payload.error_message,
        durationMs: payload.duration_ms,
      };
    });
  });

  return [unsubProgress, unsubCompleted];
}

// ---------------------------------------------------------------------------
// Top-level bridge registrar
// ---------------------------------------------------------------------------

/**
 * Registers WS handlers for session-runtime data events.
 *
 * `getEnvKey` — resolves sessionId → environmentId. Reads from the Zustand
 * `environmentIdBySessionId` map, which stays in Zustand because it's a
 * client-side index (not server state).
 *
 * Returns a cleanup function.
 */
export function registerSessionRuntimeBridge(
  ws: WebSocketClient,
  qc: QueryClient,
  getEnvKey: EnvKeyResolver,
): () => void {
  const all = [
    ...registerGitHandlers(ws, qc, getEnvKey),
    ...registerSessionDataHandlers(ws, qc),
    ...registerPrepareHandlers(ws, qc),
  ];
  return () => {
    for (const fn of all) fn();
  };
}
