import type { SliceCreator } from "@/lib/state/slice-creator";

import type { SessionRuntimeSlice, SessionRuntimeSliceState } from "./types";
import { normalizeGitStatusEntry } from "./git-status-normalizer";
import { applyGitStatus } from "./git-status-state";
import {
  invalidateGitStatusInventory as invalidateInventoryState,
  reconcileGitStatusInventory as reconcileInventoryState,
} from "./git-status-inventory";
import {
  beginWorkspaceRestoration,
  clearWorkspaceRestoration,
  completeWorkspaceRestoration,
  failWorkspaceRestoration,
} from "./workspace-restoration";
import { buildSessionViewActions } from "./session-runtime-view-actions";
import { buildSessionGitCheckoutActions } from "./session-runtime-git-checkout-actions";
import { buildBackgroundWorkActions } from "./session-runtime-background-work";
import {
  buildUserShellActions,
  migrateUserShellScopes,
  purgeUserShellEnvironment,
} from "./user-shells-state";

const maxProcessOutputBytes = 2 * 1024 * 1024;
// Shell + terminal streams are unbounded over a session's lifetime; cap them at
// the same 2MB tail the process buffer uses so a chatty shell can't grow the
// store without limit (the xterm view only renders the tail anyway).
const maxShellOutputBytes = 2 * 1024 * 1024;

function trimTailBytes(value: string, maxBytes: number) {
  if (value.length <= maxBytes) {
    return value;
  }
  return value.slice(value.length - maxBytes);
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
  delete state.sessionMcpStatus.bySessionId[sessionId];
  delete state.promptUsage.bySessionId[sessionId];
  delete state.usageInvalidation.bySessionId[sessionId];
  delete state.sessionTodos.bySessionId[sessionId];
  delete state.prepareProgress.bySessionId[sessionId];
  delete state.launchWarning.bySessionId[sessionId];
  delete state.sessionPollMode.bySessionId[sessionId];
  delete state.embeddedVscodeSupport.bySessionId[sessionId];
  delete state.backgroundWork.workloadsBySessionId[sessionId];
  delete state.backgroundWork.outputWindowsBySessionId[sessionId];
  delete state.backgroundWork.activeWorkIdBySessionId[sessionId];
  delete state.backgroundWork.loadingBySessionId[sessionId];
  delete state.backgroundWork.readGenerationBySessionId[sessionId];
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
  delete state.gitStatusDisplay.byEnvironmentRepo[envKey];
  delete state.gitStatus.refreshByEnvironmentId?.[envKey];
  delete state.gitStatus.refreshByEnvironmentRepo?.[envKey];
  delete state.gitStatus.inventoryByEnvironment?.[envKey];
  delete state.sessionCommits.byEnvironmentId[envKey];
  delete state.sessionCommits.loading[envKey];
  delete state.sessionCommits.refetchTrigger[envKey];
  delete state.gitCheckoutGeneration.byEnvironmentId[envKey];
  purgeUserShellEnvironment(state.userShells, envKey);
  delete state.workspaceRestoration.byEnvironmentId[envKey];
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
  gitStatus: {
    byEnvironmentId: {},
    byEnvironmentRepo: {},
    refreshByEnvironmentId: {},
    refreshByEnvironmentRepo: {},
    inventoryByEnvironment: {},
  },
  gitStatusDisplay: { byEnvironmentRepo: {} },
  environmentIdBySessionId: {},
  sessionCommits: { byEnvironmentId: {}, loading: {}, refetchTrigger: {} },
  gitCheckoutGeneration: { byEnvironmentId: {} },
  contextWindow: { bySessionId: {} },
  agents: { agents: [] },
  availableCommands: { bySessionId: {} },
  sessionMode: { bySessionId: {} },
  agentCapabilities: { bySessionId: {} },
  sessionModels: { bySessionId: {} },
  sessionMcpStatus: { bySessionId: {} },
  promptUsage: { bySessionId: {} },
  usageInvalidation: { bySessionId: {} },
  sessionTodos: { bySessionId: {} },
  userShells: {
    byScope: {},
    byEnvironmentId: {},
    dismissedByScope: {},
    dismissedByEnvironmentId: {},
    loading: {},
    loaded: {},
    listed: {},
    sharedGeneration: {},
  },
  prepareProgress: { bySessionId: {} },
  launchWarning: { bySessionId: {} },
  sessionPollMode: { bySessionId: {} },
  embeddedVscodeSupport: { bySessionId: {} },
  workspaceFilesRefresh: { bySessionId: {} },
  workspaceRestoration: { byEnvironmentId: {} },
  backgroundWork: {
    workloadsBySessionId: {},
    outputWindowsBySessionId: {},
    nextOutputWindowGeneration: 1,
    activeWorkIdBySessionId: {},
    loadingBySessionId: {},
    readGenerationBySessionId: {},
    nextReadGeneration: 1,
  },
};

