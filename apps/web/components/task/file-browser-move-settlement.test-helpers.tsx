import { useState } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { expect, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { useFileOperations } from "@/hooks/use-file-operations";
import { sessionId, taskId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import type { FileTreeNode } from "@/lib/types/backend";
import { FileBrowser } from "./file-browser";
import { getFileBrowserResetKey } from "./file-browser-data";
import { workspaceInventoryRevision } from "./file-browser-repository-labels";
import { useFileTreeCacheBinding } from "./file-browser-tree-state";

const wire = vi.hoisted(() => ({
  request: vi.fn(),
  on: vi.fn(),
  subscribeSession: vi.fn(() => () => {}),
  subscribeUser: vi.fn(() => () => {}),
  sessionRead: vi.fn(),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => wire,
  subscribeWebSocketClient: () => () => {},
}));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  fetchTaskSessionConditional: wire.sessionRead,
  sendFrontendErrorReport: vi.fn(async () => ({})),
}));

export const transport = wire;

export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

export const file = (path: string, size = 1): FileTreeNode => ({
  path,
  name: path.split("/").at(-1)!,
  is_dir: false,
  size,
});
export const folder = (path: string, children: FileTreeNode[] = []): FileTreeNode => ({
  path,
  name: path.split("/").at(-1) || "workspace",
  is_dir: true,
  children,
});

function remoteNode(tree: FileTreeNode, path: string): FileTreeNode | undefined {
  if (tree.path === path) return tree;
  for (const child of tree.children ?? []) {
    const match = remoteNode(child, path);
    if (match) return match;
  }
}

function remoteRename(tree: FileTreeNode, oldPath: string, newPath: string) {
  const node = remoteNode(tree, oldPath)!;
  const parentPath = oldPath.includes("/") ? oldPath.slice(0, oldPath.lastIndexOf("/")) : "";
  const nextParent = newPath.includes("/") ? newPath.slice(0, newPath.lastIndexOf("/")) : "";
  const source = remoteNode(tree, parentPath)!;
  source.children = source.children!.filter((entry) => entry !== node);
  function relocate(entry: FileTreeNode) {
    entry.path = newPath + entry.path.slice(oldPath.length);
    entry.name = entry.path.split("/").at(-1)!;
    entry.children?.forEach(relocate);
  }
  relocate(node);
  remoteNode(tree, nextParent)!.children!.push(node);
}

class TreeGeometry {
  constructor(private readonly callback: ResizeObserverCallback) {}
  observe(target: Element) {
    const height = target.hasAttribute("data-index") ? 28 : 600;
    this.callback(
      [
        {
          target,
          contentRect: { width: 600, height },
          borderBoxSize: [{ inlineSize: 600, blockSize: height }],
        } as unknown as ResizeObserverEntry,
      ],
      this as unknown as ResizeObserver,
    );
  }
  unobserve() {}
  disconnect() {}
}

type RenameResponse = { success: boolean; error?: string };
type TreeResponse = { root: FileTreeNode };
type Read = {
  path: string;
  snapshot: FileTreeNode;
  reply: ReturnType<typeof deferred<TreeResponse>>;
};
const listeners = new Map<string, Set<(event: unknown) => void>>();
const outstanding = new Set<() => void>();
let sequence = 0;

export function setupTransport() {
  vi.clearAllMocks();
  sessionStorage.clear();
  listeners.clear();
  vi.stubGlobal("ResizeObserver", TreeGeometry);
  transport.on.mockImplementation((event: string, listener: (value: unknown) => void) => {
    const group = listeners.get(event) ?? new Set();
    group.add(listener);
    listeners.set(event, group);
    return () => group.delete(listener);
  });
}

export async function disposeScenarios() {
  await act(async () => {
    for (const settle of outstanding) settle();
  });
  outstanding.clear();
  cleanup();
  if (vi.isFakeTimers()) {
    await vi.runOnlyPendingTimersAsync();
    vi.useRealTimers();
  }
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  sessionStorage.clear();
}

