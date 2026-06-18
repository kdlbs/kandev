import type { StateCreator } from "zustand";
import type {
  SessionRuntimeSlice,
  SessionRuntimeSliceState,
  SessionPollMode,
  GitStatusEntry,
  FileInfo,
} from "./types";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const debugGit = createDebugLogger("git-status:store");

const maxProcessOutputBytes = 2 * 1024 * 1024;
// Shell + terminal streams are unbounded over a session's lifetime; cap them at
// the same 2MB tail the process buffer uses so a chatty shell can't grow the
// store without limit (the xterm view only renders the tail anyway).
const maxShellOutputBytes = 2 * 1024 * 1024;

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

/** Compare two git status entries to determine if a meaningful change occurred. */
export function hasGitStatusChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  // The backend also emits fresh snapshots for focus/startup/poll events. Those
  // can carry a new timestamp for identical git data, so timestamp alone must
  // not force a store update or diff-cache invalidation.

  // Fast path: identical HEAD+base SHAs and identical file-status lists ⇒ the
  // working tree is unchanged, so the O(N) per-file diff-string walk in
  // sameFiles can be skipped. This is the dominant cost when WorkspaceTracker
  // re-emits the same snapshot under heavy rebase load. Requires both sides to
  // carry head_commit; degraded/legacy payloads without it fall through to the
  // full compare. hasFileStatsChanged is intentionally excluded: identical SHAs
  // + lists imply identical totals, so it would only waste cycles.
  if (
    existing.head_commit &&
    incoming.head_commit &&
    existing.head_commit === incoming.head_commit &&
    (existing.base_commit ?? "") === (incoming.base_commit ?? "") &&
    !hasFileListsChanged(existing, incoming) &&
    !hasBranchSummaryChanged(existing, incoming)
  ) {
    return false;
  }

  return (
    hasBranchSummaryChanged(existing, incoming) ||
    hasFileListsChanged(existing, incoming) ||
    hasFileStatsChanged(existing, incoming) ||
    !sameFiles(existing.files, incoming.files)
  );
}

/** Write a git-status update into the env/per-repo maps, skipping writes when
 *  nothing meaningfully changed. Returns whether git state changed so the WS
 *  handler can invalidate derived caches without repeating the deep (per-file,
 *  full-diff-string) comparison — the dominant cost under a massive rebase. */
function applyGitStatus(
  state: SessionRuntimeSliceState,
  sessionId: string,
  gitStatus: GitStatusEntry,
): boolean {
  const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
  // Multi-repo: when the update is tagged with repository_name, route it into
  // the per-repo map. Single-repo updates (no name) keep the legacy single-
  // status path; the per-repo map mirrors the same entry under an empty key so
  // consumers using only byEnvironmentRepo still see it.
  const repoName = gitStatus.repository_name ?? "";
  const repoMap = (state.gitStatus.byEnvironmentRepo[envKey] ??= {});
  const existingRepo = repoMap[repoName];
  const repoChanged = !existingRepo || hasGitStatusChanged(existingRepo, gitStatus);
  if (isDebug()) {
    debugGit("setGitStatus", {
      sessionId,
      envKey,
      usingFallbackKey: envKey === sessionId,
      repoName,
      prevFileCount: Object.keys(existingRepo?.files ?? {}).length,
      nextFileCount: Object.keys(gitStatus.files ?? {}).length,
      prevRepoKeys: Object.keys(repoMap),
      willMutate: repoChanged,
    });
  }
  if (repoChanged) {
    repoMap[repoName] = gitStatus;
  }
  if (repoName !== "") {
    // Multi-repo: only mirror into the legacy map when this repo's entry changed.
    if (repoChanged) state.gitStatus.byEnvironmentId[envKey] = gitStatus;
    return repoChanged;
  }
  // The empty-repo entry and byEnvironmentId track together (written and cleared
  // as a pair), so existingRepo presence/equality matches the env entry — reuse
  // repoChanged instead of comparing diffs again.
  const changed = !state.gitStatus.byEnvironmentId[envKey] || repoChanged;
  if (changed) state.gitStatus.byEnvironmentId[envKey] = gitStatus;
  return changed;
}