export type ImmerSet = Parameters<typeof createSessionRuntimeSlice>[0];

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
        draft.processes.outputsByProcessId[processId] = trimTailBytes(next, maxProcessOutputBytes);
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
        const duplicateIndex = existing.findIndex(
          (current) =>
            current.commit_sha === commit.commit_sha &&
            (current.repository_name ?? "") === (commit.repository_name ?? ""),
        );
        // A commit_created notification can arrive again while a refetch is
        // replacing the list. Update the fetched row in place so the same
        // commit is never rendered twice.
        if (duplicateIndex >= 0) {
          existing[duplicateIndex] = { ...existing[duplicateIndex], ...commit };
          draft.sessionCommits.byEnvironmentId[envKey] = existing;
          return;
        }
        // An amend replaces only the leading commit within its repository scope.
        const repositoryHeadIndex = existing.findIndex(
          (current) => (current.repository_name ?? "") === (commit.repository_name ?? ""),
        );
        if (
          repositoryHeadIndex >= 0 &&
          existing[repositoryHeadIndex].parent_sha === commit.parent_sha
        ) {
          existing[repositoryHeadIndex] = commit;
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
  migrate(draft.gitStatusDisplay.byEnvironmentRepo);
  if (draft.gitStatus.refreshByEnvironmentId) migrate(draft.gitStatus.refreshByEnvironmentId);
  if (draft.gitStatus.refreshByEnvironmentRepo) migrate(draft.gitStatus.refreshByEnvironmentRepo);
  if (draft.gitStatus.inventoryByEnvironment) migrate(draft.gitStatus.inventoryByEnvironment);
  migrate(draft.sessionCommits.loading);
  migrate(draft.sessionCommits.refetchTrigger);
  migrate(draft.gitCheckoutGeneration.byEnvironmentId);
  migrate(draft.gitStatus.byEnvironmentId);
  migrate(draft.shell.outputs);
  migrate(draft.shell.statuses);
  migrate(draft.userShells.byEnvironmentId);
  migrate(draft.userShells.dismissedByEnvironmentId);
  migrateUserShellScopes(draft, sessionId, environmentId);
  const workspaceAttempt = draft.workspaceRestoration.byEnvironmentId[sessionId];
  if (workspaceAttempt) {
    if (!(environmentId in draft.workspaceRestoration.byEnvironmentId)) {
      draft.workspaceRestoration.byEnvironmentId[environmentId] = {
        ...workspaceAttempt,
        environmentId,
      };
    }
    delete draft.workspaceRestoration.byEnvironmentId[sessionId];
  }
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

function buildWorkspaceRestorationActions(set: ImmerSet) {
  return {
    beginWorkspaceRestoration: (taskId: string, sessionId: string, environmentId: string) => {
      let attempt: ReturnType<typeof beginWorkspaceRestoration> = null;
      set((draft) => {
        attempt = beginWorkspaceRestoration(draft.workspaceRestoration, {
          taskId,
          sessionId,
          environmentId,
        });
      });
      return attempt;
    },
    completeWorkspaceRestoration: (
      attempt: Parameters<SessionRuntimeSlice["completeWorkspaceRestoration"]>[0],
    ) => {
      let completed = false;
      set((draft) => {
        completed = completeWorkspaceRestoration(draft.workspaceRestoration, attempt);
      });
      return completed;
    },
    failWorkspaceRestoration: (
      attempt: Parameters<SessionRuntimeSlice["failWorkspaceRestoration"]>[0],
      details: string,
    ) => {
      let failed = false;
      set((draft) => {
        failed = failWorkspaceRestoration(draft.workspaceRestoration, attempt, details);
      });
      return failed;
    },
    clearWorkspaceRestoration: (
      attempt: Parameters<SessionRuntimeSlice["clearWorkspaceRestoration"]>[0],
    ) => {
      let cleared = false;
      set((draft) => {
        cleared = clearWorkspaceRestoration(draft.workspaceRestoration, attempt);
      });
      return cleared;
    },
  };
}

function buildGitInventoryActions(set: ImmerSet) {
  return {
    invalidateGitStatusInventory: (sessionId: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        invalidateInventoryState(draft, envKey);
      }),
    reconcileGitStatusInventory: (
      environmentId: string,
      generation: number,
      scopes: readonly string[],
    ) => {
      let applied = false;
      set((draft) => {
        applied = reconcileInventoryState(draft, environmentId, generation, scopes);
      });
      return applied;
    },
  };
}