function createServerTransport(initial: FileTreeNode) {
  let server = structuredClone(initial);
  let holdReads = false;
  let failReads = false;
  const reads: Read[] = [];
  const accepted: string[] = [];
  const renames: Array<{
    old_path: string;
    new_path: string;
    reply: ReturnType<typeof deferred<RenameResponse>>;
    committed: boolean;
  }> = [];
  transport.request.mockImplementation(
    (action: string, args: { path?: string; old_path?: string; new_path?: string }) => {
      if (action === "workspace.tree.get") {
        if (failReads) return Promise.reject(new Error("tree unavailable"));
        const path = args.path ?? "";
        const snapshot = structuredClone(remoteNode(server, path)!);
        // The real depth-one endpoint returns child directories as placeholders.
        for (const child of snapshot.children ?? []) if (child.is_dir) delete child.children;
        if (!holdReads) return Promise.resolve({ root: snapshot });
        const reply = deferred<TreeResponse>();
        reads.push({ path, snapshot, reply });
        outstanding.add(() => reply.resolve({ root: snapshot }));
        return reply.promise;
      }
      if (action === "workspace.file.rename") {
        const reply = deferred<RenameResponse>();
        const old_path = args.old_path!;
        const new_path = args.new_path!;
        const rename = { old_path, new_path, reply, committed: false };
        renames.push(rename);
        outstanding.add(() => reply.resolve({ success: false }));
        return reply.promise.then((response) => {
          if (response.success && !rename.committed) {
            accepted.push(new_path);
            remoteRename(server, old_path, new_path);
          }
          return response;
        });
      }
      throw new Error(`Unconfigured external request: ${action}`);
    },
  );
  return {
    reads,
    accepted,
    renames,
    replaceServer: (tree: FileTreeNode) => {
      server = structuredClone(tree);
    },
    holdReads: (hold = true) => {
      holdReads = hold;
    },
    failReads: () => {
      failReads = true;
    },
    acceptRemotely(index: number) {
      const rename = renames[index];
      rename.committed = true;
      remoteRename(server, rename.old_path, rename.new_path);
      accepted.push(rename.new_path);
    },
  };
}

type ScenarioSession = ReturnType<typeof createSession>;

function createSession() {
  const id = `move-settlement-${++sequence}`;
  const session = {
    id: sessionId(id),
    task_id: taskId(`task-${id}`),
    environment_id: `env-${id}`,
    workspace_path: "/workspace",
    state: "WAITING_FOR_INPUT" as const,
    started_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:00Z",
  } satisfies TaskSession;
  return session;
}

