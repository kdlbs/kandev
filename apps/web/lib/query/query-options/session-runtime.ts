/**
 * TanStack Query options for the session-runtime domain (Wave 5b).
 *
 * These factories are consumed by useQuery hooks in
 * hooks/domains/session/* and by the session-runtime bridge.
 *
 * Key conventions:
 *   - git / commits keys use envKey = environmentIdBySessionId[sid] ?? sid.
 *     The environmentIdBySessionId map stays in Zustand (client-side index).
 *   - All other keys use the session ID directly.
 *   - staleTime: 5 * 60_000 for session queries (active-session protection).
 */

import { queryOptions } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type {
  ContextWindowEntry,
  AvailableCommand,
  AgentCapabilitiesEntry,
  PromptUsageEntry,
  TodoEntry,
  SessionPollMode,
  PrepareProgressState,
  SessionCommit,
  SessionModeEntry,
  SessionModelEntry,
  ConfigOptionEntry,
} from "@/lib/state/slices/session-runtime/types";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

// ---------------------------------------------------------------------------
// Cache data shapes (stored in TQ, consumed by hooks)
// ---------------------------------------------------------------------------

export type GitStatusData = {
  /** Primary git status (most recently received, or undefined before first event). */
  byEnvironmentId: GitStatusEntry | undefined;
  /** Per-repository statuses for multi-repo workspaces (keyed by repo name). */
  byEnvironmentRepo: Record<string, GitStatusEntry>;
};

export type SessionModeData = {
  currentModeId: string;
  availableModes: SessionModeEntry[];
};

export type SessionModelsData = {
  currentModelId: string;
  models: SessionModelEntry[];
  configOptions: ConfigOptionEntry[];
};

// ---------------------------------------------------------------------------
// Shared config
// ---------------------------------------------------------------------------

/** staleTime applied to all session-scoped queries (active-session protection). */
const SESSION_STALE_TIME = 5 * 60_000;

// ---------------------------------------------------------------------------
// Git status (env-keyed)
// ---------------------------------------------------------------------------

export const gitStatusQueryOptions = (envKey: string) =>
  queryOptions<GitStatusData>({
    queryKey: qk.session.git(envKey),
    queryFn: () => ({ byEnvironmentId: undefined, byEnvironmentRepo: {} }),
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });

// ---------------------------------------------------------------------------
// Git commits (env-keyed)
// ---------------------------------------------------------------------------

export const gitCommitsQueryOptions = (envKey: string) =>
  queryOptions<SessionCommit[]>({
    queryKey: qk.session.commits(envKey),
    queryFn: () => [],
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });

// ---------------------------------------------------------------------------
// Context window (session-keyed)
// ---------------------------------------------------------------------------

export const contextWindowQueryOptions = (sessionId: string) =>
  queryOptions<ContextWindowEntry | null>({
    queryKey: qk.session.context(sessionId),
    queryFn: () => null,
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Session mode (session-keyed)
// ---------------------------------------------------------------------------

export const sessionModeQueryOptions = (sessionId: string) =>
  queryOptions<SessionModeData | null>({
    queryKey: qk.session.mode(sessionId),
    queryFn: () => null,
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Session models (session-keyed)
// ---------------------------------------------------------------------------

export const sessionModelsQueryOptions = (sessionId: string) =>
  queryOptions<SessionModelsData | null>({
    queryKey: qk.session.models(sessionId),
    queryFn: () => null,
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Agent capabilities (session-keyed)
// ---------------------------------------------------------------------------

export const agentCapabilitiesQueryOptions = (sessionId: string) =>
  queryOptions<AgentCapabilitiesEntry | null>({
    queryKey: qk.session.agentCapabilities(sessionId),
    queryFn: () => null,
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Prompt usage (session-keyed)
// ---------------------------------------------------------------------------

export const promptUsageQueryOptions = (sessionId: string) =>
  queryOptions<PromptUsageEntry | null>({
    queryKey: qk.session.promptUsage(sessionId),
    queryFn: () => null,
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Session todos (session-keyed)
// ---------------------------------------------------------------------------

export const sessionTodosQueryOptions = (sessionId: string) =>
  queryOptions<TodoEntry[]>({
    queryKey: qk.session.todos(sessionId),
    queryFn: () => [],
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Available commands (session-keyed)
// ---------------------------------------------------------------------------

export const availableCommandsQueryOptions = (sessionId: string) =>
  queryOptions<AvailableCommand[]>({
    queryKey: qk.session.availableCommands(sessionId),
    queryFn: () => [],
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Session poll mode (session-keyed)
// ---------------------------------------------------------------------------

export const sessionPollModeQueryOptions = (sessionId: string) =>
  queryOptions<SessionPollMode | null>({
    queryKey: qk.session.pollMode(sessionId),
    queryFn: () => null,
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Prepare progress (session-keyed)
// ---------------------------------------------------------------------------

export const prepareProgressQueryOptions = (sessionId: string) =>
  queryOptions<PrepareProgressState["bySessionId"][string] | null>({
    queryKey: qk.session.prepareProgress(sessionId),
    queryFn: () => null,
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Namespace export (mirrors the qk / kanbanQueryOptions pattern)
// ---------------------------------------------------------------------------

export const sessionRuntimeQueryOptions = {
  gitStatus: gitStatusQueryOptions,
  gitCommits: gitCommitsQueryOptions,
  context: contextWindowQueryOptions,
  mode: sessionModeQueryOptions,
  models: sessionModelsQueryOptions,
  agentCapabilities: agentCapabilitiesQueryOptions,
  promptUsage: promptUsageQueryOptions,
  todos: sessionTodosQueryOptions,
  availableCommands: availableCommandsQueryOptions,
  pollMode: sessionPollModeQueryOptions,
  prepareProgress: prepareProgressQueryOptions,
};
