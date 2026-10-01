import type { Dispatch, SetStateAction } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  on: vi.fn(),
  listeners: new Set<(msg: unknown) => void>(),
}));
const client = { on: mocks.on };
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => client }));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileTree: mocks.request,
  searchWorkspaceFiles: vi.fn(),
  requestFileContent: vi.fn(),
}));
vi.mock("@/hooks/domains/session/use-session-agentctl", () => ({
  useSessionAgentctl: () => ({ isReady: true }),
}));
import { FolderRefreshes } from "./file-browser-refresh";
import { applyFileChanges, useFileBrowserTree } from "./file-browser-hooks";

const node = (path: string, children?: FileTreeNode[]): FileTreeNode => ({
  name: path.split("/").pop() ?? "",
  path,
  is_dir: children !== undefined || !path.includes("."),
  size: 0,
  ...(children === undefined ? {} : { children }),
});
const SRC_CURRENT = "src/current.ts";
const OTHER_CURRENT = "other/current.ts";
const INITIAL = node("", [
  node("src", [node("src/deep", [node("src/deep/keep.ts")])]),
  node("other", [node("other/keep.ts")]),
]);
function deferred() {
  let resolve!: (value: { root: FileTreeNode }) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<{ root: FileTreeNode }>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}
async function mount() {
  const hook = renderHook(({ id }) => useFileBrowserTree(id), { initialProps: { id: "A" } });
  await waitFor(() => expect(hook.result.current.loadState).toBe("loaded"));
  mocks.request.mockReset();
  return hook;
}
function emit(paths: string[]) {
  act(() => {
    for (const listener of mocks.listeners)
      listener({ payload: { changes: paths.map((path) => ({ path, operation: "create" })) } });
  });
}
const paths = (tree: FileTreeNode | null, folder = ""): string[] | undefined => {
  if (!tree) return undefined;
  if (tree.path === folder) return tree.children?.map((child) => child.path);
  for (const child of tree.children ?? []) {
    const result = paths(child, folder);
    if (result) return result;
  }
};
beforeEach(() => {
  mocks.listeners.clear();
  mocks.request.mockReset();
  mocks.on.mockReset();
  mocks.on.mockImplementation((_event: string, listener: (msg: unknown) => void) => {
    mocks.listeners.add(listener);
    return () => mocks.listeners.delete(listener);
  });
  sessionStorage.clear();
  for (const id of ["A", "B"])
    sessionStorage.setItem(
      `kandev.filesPanel.expanded.${id}`,
      JSON.stringify(["src", "src/deep", "other"]),
    );
  mocks.request.mockImplementation((_client: unknown, _id: string, path: string) => {
    const find = (current: FileTreeNode): FileTreeNode | undefined =>
      current.path === path ? current : current.children?.map(find).find(Boolean);
    return Promise.resolve({ root: find(INITIAL) });
  });
});
afterEach(cleanup);

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4
// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
describe("Files file-watch refresh publication", () => {
  it("does not resurrect a file from an older root snapshot", async () => {
    const hook = await mount();
    const old = deferred();
    const current = deferred();
    mocks.request.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    emit(["ghost.ts"]);
    emit(["ghost.ts"]);
    await act(async () => current.resolve({ root: node("", []) }));
    await act(async () => old.resolve({ root: node("", [node("ghost.ts")]) }));
    expect(paths(hook.result.current.tree)).toEqual([]);
  });

  it("keeps independent siblings when they resolve in reverse order", async () => {
    const hook = await mount();
    const src = deferred();
    const other = deferred();
    mocks.request.mockReturnValueOnce(src.promise).mockReturnValueOnce(other.promise);
    emit([SRC_CURRENT]);
    emit([OTHER_CURRENT]);
    await act(async () => other.resolve({ root: node("other", [node(OTHER_CURRENT)]) }));
    await act(async () => src.resolve({ root: node("src", [node(SRC_CURRENT)]) }));
    expect(paths(hook.result.current.tree, "src")).toEqual([SRC_CURRENT]);
    expect(paths(hook.result.current.tree, "other")).toEqual([OTHER_CURRENT]);
  });

  it("accepts current siblings in a batch with a superseded folder", async () => {
    const hook = await mount();
    const oldSrc = deferred();
    const other = deferred();
    const src = deferred();
    mocks.request
      .mockReturnValueOnce(oldSrc.promise)
      .mockReturnValueOnce(other.promise)
      .mockReturnValueOnce(src.promise);
    emit(["src/old.ts", OTHER_CURRENT]);
    emit([SRC_CURRENT]);
    await act(async () => src.resolve({ root: node("src", [node(SRC_CURRENT)]) }));
    await act(async () => oldSrc.resolve({ root: node("src", [node("src/old.ts")]) }));
    await act(async () => other.resolve({ root: node("other", [node(OTHER_CURRENT)]) }));
    expect(paths(hook.result.current.tree, "src")).toEqual([SRC_CURRENT]);
    expect(paths(hook.result.current.tree, "other")).toEqual([OTHER_CURRENT]);
  });

  it("preserves loaded descendants below subfolder placeholders", async () => {
    const hook = await mount();
    mocks.request.mockResolvedValue({
      root: node("src", [node("src/deep"), node(SRC_CURRENT)]),
    });
    emit([SRC_CURRENT]);
    await waitFor(() => expect(paths(hook.result.current.tree, "src")).toContain(SRC_CURRENT));
    expect(paths(hook.result.current.tree, "src/deep")).toEqual(["src/deep/keep.ts"]);
    expect(hook.result.current.tree?.children?.find((child) => child.path === "src")?.name).toBe(
      "src",
    );
  });

  it("clears authoritative omitted children while retaining a sibling", async () => {
    const hook = await mount();
    mocks.request.mockResolvedValue({ root: node("src") });
    emit(["src/gone.ts"]);
    await waitFor(() => expect(paths(hook.result.current.tree, "src") ?? []).toEqual([]));
    expect(paths(hook.result.current.tree, "other")).toEqual(["other/keep.ts"]);
  });

  it("does not recreate a parent removed by a current root", async () => {
    const hook = await mount();
    const src = deferred();
    const root = deferred();
    mocks.request.mockReturnValueOnce(src.promise).mockReturnValueOnce(root.promise);
    emit(["src/new.ts"]);
    emit(["top.ts"]);
    await act(async () => root.resolve({ root: node("", [node("other")]) }));
    await act(async () => src.resolve({ root: node("src", [node("src/new.ts")]) }));
    expect(paths(hook.result.current.tree)).toEqual(["other"]);
  });

  it.each([false, true])(
    "retires pending replies across session switch, return=%s",
    async (returnToA) => {
      const hook = await mount();
      const old = deferred();
      mocks.request.mockReturnValueOnce(old.promise);
      emit(["ghost.ts"]);
      mocks.request.mockImplementation(() =>
        Promise.resolve({ root: node("", [node("replacement.ts")]) }),
      );
      hook.rerender({ id: "B" });
      if (returnToA) hook.rerender({ id: "A" });
      await waitFor(() => expect(hook.result.current.loadState).toBe("loaded"));
      await act(async () => old.resolve({ root: node("", [node("ghost.ts")]) }));
      expect(paths(hook.result.current.tree)).toEqual(["replacement.ts"]);
    },
  );

  it("does not use a failed newer refresh as permission for an older reply", async () => {
    const hook = await mount();
    const old = deferred();
    const current = deferred();
    mocks.request.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    emit(["ghost.ts"]);
    emit(["ghost.ts"]);
    await act(async () => current.reject(new Error("temporary failure")));
    await act(async () => old.resolve({ root: node("", [node("ghost.ts")]) }));
    expect(paths(hook.result.current.tree)).toEqual(["src", "other"]);
  });
});

