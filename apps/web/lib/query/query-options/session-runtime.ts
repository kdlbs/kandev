import { queryOptions } from "@tanstack/react-query";
import type { SessionAgentctlStatus } from "@/lib/state/slices/session/types";
import type {
  AgentCapabilitiesEntry,
  AvailableCommand,
  ContextWindowEntry,
  GitStatusEntry,
  ProcessStatusEntry,
  PromptUsageEntry,
  SessionCommit,
  SessionModeEntry,
  SessionModelEntry,
  ConfigOptionEntry,
  SessionPollMode,
  SessionPrepareState,
  TodoEntry,
  UserShellInfo,
} from "@/lib/state/slices/session-runtime/types";
import { getWebSocketClient } from "@/lib/ws/connection";
import { qk } from "../keys";

export type GitStatusQueryData = {
  latest?: GitStatusEntry;
  byRepo: Record<string, GitStatusEntry>;
};

export type SessionModelsQueryData = {
  currentModelId: string;
  models: SessionModelEntry[];
  configOptions: ConfigOptionEntry[];
};

export type SessionModeQueryData = {
  currentModeId: string;
  availableModes: SessionModeEntry[];
};

export type SessionProcessesQueryData = {
  processesById: Record<string, ProcessStatusEntry>;
  processIds: string[];
  activeProcessId?: string;
  devProcessId?: string;
};

export type SessionCommitsResponse = {
  commits: SessionCommit[];
  ready: boolean;
};

function passiveQueryOptions<T>(queryKey: readonly unknown[], enabled = true) {
  return queryOptions<T | null>({
    queryKey,
    queryFn: async () => null,
    enabled: false,
    staleTime: Infinity,
    gcTime: 10 * 60_000,
    meta: { passive: enabled },
  });
}

export function gitStatusQueryOptions(environmentId: string) {
  return passiveQueryOptions<GitStatusQueryData>(qk.sessionRuntime.gitStatus(environmentId));
}

export async function fetchSessionCommitsSnapshot(
  sessionId: string,
): Promise<SessionCommitsResponse> {
  const client = getWebSocketClient();
  if (!client) return { commits: [], ready: false };
  const response = await client.request<{ commits?: SessionCommit[]; ready?: boolean }>(
    "session.git.commits",
    { session_id: sessionId },
  );
  return { commits: response.commits ?? [], ready: response.ready !== false };
}

export function sessionCommitsQueryOptions(environmentId: string, sessionId: string) {
  return queryOptions({
    queryKey: qk.sessionRuntime.commits(environmentId),
    queryFn: async () => {
      const response = await fetchSessionCommitsSnapshot(sessionId);
      if (!response.ready) {
        throw new Error("session commits are not ready");
      }
      return response.commits;
    },
    enabled: Boolean(environmentId && sessionId),
    staleTime: 60_000,
  });
}

export function prepareProgressQueryOptions(sessionId: string) {
  return passiveQueryOptions<SessionPrepareState>(qk.sessionRuntime.prepare(sessionId));
}

export function contextWindowQueryOptions(sessionId: string) {
  return passiveQueryOptions<ContextWindowEntry>(qk.sessionRuntime.contextWindow(sessionId));
}

export function availableCommandsQueryOptions(sessionId: string) {
  return passiveQueryOptions<AvailableCommand[]>(qk.sessionRuntime.availableCommands(sessionId));
}

export function sessionModeQueryOptions(sessionId: string) {
  return passiveQueryOptions<SessionModeQueryData>(qk.sessionRuntime.mode(sessionId));
}

export function agentCapabilitiesQueryOptions(sessionId: string) {
  return passiveQueryOptions<AgentCapabilitiesEntry>(
    qk.sessionRuntime.agentCapabilities(sessionId),
  );
}

export function sessionModelsQueryOptions(sessionId: string) {
  return passiveQueryOptions<SessionModelsQueryData>(qk.sessionRuntime.models(sessionId));
}

export function promptUsageQueryOptions(sessionId: string) {
  return passiveQueryOptions<PromptUsageEntry>(qk.sessionRuntime.promptUsage(sessionId));
}

export function sessionTodosQueryOptions(sessionId: string) {
  return passiveQueryOptions<TodoEntry[]>(qk.sessionRuntime.todos(sessionId));
}

export function sessionPollModeQueryOptions(sessionId: string) {
  return passiveQueryOptions<SessionPollMode>(qk.sessionRuntime.pollMode(sessionId));
}

export function sessionAgentctlQueryOptions(sessionId: string) {
  return passiveQueryOptions<SessionAgentctlStatus>(qk.sessionRuntime.agentctl(sessionId));
}

export function sessionProcessesQueryOptions(sessionId: string) {
  return passiveQueryOptions<SessionProcessesQueryData>(qk.sessionRuntime.processes(sessionId));
}

export async function fetchUserShellsSnapshot(
  environmentId: string,
  taskId?: string | null,
): Promise<UserShellInfo[]> {
  const client = getWebSocketClient();
  if (!client) return [];
  const payload: Record<string, unknown> = {
    task_environment_id: environmentId,
    include_parked: true,
  };
  if (taskId) payload.task_id = taskId;
  const response = await client.request<{ shells?: UserShellListItem[] }>(
    "user_shell.list",
    payload,
    10000,
  );
  return (response.shells ?? []).map(mapTerminalInfoToUserShell);
}

export function userShellsQueryOptions(environmentId: string, taskId?: string | null) {
  return queryOptions({
    queryKey: qk.sessionRuntime.userShells(environmentId, taskId),
    queryFn: () => fetchUserShellsSnapshot(environmentId, taskId),
    enabled: Boolean(environmentId),
    staleTime: 30_000,
  });
}

type UserShellListItem = {
  id?: string;
  terminal_id?: string;
  kind?: UserShellInfo["kind"];
  seq?: number;
  display_name?: string;
  custom_name?: string | null;
  state?: UserShellInfo["state"];
  pty_status?: UserShellInfo["ptyStatus"];
  label?: string;
  closable?: boolean;
  initial_command?: string;
  process_id?: string;
  running?: boolean;
};

function mapTerminalInfoToUserShell(item: UserShellListItem): UserShellInfo {
  const terminalId = item.id ?? item.terminal_id ?? "";
  return {
    terminalId,
    kind: item.kind,
    seq: item.seq,
    customName: item.custom_name ?? null,
    displayName: item.display_name,
    state: item.state,
    ptyStatus: item.pty_status,
    processId: item.process_id,
    running: item.running ?? item.pty_status === "running",
    label: item.label || item.display_name || "Terminal",
    closable: item.closable ?? true,
    initialCommand: item.initial_command,
  };
}