export const createSessionRuntimeSlice: SliceCreator<SessionRuntimeSlice> = (set) => ({
  ...defaultSessionRuntimeState,
  ...buildTerminalShellProcessActions(set),
  // Returns whether the git state meaningfully changed so callers (the WS
  // handler) can decide to invalidate derived caches without re-running the
  // expensive deep comparison themselves. The comparison walks every file's
  // full diff string, so under a heavy rebase (thousands of files, frequent
  // updates) it must run at most once per event — not once here and again in
  // the caller.
  setGitStatus: (taskEnvironmentId, gitStatus) => {
    let changed = false;
    set((draft) => {
      changed = applyGitStatus(draft, taskEnvironmentId, normalizeGitStatusEntry(gitStatus));
    });
    return changed;
  },
  setGitStatusRefresh: (taskEnvironmentId, repositoryName, refresh) =>
    set((draft) => {
      if (!taskEnvironmentId) return;
      if (repositoryName === undefined) {
        const byEnvironmentId = (draft.gitStatus.refreshByEnvironmentId ??= {});
        if (refresh) byEnvironmentId[taskEnvironmentId] = refresh;
        else delete byEnvironmentId[taskEnvironmentId];
        return;
      }
      const byEnvironmentRepo = (draft.gitStatus.refreshByEnvironmentRepo ??= {});
      const repoMap = (byEnvironmentRepo[taskEnvironmentId] ??= {});
      if (refresh) repoMap[repositoryName] = refresh;
      else {
        delete repoMap[repositoryName];
        if (Object.keys(repoMap).length === 0) {
          delete draft.gitStatus.refreshByEnvironmentRepo?.[taskEnvironmentId];
        }
      }
    }),
  clearGitStatus: (sessionId) =>
    set((draft) => {
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      delete draft.gitStatus.byEnvironmentId[envKey];
      delete draft.gitStatus.byEnvironmentRepo[envKey];
      delete draft.gitStatusDisplay.byEnvironmentRepo[envKey];
      delete draft.gitStatus.refreshByEnvironmentId?.[envKey];
      delete draft.gitStatus.refreshByEnvironmentRepo?.[envKey];
    }),
  bumpWorkspaceFilesRefresh: (sessionId) =>
    set((draft) => {
      draft.workspaceFilesRefresh.bySessionId[sessionId] =
        (draft.workspaceFilesRefresh.bySessionId[sessionId] ?? 0) + 1;
    }),
  clearLegacyGitStatusEntry: (sessionId) =>
    set((draft) => {
      // Drops the single-repo (empty-repo-name) entries so a session that just
      // transitioned to multi-repo via add_branch_to_task stops surfacing the
      // pre-transition snapshot — its workspace tracker was replaced on the
      // backend and will never emit another update under the empty key. The
      // per-repo entries (real repo names) are intentionally left in place;
      // they continue to receive fresh status updates from the new trackers.
      // Refresh failures belong to the same retired scope and must go too, or
      // a blank repository name keeps the warning visible after every named
      // repository has recovered.
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      const repoMap = draft.gitStatus.byEnvironmentRepo[envKey];
      if (repoMap && "" in repoMap) {
        delete repoMap[""];
      }
      const refreshRepoMap = draft.gitStatus.refreshByEnvironmentRepo?.[envKey];
      if (refreshRepoMap && "" in refreshRepoMap) {
        delete refreshRepoMap[""];
        if (Object.keys(refreshRepoMap).length === 0) {
          delete draft.gitStatus.refreshByEnvironmentRepo?.[envKey];
        }
      }
      delete draft.gitStatusDisplay.byEnvironmentRepo[envKey]?.[""];
      delete draft.gitStatus.byEnvironmentId[envKey];
      // The environment-level failure belonged to the retired unnamed root.
      delete draft.gitStatus.refreshByEnvironmentId?.[envKey];
    }),
  ...buildGitInventoryActions(set),
  registerSessionEnvironment: (sessionId, environmentId) =>
    set((draft) => {
      draft.environmentIdBySessionId[sessionId] = environmentId;
      migrateEnvKeyedData(draft, sessionId, environmentId);
    }),
  ...buildWorkspaceRestorationActions(set),
  ...buildContextWindowActions(set),
  ...buildSessionCommitActions(set),
  ...buildSessionGitCheckoutActions(set),
  setAvailableCommands: (sessionId, commands) =>
    set((draft) => {
      draft.availableCommands.bySessionId[sessionId] = commands;
    }),
  clearAvailableCommands: (sessionId) =>
    set((draft) => {
      delete draft.availableCommands.bySessionId[sessionId];
    }),
  setSessionMode: (sessionId, modeId, availableModes, requestedModeId, settingsPolicy) =>
    set((draft) => {
      const existing = draft.sessionMode.bySessionId[sessionId];
      const nextSettingsPolicy =
        settingsPolicy === "strict" ? undefined : (settingsPolicy ?? existing?.settingsPolicy);
      draft.sessionMode.bySessionId[sessionId] = {
        currentModeId: modeId,
        availableModes: availableModes ?? existing?.availableModes ?? [],
        requestedModeId,
        ...(nextSettingsPolicy ? { settingsPolicy: nextSettingsPolicy } : {}),
      };
    }),
  clearSessionMode: (sessionId) =>
    set((draft) => {
      delete draft.sessionMode.bySessionId[sessionId];
    }),
  ...buildSessionViewActions(set),
  ...buildBackgroundWorkActions(set),
  ...buildUserShellActions(set),
});