function trimTailBytes(value: string, maxBytes: number) {
  if (value.length <= maxBytes) {
    return value;
  }
  return value.slice(value.length - maxBytes);
}

function trimProcessOutput(value: string) {
  return trimTailBytes(value, maxProcessOutputBytes);
}

/** Append a chunk to a terminal's output array, dropping the oldest chunks once
 *  the buffered total exceeds the cap so the array can't grow without bound. A
 *  single chunk larger than the cap is itself clamped to the tail, so the buffer
 *  stays bounded even on the very first (or one giant) write. */
function appendTerminalChunk(output: string[], data: string) {
  output.push(trimTailBytes(data, maxShellOutputBytes));
  let total = output.reduce((sum, chunk) => sum + chunk.length, 0);
  while (output.length > 1 && total > maxShellOutputBytes) {
    total -= output[0].length;
    output.shift();
  }
}

/** Per-session runtime maps (keyed directly by sessionId). */
function purgePerSessionRuntime(state: SessionRuntimeSliceState, sessionId: string) {
  delete state.contextWindow.bySessionId[sessionId];
  delete state.availableCommands.bySessionId[sessionId];
  delete state.sessionMode.bySessionId[sessionId];
  delete state.agentCapabilities.bySessionId[sessionId];
  delete state.sessionModels.bySessionId[sessionId];
  delete state.promptUsage.bySessionId[sessionId];
  delete state.sessionTodos.bySessionId[sessionId];
  delete state.prepareProgress.bySessionId[sessionId];
  delete state.sessionPollMode.bySessionId[sessionId];
}

/** Process status + output for every process owned by the session. */
function purgeSessionProcesses(state: SessionRuntimeSliceState, sessionId: string) {
  const processIds = state.processes.processIdsBySessionId[sessionId] ?? [];
  for (const processId of processIds) {
    delete state.processes.outputsByProcessId[processId];
    delete state.processes.processesById[processId];
  }
  delete state.processes.processIdsBySessionId[sessionId];
  delete state.processes.activeProcessBySessionId[sessionId];
  delete state.processes.devProcessBySessionId[sessionId];
}

/** Environment-scoped maps are shared by every session in the same environment,
 *  so only drop them once the last session for that environment is gone. */
function purgeEnvScopedRuntime(state: SessionRuntimeSliceState, envKey: string) {
  const stillReferenced = Object.values(state.environmentIdBySessionId).includes(envKey);
  if (stillReferenced) return;
  delete state.shell.outputs[envKey];
  delete state.shell.statuses[envKey];
  delete state.gitStatus.byEnvironmentId[envKey];
  delete state.gitStatus.byEnvironmentRepo[envKey];
  delete state.sessionCommits.byEnvironmentId[envKey];
  delete state.sessionCommits.loading[envKey];
  delete state.sessionCommits.refetchTrigger[envKey];
  delete state.userShells.byEnvironmentId[envKey];
  delete state.userShells.loading[envKey];
  delete state.userShells.loaded[envKey];
}

/** Drop all runtime state tied to a removed session so closed/replaced sessions
 *  don't leave orphaned buffers, process output, and per-session maps behind. */
export function purgeSessionRuntimeState(state: SessionRuntimeSliceState, sessionId: string) {
  const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
  purgePerSessionRuntime(state, sessionId);
  purgeSessionProcesses(state, sessionId);
  delete state.environmentIdBySessionId[sessionId];
  purgeEnvScopedRuntime(state, envKey);
}