it("rejects a queued updater when the owner retires", async () => {
  const reply = deferred();
  mocks.request.mockReturnValueOnce(reply.promise);
  const updates: SetStateAction<FileTreeNode | null>[] = [];
  const setTree: Dispatch<SetStateAction<FileTreeNode | null>> = (update) => updates.push(update);
  let current = true;
  const setLoadState = vi.fn();
  applyFileChanges({
    client: client as never,
    sessionId: "A",
    expandedPaths: new Set(),
    changes: [{ path: "ghost.ts" }],
    setTree,
    setLoadState,
    isCurrent: () => current,
  });
  await act(async () => reply.resolve({ root: node("", [node("ghost.ts")]) }));
  current = false;
  expect((updates[0] as (tree: FileTreeNode) => FileTreeNode)(INITIAL)).toBe(INITIAL);
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
it("rejects superseded tree and load-state updaters queued before the next request", async () => {
  const refreshes = new FolderRefreshes();
  const old = deferred();
  const current = deferred();
  mocks.request.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
  const trees: SetStateAction<FileTreeNode | null>[] = [];
  const states: SetStateAction<import("./file-browser-hooks").LoadState>[] = [];
  const ctx = {
    client: client as never,
    sessionId: "A",
    expandedPaths: new Set<string>(),
    changes: [{ path: "queued.ts" }],
    refreshes,
    setTree: (update: SetStateAction<FileTreeNode | null>) => trees.push(update),
    setLoadState: (update: SetStateAction<import("./file-browser-hooks").LoadState>) =>
      states.push(update),
  };
  applyFileChanges(ctx);
  await act(async () => old.resolve({ root: node("", [node("queued.ts")]) }));
  // A React commit that has not consumed these updaters must not release their tokens.
  refreshes.committed();
  applyFileChanges(ctx);
  expect((trees[0] as (tree: FileTreeNode) => FileTreeNode)(INITIAL)).toBe(INITIAL);
  expect((states[0] as (state: string) => string)("waiting")).toBe("waiting");
  await act(async () => current.resolve({ root: node("", []) }));
  expect((trees[1] as (tree: FileTreeNode) => FileTreeNode)(INITIAL).children).toEqual([]);
  expect((states[1] as (state: string) => string)("waiting")).toBe("loaded");
});

it("releases committed refresh history and retires outstanding folder tokens", () => {
  const refreshes = new FolderRefreshes();
  // Inspect retained bookkeeping to prove the memory bound independently of tree output.
  const retained = (refreshes as unknown as { latest: Map<string, unknown> }).latest;
  for (let index = 0; index < 1000; index++) {
    const token = refreshes.begin(`folder-${index}`);
    token.treeApplied = true;
    token.stateApplied = true;
    refreshes.committed();
  }
  expect(retained.size).toBe(0);
  const pending = refreshes.begin("pending");
  refreshes.committed();
  expect(retained.size).toBe(1);
  refreshes.retire();
  expect(pending.current).toBe(false);
  expect(retained.size).toBe(0);
});
