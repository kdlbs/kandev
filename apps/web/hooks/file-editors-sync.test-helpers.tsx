import { createHash } from "node:crypto";
import { StrictMode, type ReactNode } from "react";
import { act, cleanup } from "@testing-library/react";
import { vi } from "vitest";
import type { DockviewApi, DockviewPanelApi } from "dockview-react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { syncOpenFileFromWorkspace } from "./file-editors-sync";

export const FILE = "src/refresh.ts";
export const REPO = "web";
let sequence = 0;

export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

export function expectedHash(content: string) {
  return createHash("sha256").update(content).digest("hex");
}

export function buffer(path = FILE, repo: string | undefined = REPO) {
  return useDockviewStore.getState().openFiles.get(buildRepoScopedItemId(path, repo))!;
}

export function seed(overrides: Partial<FileEditorState> = {}) {
  const path = overrides.path ?? FILE;
  const repo = Object.hasOwn(overrides, "repo") ? overrides.repo : REPO;
  // i18n-exempt: Test workspace bytes used by genuine hashing assertions, not UI copy.
  const content = overrides.content ?? "initial disk";
  act(() =>
    useDockviewStore.getState().setFileState(buildRepoScopedItemId(path, repo), {
      path,
      repo,
      name: path.split("/").pop()!,
      content,
      originalContent: content,
      originalHash: expectedHash(content),
      isDirty: false,
      ...overrides,
    }),
  );
  return useDockviewStore.getState().openFiles.get(buildRepoScopedItemId(path, repo))!;
}

export function panelHost(path = FILE, repo = REPO, active = false) {
  const listeners = new Set<(event: { isActive: boolean }) => void>();
  const panel = {
    id: `file:${buildRepoScopedItemId(path, repo)}`,
    params: { path, repo, isDirty: true } as Record<string, unknown>,
    setTitle: vi.fn(),
    api: {
      isActive: active,
      onDidActiveChange: vi.fn((listener: (event: { isActive: boolean }) => void) => {
        listeners.add(listener);
        return { dispose: vi.fn(() => listeners.delete(listener)) };
      }),
      updateParameters: vi.fn((params: Record<string, unknown>) => {
        panel.params = params;
      }),
    },
  };
  const host = {
    getPanel: (id: string) => (id === panel.id ? panel : undefined),
    onDidActivePanelChange: () => ({ dispose: vi.fn() }),
    onDidRemovePanel: () => ({ dispose: vi.fn() }),
  } as unknown as DockviewApi;
  act(() => useDockviewStore.setState({ api: host }));
  return {
    host,
    panel,
    api: panel.api as unknown as DockviewPanelApi,
    listeners,
    activate: (isActive = true) => act(() => listeners.forEach((fn) => fn({ isActive }))),
  };
}

export function providers({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>{children}</TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}

export function strictProviders({ children }: { children: ReactNode }) {
  return <StrictMode>{providers({ children })}</StrictMode>;
}

export function selectSession(store: StoreApi<AppState>, id = `refresh-${++sequence}`) {
  act(() => {
    store.setState((state) => ({
      tasks: { ...state.tasks, activeSessionId: id },
      environmentIdBySessionId: { ...state.environmentIdBySessionId, [id]: `env-${id}` },
    }));
  });
  publishGit(store, "initial signature", id);
  return id;
}

export function publishGit(store: StoreApi<AppState>, diff: string, session?: string) {
  const id = session ?? store.getState().tasks.activeSessionId!;
  const status: GitStatusEntry = {
    branch: "main",
    remote_branch: null,
    modified: [FILE],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    repository_name: REPO,
    files: { [FILE]: { path: FILE, status: "modified", staged: false, diff } },
    timestamp: `2026-10-08T01:00:${String(++sequence).padStart(2, "0")}Z`,
  };
  act(() => store.getState().setGitStatus(`env-${id}`, status));
}

export function transport() {
  const previous = getWebSocketClient();
  const client = new WebSocketClient("ws://editor-refresh.invalid");
  const digestDescriptor = Object.getOwnPropertyDescriptor(crypto.subtle, "digest");
  const genuineDigest = crypto.subtle.digest.bind(crypto.subtle);
  const hashes: Promise<ArrayBuffer>[] = [];
  Object.defineProperty(crypto.subtle, "digest", {
    configurable: true,
    writable: true,
    value: (algorithm: AlgorithmIdentifier, data: BufferSource) => {
      const result = genuineDigest(algorithm, data);
      hashes.push(result);
      return result;
    },
  });
  const requests: Array<ReturnType<typeof deferred<unknown>> & { payload: unknown }> = [];
  const operations: Promise<unknown>[] = [];
  vi.spyOn(client, "request").mockImplementation(<T,>(action: string, payload: unknown) => {
    if (action !== "workspace.file.get") throw new Error(`Unexpected transport action: ${action}`);
    const request = { ...deferred<unknown>(), payload };
    requests.push(request);
    // Observe rejection for teardown without altering the production promise.
    operations.push(
      request.promise.then(
        () => undefined,
        () => undefined,
      ),
    );
    return request.promise as Promise<T>;
  });
  setWebSocketClient(client);
  const sync = (overrides: Partial<Parameters<typeof syncOpenFileFromWorkspace>[0]> = {}) => {
    const operation = syncOpenFileFromWorkspace({
      client,
      sessionId: "direct-session",
      fileKey: buildRepoScopedItemId(FILE, REPO),
      path: FILE,
      repo: REPO,
      updateFileState: useDockviewStore.getState().updateFileState,
      isCurrent: () => true,
      ...overrides,
    });
    operations.push(operation);
    return operation;
  };
  const reply = async (index: number, content: string, metadata = {}) => {
    await act(async () => {
      requests[index].resolve({ path: FILE, content, ...metadata });
      await requests[index].promise;
    });
    await joinHashes();
  };
  const joinHashes = async () => {
    await act(async () => {
      await Promise.all(hashes);
    });
  };
  const dispose = async () => {
    cleanup();
    for (const request of requests) request.resolve({ path: FILE, content: "teardown" });
    await act(async () => {
      await Promise.all(operations);
    });
    await joinHashes();
    client.disconnect();
    setWebSocketClient(previous);
    useDockviewStore.getState().clearFileStates();
    useDockviewStore.setState({ api: null });
    vi.restoreAllMocks();
    if (digestDescriptor) Object.defineProperty(crypto.subtle, "digest", digestDescriptor);
    else Reflect.deleteProperty(crypto.subtle, "digest");
  };
  return { client, requests, sync, reply, joinHashes, dispose };
}

export function pauseHash(content: string) {
  const entered = deferred<void>();
  const release = deferred<void>();
  const finished = deferred<void>();
  let started = false;
  const genuineDigest = crypto.subtle.digest.bind(crypto.subtle);
  const spy = vi.spyOn(crypto.subtle, "digest").mockImplementation(async (algorithm, data) => {
    const selected = new TextDecoder().decode(data) === content;
    if (selected) {
      started = true;
      entered.resolve();
      await release.promise;
    }
    try {
      return await genuineDigest(algorithm, data);
    } finally {
      if (selected) finished.resolve();
    }
  });
  return {
    entered: entered.promise,
    release: () => release.resolve(),
    join: async () => {
      release.resolve();
      if (started) await finished.promise;
    },
    restore: () => spy.mockRestore(),
  };
}