export const defaultSessionRuntimeState: SessionRuntimeSliceState = {
  terminal: { terminals: [] },
  shell: { outputs: {}, statuses: {} },
  processes: {
    outputsByProcessId: {},
    processesById: {},
    processIdsBySessionId: {},
    activeProcessBySessionId: {},
    devProcessBySessionId: {},
  },
  gitStatus: { byEnvironmentId: {}, byEnvironmentRepo: {} },
  environmentIdBySessionId: {},
  sessionCommits: { byEnvironmentId: {}, loading: {}, refetchTrigger: {} },
  contextWindow: { bySessionId: {} },
  agents: { agents: [] },
  availableCommands: { bySessionId: {} },
  sessionMode: { bySessionId: {} },
  agentCapabilities: { bySessionId: {} },
  sessionModels: { bySessionId: {} },
  promptUsage: { bySessionId: {} },
  sessionTodos: { bySessionId: {} },
  userShells: { byEnvironmentId: {}, loading: {}, loaded: {} },
  prepareProgress: { bySessionId: {} },
  sessionPollMode: { bySessionId: {} },
};

type ImmerSet = Parameters<typeof createSessionRuntimeSlice>[0];

function buildTerminalShellProcessActions(set: ImmerSet) {
  return {
    setTerminalOutput: (terminalId: string, data: string) =>
      set((draft) => {
        const existing = draft.terminal.terminals.find((terminal) => terminal.id === terminalId);
        if (existing) {
          appendTerminalChunk(existing.output, data);
        } else {
          // Route the first chunk through appendTerminalChunk too so the cap is
          // enforced even on the initial WS payload for a new terminal.
          const output: string[] = [];
          appendTerminalChunk(output, data);
          draft.terminal.terminals.push({ id: terminalId, output });
        }
      }),
    appendShellOutput: (sessionId: string, data: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.shell.outputs[envKey] = trimTailBytes(
          (draft.shell.outputs[envKey] || "") + data,
          maxShellOutputBytes,
        );
      }),
    setShellStatus: (
      sessionId: string,
      status: { available: boolean; running?: boolean; shell?: string; cwd?: string },
    ) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.shell.statuses[envKey] = status;
      }),
    clearShellOutput: (sessionId: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.shell.outputs[envKey] = "";
      }),
    appendProcessOutput: (processId: string, data: string) =>
      set((draft) => {
        const next = (draft.processes.outputsByProcessId[processId] || "") + data;
        draft.processes.outputsByProcessId[processId] = trimProcessOutput(next);
      }),
    upsertProcessStatus: (status: Parameters<SessionRuntimeSlice["upsertProcessStatus"]>[0]) =>
      set((draft) => {
        draft.processes.processesById[status.processId] = status;
        const list = draft.processes.processIdsBySessionId[status.sessionId] || [];
        if (!list.includes(status.processId)) {
          draft.processes.processIdsBySessionId[status.sessionId] = [...list, status.processId];
        }
        if (status.kind === "dev") {
          draft.processes.devProcessBySessionId[status.sessionId] = status.processId;
        }
      }),
    clearProcessOutput: (processId: string) =>
      set((draft) => {
        draft.processes.outputsByProcessId[processId] = "";
      }),
    setActiveProcess: (sessionId: string, processId: string) =>
      set((draft) => {
        draft.processes.activeProcessBySessionId[sessionId] = processId;
      }),
  };
}

function buildSessionCommitActions(set: ImmerSet) {
  return {
    setSessionCommits: (
      sessionId: string,
      commits: Parameters<SessionRuntimeSlice["setSessionCommits"]>[1],
      opts?: { allowEmpty?: boolean },
    ) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const existing = draft.sessionCommits.byEnvironmentId[envKey];
        // Default guard: prevent a stale empty-array response from overwriting
        // commits that arrived via incremental notifications while the request
        // was in flight (race between fetch start and commit_created events).
        //
        // Under stale-while-revalidate, a `commits_reset` or `branch_switched`
        // refetch can *legitimately* return [] — the backend actually has no
        // commits. The caller must opt in to that path with `allowEmpty: true`
        // so the panel stops showing the pre-reset list.
        if (!opts?.allowEmpty && commits.length === 0 && existing && existing.length > 0) {
          return;
        }
        draft.sessionCommits.byEnvironmentId[envKey] = commits;
      }),
    setSessionCommitsLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.sessionCommits.loading[envKey] = loading;
      }),
    addSessionCommit: (
      sessionId: string,
      commit: Parameters<SessionRuntimeSlice["addSessionCommit"]>[1],
    ) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const existing = draft.sessionCommits.byEnvironmentId[envKey] || [];
        // For amend: only replace HEAD (first entry) if it has the same parent
        if (existing.length > 0 && existing[0].parent_sha === commit.parent_sha) {
          existing[0] = commit;
          draft.sessionCommits.byEnvironmentId[envKey] = existing;
        } else {
          draft.sessionCommits.byEnvironmentId[envKey] = [commit, ...existing];
        }
      }),
    clearSessionCommits: (sessionId: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        delete draft.sessionCommits.byEnvironmentId[envKey];
      }),
    bumpSessionCommitsRefetch: (sessionId: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const prev = draft.sessionCommits.refetchTrigger[envKey] ?? 0;
        draft.sessionCommits.refetchTrigger[envKey] = prev + 1;
      }),
  };
}