function renderScenario(session: ScenarioSession) {
  const id = session.id;
  let cacheBinding!: ReturnType<typeof useFileTreeCacheBinding>;
  function CacheObserver() {
    cacheBinding = useFileTreeCacheBinding(
      session.environment_id,
      getFileBrowserResetKey({
        sessionId: id,
        environmentId: session.environment_id,
        worktreeCount: 0,
        inventoryRevision: workspaceInventoryRevision(undefined),
        workspaceFilesRefresh: 0,
      }),
    );
    return null;
  }
  let changeVisit!: (mounted: boolean, sessionId?: string) => void;
  function BrowserVisit() {
    const [visit, setVisit] = useState({ mounted: true, sessionId: id });
    changeVisit = (mounted, nextSessionId = id) =>
      setVisit({ mounted, sessionId: sessionId(nextSessionId) });
    const operations = useFileOperations(visit.sessionId);
    return visit.mounted ? (
      <FileBrowser
        sessionId={visit.sessionId}
        environmentId={session.environment_id}
        onOpenFile={() => {}}
        onRenameFile={operations.renameFile}
      />
    ) : null;
  }
  const view = render(
    <StateProvider
      initialState={{
        editors: { items: [], loaded: true, loading: false, folderOpeningAvailable: false },
        userSettings: { ...defaultState.userSettings, loaded: true },
        connection: { status: "connected", error: null, issueSeverity: "none" },
        taskSessions: { items: { [id]: session } },
        environmentIdBySessionId: { [id]: session.environment_id },
        sessionAgentctl: { itemsBySessionId: { [id]: { status: "ready" } } },
      }}
    >
      <ToastProvider>
        <TooltipProvider>
          <CacheObserver />
          <BrowserVisit />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  return {
    view,
    cachedTree: () => cacheBinding.cache.get(cacheBinding.key),
    changeVisit: (mounted: boolean, sessionId?: string) => changeVisit(mounted, sessionId),
  };
}

function scenarioActions(
  server: ReturnType<typeof createServerTransport>,
  rendered: ReturnType<typeof renderScenario>,
  id: string,
) {
  const { view } = rendered;
  const { renames } = server;
  const row = (path: string) =>
    view.container.querySelector<HTMLElement>(
      `[data-testid="file-tree-node"][data-path="${path}"]`,
    );
  return {
    ...rendered,
    ...server,
    id,
    row,
    async expand(path: string) {
      await act(async () => {
        fireEvent.click(row(path)!);
      });
    },
    paths: () =>
      Array.from(
        view.container.querySelectorAll<HTMLElement>("[data-testid='file-tree-node']"),
      ).map((node) => node.dataset.path!),
    async drag(paths = ["alpha.txt", "beta.txt"]) {
      for (const path of paths) {
        await act(async () => {
          fireEvent.click(row(path)!, { ctrlKey: true });
        });
      }
      await act(async () => {
        const dataTransfer = new DataTransfer();
        fireEvent.dragStart(row(paths[0])!, { dataTransfer });
        expect(JSON.parse(dataTransfer.getData("text/plain"))).toEqual(paths);
        fireEvent.drop(row("dest")!, { dataTransfer });
      });
      expect(renames).toHaveLength(paths.length);
    },
    async settle(index: number, outcome: boolean | Error) {
      await act(async () => {
        if (outcome instanceof Error) renames[index].reply.reject(outcome);
        else
          renames[index].reply.resolve({
            success: outcome,
            ...(outcome ? {} : { error: "rename refused" }),
          });
      });
    },
    async notify(paths = ["alpha.txt", "dest/alpha.txt"]) {
      expect(listeners.get("session.workspace.file.changes")?.size).toBe(1);
      await act(async () => {
        for (const listener of listeners.get("session.workspace.file.changes")!) {
          listener({
            payload: {
              session_id: id,
              changes: paths.map((path) => ({ path, operation: "rename" })),
            },
          });
        }
      });
    },
    async releaseReads(batch: Read[]) {
      await act(async () => {
        for (const read of batch) read.reply.resolve({ root: read.snapshot });
      });
    },
    async visit(mounted: boolean, sessionId?: string) {
      await act(async () => {
        rendered.changeVisit(mounted, sessionId);
      });
    },
  };
}

export async function startScenario(
  initial = folder("", [file("alpha.txt"), file("beta.txt"), folder("dest")]),
) {
  const session = createSession();
  transport.sessionRead.mockResolvedValue({ status: "modified", etag: null, data: { session } });
  const server = createServerTransport(initial);
  const rendered = renderScenario(session);
  const actions = scenarioActions(server, rendered, session.id);
  const { row } = actions;
  await waitFor(() => expect(row("dest")).not.toBeNull());
  await act(async () => {
    fireEvent.click(row("dest")!);
  });
  expect(
    transport.request.mock.calls.some(
      ([action, args]) => action === "workspace.tree.get" && args.path === "dest",
    ),
  ).toBe(true);
  // The initial and destination reads are settled before timers/selection/DnD.
  await act(async () => {});
  vi.useFakeTimers();
  return actions;
}
