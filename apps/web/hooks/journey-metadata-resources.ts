import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import {
  SharedResourceReads,
  raceSharedRead,
  storeReadScopeIdentity,
} from "@/lib/state/shared-resource-reads";
import { fetchUserSettings, listRepositories, listWorkflows, listWorkspaces } from "@/lib/api";
import { getTaskCIAutomationOptions } from "@/lib/api/domains/github-api";
import { getAgentProfileMcpConfigAction } from "@/app/actions/agents";

type ReadOptions = { signal?: AbortSignal; refresh?: boolean };

/** Each projection has one flight per store and authorized workspace generation. */
export function createScopedMetadataRead<T>() {
  const owners = new WeakMap<
    StoreApi<AppState>,
    { identity: string; reads: SharedResourceReads<T> }
  >();
  return async (
    store: StoreApi<AppState>,
    key: string,
    load: (signal: AbortSignal) => Promise<T>,
    options: ReadOptions = {},
  ): Promise<T> => {
    if (options.signal?.aborted) throw new DOMException("The operation was aborted", "AbortError");
    const identity = storeReadScopeIdentity(store);
    let owner = owners.get(store);
    if (owner?.identity !== identity) {
      owner?.reads.dispose();
      owner = {
        identity,
        reads: new SharedResourceReads(() => storeReadScopeIdentity(store) === identity),
      };
      owners.set(store, owner);
    }
    const release = owner.reads.retain(key);
    options.signal?.addEventListener("abort", release, { once: true });
    try {
      const request = owner.reads.read(key, load, options);
      const result = await (options.signal ? raceSharedRead(request, options.signal) : request);
      if (storeReadScopeIdentity(store) !== identity)
        throw new DOMException("The operation was aborted", "AbortError");
      return result;
    } finally {
      options.signal?.removeEventListener("abort", release);
      release();
    }
  };
}

function metadataRead<T>() {
  const read = createScopedMetadataRead<T>();
  return (
    store: StoreApi<AppState> | undefined,
    key: string,
    load: (signal: AbortSignal) => Promise<T>,
    options: ReadOptions = {},
  ) =>
    store ? read(store, key, load, options) : load(options.signal ?? new AbortController().signal);
}

const repositoryRead = metadataRead<Awaited<ReturnType<typeof listRepositories>>>();
const workspaceRead = metadataRead<Awaited<ReturnType<typeof listWorkspaces>>>();
const workflowRead = metadataRead<Awaited<ReturnType<typeof listWorkflows>>>();
const settingsRead = metadataRead<Awaited<ReturnType<typeof fetchUserSettings>>>();
const ciRead = metadataRead<Awaited<ReturnType<typeof getTaskCIAutomationOptions>>>();
const mcpRead = metadataRead<Awaited<ReturnType<typeof getAgentProfileMcpConfigAction>>>();

export function readJourneyRepositories(
  store: StoreApi<AppState> | undefined,
  workspaceId: string,
  options: ReadOptions & { includeScripts?: boolean; retryTransient?: boolean } = {},
) {
  const includeScripts = options.includeScripts ?? false;
  return repositoryRead(
    store,
    JSON.stringify([workspaceId, includeScripts]),
    (signal) =>
      loadRepositories(workspaceId, includeScripts, signal, options.retryTransient ?? true),
    options,
  );
}

async function loadRepositories(
  workspaceId: string,
  includeScripts: boolean,
  signal: AbortSignal,
  retryTransient: boolean,
) {
  const delays = includeScripts || !retryTransient ? [] : [100, 250, 500, 1_000];
  for (const delay of delays) {
    try {
      return await listRepositories(
        workspaceId,
        { includeScripts },
        { cache: "no-store", init: { signal } },
      );
    } catch (error) {
      if (signal.aborted) throw error;
      await abortableDelay(delay, signal);
    }
  }
  return listRepositories(workspaceId, { includeScripts }, { cache: "no-store", init: { signal } });
}

function abortableDelay(delay: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const onAbort = () => {
      clearTimeout(timer);
      reject(new DOMException("The operation was aborted", "AbortError"));
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, delay);
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });
  });
}

export function readJourneyWorkspaces(
  store: StoreApi<AppState> | undefined,
  options: ReadOptions = {},
) {
  return workspaceRead(
    store,
    "workspaces",
    (signal) => listWorkspaces({ cache: "no-store", init: { signal } }),
    options,
  );
}

export function readJourneyWorkflows(
  store: StoreApi<AppState> | undefined,
  workspaceId: string,
  options: ReadOptions & { includeHidden?: boolean } = {},
) {
  const includeHidden = options.includeHidden ?? false;
  return workflowRead(
    store,
    JSON.stringify([workspaceId, includeHidden]),
    (signal) => listWorkflows(workspaceId, { cache: "no-store", includeHidden, init: { signal } }),
    options,
  );
}

export function readJourneyUserSettings(
  store: StoreApi<AppState> | undefined,
  options: ReadOptions = {},
) {
  return settingsRead(
    store,
    "user-settings",
    (signal) => fetchUserSettings({ cache: "no-store", init: { signal } }),
    options,
  );
}

export function readJourneyCIOptions(
  store: StoreApi<AppState>,
  taskId: string,
  options: ReadOptions = {},
) {
  return ciRead(
    store,
    taskId,
    (signal) => getTaskCIAutomationOptions(taskId, { cache: "no-store", init: { signal } }),
    options,
  );
}

export function readJourneyMcpConfig(
  store: StoreApi<AppState>,
  profileId: string,
  options: ReadOptions = {},
) {
  return mcpRead(
    store,
    profileId,
    (signal) => getAgentProfileMcpConfigAction(profileId, { signal }),
    options,
  );
}