function buildUserShellActions(set: ImmerSet) {
  return {
    setUserShells: (
      environmentId: string,
      shells: Parameters<SessionRuntimeSlice["setUserShells"]>[1],
    ) =>
      set((draft) => {
        if (!environmentId) return;
        draft.userShells.byEnvironmentId[environmentId] = shells;
        draft.userShells.loaded[environmentId] = true;
        draft.userShells.loading[environmentId] = false;
      }),
    setUserShellsLoading: (environmentId: string, loading: boolean) =>
      set((draft) => {
        if (!environmentId) return;
        draft.userShells.loading[environmentId] = loading;
      }),
    addUserShell: (
      environmentId: string,
      shell: Parameters<SessionRuntimeSlice["addUserShell"]>[1],
    ) =>
      set((draft) => {
        if (!environmentId) return;
        const existing = draft.userShells.byEnvironmentId[environmentId] || [];
        if (!existing.some((s) => s.terminalId === shell.terminalId)) {
          draft.userShells.byEnvironmentId[environmentId] = [...existing, shell];
        }
      }),
    removeUserShell: (environmentId: string, terminalId: string) =>
      set((draft) => {
        if (!environmentId) return;
        const existing = draft.userShells.byEnvironmentId[environmentId] || [];
        draft.userShells.byEnvironmentId[environmentId] = existing.filter(
          (s) => s.terminalId !== terminalId,
        );
      }),
    updateUserShell: (
      environmentId: string,
      terminalId: string,
      patch: Parameters<SessionRuntimeSlice["updateUserShell"]>[2],
    ) =>
      set((draft) => {
        if (!environmentId) return;
        const existing = draft.userShells.byEnvironmentId[environmentId];
        if (!existing) return;
        draft.userShells.byEnvironmentId[environmentId] = existing.map((s) =>
          s.terminalId === terminalId ? { ...s, ...patch } : s,
        );
      }),
    setSessionPollMode: (sessionId: string, mode: SessionPollMode) =>
      set((draft) => {
        draft.sessionPollMode.bySessionId[sessionId] = mode;
      }),
  };
}

/**
 * Migrate any env-keyed data stored under the fallback `sessionId` key to the
 * proper `environmentId` key so selectors don't see stale data after the
 * session→environment mapping is registered.
 */
export function migrateEnvKeyedData(
  draft: SessionRuntimeSliceState,
  sessionId: string,
  environmentId: string,
) {
  if (sessionId === environmentId) return;
  const migrate = <T>(store: Record<string, T>) => {
    if (sessionId in store) {
      if (!(environmentId in store)) {
        store[environmentId] = store[sessionId];
      }
      delete store[sessionId];
    }
  };
  migrate(draft.sessionCommits.byEnvironmentId);
  migrate(draft.gitStatus.byEnvironmentRepo);
  migrate(draft.sessionCommits.loading);
  migrate(draft.sessionCommits.refetchTrigger);
  migrate(draft.gitStatus.byEnvironmentId);
  migrate(draft.shell.outputs);
  migrate(draft.shell.statuses);
  migrate(draft.userShells.byEnvironmentId);
  migrate(draft.userShells.loading);
  migrate(draft.userShells.loaded);
}

