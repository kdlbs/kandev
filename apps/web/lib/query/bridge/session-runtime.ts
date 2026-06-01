/**
 * Session-runtime data → TanStack Query bridge (Wave 5b).
 *
 * This bridge is now the SOLE writer for the migrated session-runtime server
 * fields (git status, session mode, session models, session todos, poll mode):
 * their Zustand slice fields + handlers were removed in the D6 migration. The
 * remaining (not-yet-migrated) fields are still mirrored from their Zustand
 * handlers:
 *   session.git.event        → git status (TQ-only) + commits (commits still in Zustand)
 *   session.mode_changed     → mode (TQ-only)
 *   session.models_updated   → models (TQ-only; handlers/session-models.ts kept for
 *                              the client-only activeModel-clearing side effect)
 *   session.poll_mode_changed→ poll mode (TQ-only)
 *   session.todos_updated    → todos (TQ-only)
 *   session.prompt_usage     → prompt_usage (also lib/ws/handlers/prompt-usage.ts)
 *   session.available_commands → available_commands (also available-commands.ts)
 *   session.agent_capabilities → agent_capabilities (also agent-capabilities.ts)
 *   executor.prepare.*       → prepare progress/completed (also executor-prepare.ts)
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
  AvailableCommand,
  FileInfo,
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
import { wrapBridgeHandler } from "./index";

// ---------------------------------------------------------------------------
// environmentIdBySessionId resolver type
// ---------------------------------------------------------------------------

/** Resolves sessionId → environmentId for cache key routing. */
export type EnvKeyResolver = (sessionId: string) => string;

// ---------------------------------------------------------------------------
// Git status helpers (ported from session-runtime-slice on main, commit
// d76836d7). The Zustand gitStatus slice field was deleted in the TQ
// migration, so these pure comparators are kept locally here.
//
// Key contract: the backend re-emits identical git data with a FRESH timestamp
// on focus/startup/poll, so the timestamp must NOT force a store update or a
// diff-cache invalidation. Comparison is CONTENT-only (branch summary + file
// lists + file stats + per-file deep equality).
// ---------------------------------------------------------------------------

/** Compute total additions/deletions across all files. */
function computeFileStats(files: Record<string, FileInfo> | undefined): {
  additions: number;
  deletions: number;
} {
  if (!files) return { additions: 0, deletions: 0 };
  let additions = 0;
  let deletions = 0;
  for (const f of Object.values(files)) {
    additions += f.additions || 0;
    deletions += f.deletions || 0;
  }
  return { additions, deletions };
}

function sameStringList(existing: string[] | undefined, incoming: string[] | undefined): boolean {
  const a = existing ?? [];
  const b = incoming ?? [];
  if (a.length !== b.length) return false;
  const sortedA = [...a].sort();
  const sortedB = [...b].sort();
  return sortedA.every((value, index) => value === sortedB[index]);
}

const COMPARABLE_FILE_FIELDS = [
  "path",
  "status",
  "staged",
  "additions",
  "deletions",
  "old_path",
  "diff",
  "diff_skip_reason",
  "repository_name",
] as const;

function comparableFileInfo(file: FileInfo) {
  return {
    path: file.path,
    status: file.status,
    staged: file.staged,
    additions: file.additions ?? 0,
    deletions: file.deletions ?? 0,
    old_path: file.old_path ?? "",
    diff: file.diff ?? "",
    diff_skip_reason: file.diff_skip_reason ?? "",
    repository_name: file.repository_name ?? "",
  };
}

function sameFileInfo(existing: FileInfo | undefined, incoming: FileInfo | undefined): boolean {
  if (!existing || !incoming) return existing === incoming;
  const a = comparableFileInfo(existing);
  const b = comparableFileInfo(incoming);
  return COMPARABLE_FILE_FIELDS.every((field) => a[field] === b[field]);
}

function sameFiles(
  existingFiles: Record<string, FileInfo> | undefined,
  newFiles: Record<string, FileInfo> | undefined,
): boolean {
  if (!existingFiles || !newFiles) return existingFiles === newFiles;
  const existingFileKeys = Object.keys(existingFiles).sort();
  const newFileKeys = Object.keys(newFiles).sort();
  if (existingFileKeys.length !== newFileKeys.length) return false;
  for (let i = 0; i < existingFileKeys.length; i += 1) {
    const key = existingFileKeys[i];
    if (key !== newFileKeys[i]) return false;
    if (!sameFileInfo(existingFiles[key], newFiles[key])) return false;
  }
  return true;
}

function hasBranchSummaryChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  return (
    existing.branch !== incoming.branch ||
    existing.remote_branch !== incoming.remote_branch ||
    existing.ahead !== incoming.ahead ||
    existing.behind !== incoming.behind ||
    (existing.repository_name ?? "") !== (incoming.repository_name ?? "") ||
    existing.branch_additions !== incoming.branch_additions ||
    existing.branch_deletions !== incoming.branch_deletions
  );
}

function hasFileListsChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  return (
    !sameStringList(existing.modified, incoming.modified) ||
    !sameStringList(existing.added, incoming.added) ||
    !sameStringList(existing.deleted, incoming.deleted) ||
    !sameStringList(existing.untracked, incoming.untracked) ||
    !sameStringList(existing.renamed, incoming.renamed)
  );
}

function hasFileStatsChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  // Fast early-exit: aggregate totals differ → sameFiles would also return false,
  // but this avoids the per-file deep comparison when the gross numbers differ.
  const existingTotal = computeFileStats(existing.files);
  const newTotal = computeFileStats(incoming.files);
  return (
    existingTotal.additions !== newTotal.additions || existingTotal.deletions !== newTotal.deletions
  );
}

/** Content-only comparison: timestamp is deliberately ignored (see header). */
function hasGitStatusChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  return (
    hasBranchSummaryChanged(existing, incoming) ||
    hasFileListsChanged(existing, incoming) ||
    hasFileStatsChanged(existing, incoming) ||
    !sameFiles(existing.files, incoming.files)
  );
}

/**
 * Apply an incoming status to the cache, reporting whether it mutated.
 *
 * Mirrors main's `setGitStatus` + `gitStatusWouldMutate` (session-runtime-slice,
 * commit d76836d7). Multi-repo routing: when the update is tagged with a
 * `repository_name`, the per-repo slot is authoritative — `changed` reflects
 * only THAT repo's slot, and the env-level (`byEnvironmentId`) mirror is updated
 * only when the repo slot changed. This prevents sibling-repo events from
 * ping-ponging the env-level slot and spuriously reporting a change.
 *
 * `changed` is true only when the status CONTENT actually differs from what's
 * cached — the caller uses it to gate the cumulative-diff cache invalidation so
 * that backend re-emits (identical data, fresh timestamp) don't churn the diff
 * panel.
 */
function updateGitStatusData(
  prev: GitStatusData | undefined,
  incoming: GitStatusEntry,
): { data: GitStatusData; changed: boolean } {
  const repoName = incoming.repository_name ?? "";
  const existingEnv = prev?.byEnvironmentId;
  const existingRepo = prev?.byEnvironmentRepo[repoName];

  const repoChanged = !existingRepo || hasGitStatusChanged(existingRepo, incoming);
  const envChanged = !existingEnv || hasGitStatusChanged(existingEnv, incoming);

  // gitStatusWouldMutate semantics: per-repo updates are gated on the repo
  // slot alone; env-level (unnamed) updates consider both slots.
  const changed = repoName !== "" ? repoChanged : envChanged || repoChanged;

  if (!changed) {
    return {
      data: prev ?? { byEnvironmentId: incoming, byEnvironmentRepo: { [repoName]: incoming } },
      changed: false,
    };
  }

  const prevRepos = prev ? prev.byEnvironmentRepo : {};
  const prevById = prev ? prev.byEnvironmentId : undefined;

  // byEnvironmentId mirror: written when the unnamed status changed, OR when a
  // named repo's slot changed (so single-status consumers see the latest repo).
  const shouldWriteEnv = repoName === "" ? envChanged : repoChanged;

  return {
    data: {
      byEnvironmentId: shouldWriteEnv ? incoming : prevById,
      byEnvironmentRepo: repoChanged ? { ...prevRepos, [repoName]: incoming } : prevRepos,
    },
    changed: true,
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
  const unsubGit = ws.on(
    "session.git.event",
    wrapBridgeHandler(qc, "session.git.event", (message) => {
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
          // Gate the diff-cache invalidation on a genuine content change. The
          // backend re-emits identical git data with a fresh timestamp on
          // focus/startup/poll; those must NOT churn the cumulative-diff cache.
          let statusChanged = false;
          qc.setQueryData<GitStatusData>(qk.session.git(envKey), (prev) => {
            const result = updateGitStatusData(prev, incoming);
            statusChanged = result.changed;
            return result.data;
          });
          if (statusChanged) invalidateCumulativeDiffCache(envKey);
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
    }),
  );

  return [unsubGit];
}

// ---------------------------------------------------------------------------
// Session data handlers sub-registrar (mode, models, todos, usage, capabilities)
// ---------------------------------------------------------------------------

function registerModeAndModelsHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  const unsubMode = ws.on(
    "session.mode_changed",
    wrapBridgeHandler(qc, "session.mode_changed", (message) => {
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
    }),
  );

  const unsubModels = ws.on(
    "session.models_updated",
    wrapBridgeHandler(qc, "session.models_updated", (message) => {
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
    }),
  );

  return [unsubMode, unsubModels];
}

function registerTodosAndUsageHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  const unsubTodos = ws.on(
    "session.todos_updated",
    wrapBridgeHandler(qc, "session.todos_updated", (message) => {
      const payload = message.payload as SessionTodosPayload | undefined;
      if (!payload?.session_id) return;
      const entries: TodoEntry[] = (payload.entries ?? []).map((e) => ({
        description: e.description,
        status: e.status as TodoEntry["status"],
        priority: e.priority,
      }));
      qc.setQueryData<TodoEntry[]>(qk.session.todos(payload.session_id), () => entries);
    }),
  );

  const unsubPromptUsage = ws.on(
    "session.prompt_usage",
    wrapBridgeHandler(qc, "session.prompt_usage", (message) => {
      const payload = message.payload as SessionPromptUsagePayload | undefined;
      if (!payload?.session_id || !payload.usage) return;
      qc.setQueryData(qk.session.promptUsage(payload.session_id), () => ({
        inputTokens: payload.usage.input_tokens,
        outputTokens: payload.usage.output_tokens,
        cachedReadTokens: payload.usage.cached_read_tokens,
        cachedWriteTokens: payload.usage.cached_write_tokens,
        totalTokens: payload.usage.total_tokens,
      }));
    }),
  );

  const unsubCapabilities = ws.on(
    "session.agent_capabilities",
    wrapBridgeHandler(qc, "session.agent_capabilities", (message) => {
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
    }),
  );

  const unsubPollMode = ws.on(
    "session.poll_mode_changed",
    wrapBridgeHandler(qc, "session.poll_mode_changed", (message) => {
      const { session_id, poll_mode } = message.payload;
      if (!session_id || !poll_mode) return;
      const VALID = new Set(["fast", "slow", "paused"]);
      if (!VALID.has(poll_mode as string)) return;
      qc.setQueryData(qk.session.pollMode(session_id as string), () => poll_mode);
    }),
  );

  const unsubAvailableCommands = ws.on(
    "session.available_commands",
    wrapBridgeHandler(qc, "session.available_commands", (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      const sessionId = payload.session_id as string;
      const commands = (payload.available_commands || []) as AvailableCommand[];
      qc.setQueryData<AvailableCommand[]>(qk.session.availableCommands(sessionId), () => commands);
    }),
  );

  return [unsubTodos, unsubPromptUsage, unsubCapabilities, unsubPollMode, unsubAvailableCommands];
}

function registerSessionDataHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  return [...registerModeAndModelsHandlers(ws, qc), ...registerTodosAndUsageHandlers(ws, qc)];
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

  const unsubProgress = ws.on(
    "executor.prepare.progress",
    wrapBridgeHandler(qc, "executor.prepare.progress", (message) => {
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
    }),
  );

  const unsubCompleted = ws.on(
    "executor.prepare.completed",
    wrapBridgeHandler(qc, "executor.prepare.completed", (message) => {
      const payload = message.payload as PrepareCompletedPayload;
      if (!payload.session_id) return;
      qc.setQueryData<PrepareState | null>(
        qk.session.prepareProgress(payload.session_id),
        (prev) => {
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
        },
      );
    }),
  );

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