function buildContextWindowActions(set: ImmerSet) {
  return {
    setContextWindow: (
      sessionId: string,
      contextWindow: Parameters<SessionRuntimeSlice["setContextWindow"]>[1],
    ) =>
      set((draft) => {
        draft.contextWindow.bySessionId[sessionId] = contextWindow;
      }),
    clearContextWindow: (sessionId: string) =>
      set((draft) => {
        delete draft.contextWindow.bySessionId[sessionId];
      }),
  };
}

export const createSessionRuntimeSlice: StateCreator<
  SessionRuntimeSlice,
  [["zustand/immer", never]],
  [],
  SessionRuntimeSlice
> = (set) => ({
  ...defaultSessionRuntimeState,
  ...buildTerminalShellProcessActions(set),
  // Returns whether the git state meaningfully changed so callers (the WS
  // handler) can decide to invalidate derived caches without re-running the
  // expensive deep comparison themselves. The comparison walks every file's
  // full diff string, so under a heavy rebase (thousands of files, frequent
  // updates) it must run at most once per event — not once here and again in
  // the caller.
  setGitStatus: (sessionId, gitStatus) => {
    let changed = false;
    set((draft) => {
      changed = applyGitStatus(draft, sessionId, gitStatus);
    });
    return changed;
  },
  clearGitStatus: (sessionId) =>
    set((draft) => {
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      delete draft.gitStatus.byEnvironmentId[envKey];
      delete draft.gitStatus.byEnvironmentRepo[envKey];
    }),
  clearLegacyGitStatusEntry: (sessionId) =>
    set((draft) => {
      // Drops the single-repo (empty-repo-name) entries so a session that just
      // transitioned to multi-repo via add_branch_to_task stops surfacing the
      // pre-transition snapshot — its workspace tracker was replaced on the
      // backend and will never emit another update under the empty key. The
      // per-repo entries (real repo names) are intentionally left in place;
      // they continue to receive fresh status updates from the new trackers.
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      const repoMap = draft.gitStatus.byEnvironmentRepo[envKey];
      if (repoMap && "" in repoMap) {
        delete repoMap[""];
      }
      delete draft.gitStatus.byEnvironmentId[envKey];
    }),
  registerSessionEnvironment: (sessionId, environmentId) =>
    set((draft) => {
      draft.environmentIdBySessionId[sessionId] = environmentId;
      migrateEnvKeyedData(draft, sessionId, environmentId);
    }),
  ...buildContextWindowActions(set),
  ...buildSessionCommitActions(set),
  setAvailableCommands: (sessionId, commands) =>
    set((draft) => {
      draft.availableCommands.bySessionId[sessionId] = commands;
    }),
  clearAvailableCommands: (sessionId) =>
    set((draft) => {
      delete draft.availableCommands.bySessionId[sessionId];
    }),
  setSessionMode: (sessionId, modeId, availableModes) =>
    set((draft) => {
      const existing = draft.sessionMode.bySessionId[sessionId];
      draft.sessionMode.bySessionId[sessionId] = {
        currentModeId: modeId,
        availableModes: availableModes ?? existing?.availableModes ?? [],
      };
    }),
  clearSessionMode: (sessionId) =>
    set((draft) => {
      delete draft.sessionMode.bySessionId[sessionId];
    }),
  setAgentCapabilities: (sessionId, caps) =>
    set((draft) => {
      draft.agentCapabilities.bySessionId[sessionId] = caps;
    }),
  setSessionModels: (sessionId, data) =>
    set((draft) => {
      draft.sessionModels.bySessionId[sessionId] = data;
    }),
  setPromptUsage: (sessionId, usage) =>
    set((draft) => {
      draft.promptUsage.bySessionId[sessionId] = usage;
    }),
  setSessionTodos: (sessionId, entries) =>
    set((draft) => {
      draft.sessionTodos.bySessionId[sessionId] = entries;
    }),
  ...buildUserShellActions(set),
});
