import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type React from "react";
import type { FileTreeNode } from "@/lib/types/backend";
import { useFileTreeReveal } from "./file-tree-reveal";

const TARGET_PATH = "src/main/Greeting.kt";

function file(name: string, parentPath: string): FileTreeNode {
  return { name, path: `${parentPath}/${name}`, is_dir: false, size: 0 };
}

function dir(name: string, parentPath = "", children?: FileTreeNode[]): FileTreeNode {
  return {
    name,
    path: parentPath ? `${parentPath}/${name}` : name,
    is_dir: true,
    size: 0,
    children,
  };
}

function root(children: FileTreeNode[]): FileTreeNode {
  return { name: "", path: "", is_dir: true, size: 0, children };
}

function srcOnlyTree(): FileTreeNode {
  return root([dir("src", "", [])]);
}

function mainLoadedTree(includeTarget = false): FileTreeNode {
  return root([
    dir("src", "", [dir("main", "src", includeTarget ? [file("Greeting.kt", "src/main")] : [])]),
  ]);
}

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
}

function expansionState() {
  const expanded = { current: new Set<string>() };
  const setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>> = vi.fn((update) => {
    expanded.current = typeof update === "function" ? update(expanded.current) : update;
  });
  return { expanded, setExpandedPaths };
}

type HookProps = {
  activeFilePath: string;
  sessionId: string;
  tree: FileTreeNode | null;
};

afterEach(() => {
  vi.useRealTimers();
});

it("loads a missing ancestor chain one level at a time", async () => {
  const firstLoad = deferred<boolean>();
  const secondLoad = deferred<boolean>();
  const loadChildren = vi
    .fn<(node: FileTreeNode) => Promise<boolean>>()
    .mockReturnValueOnce(firstLoad.promise)
    .mockReturnValueOnce(secondLoad.promise);
  const { expanded, setExpandedPaths } = expansionState();
  const { rerender } = renderHook(
    ({ activeFilePath, sessionId, tree }: HookProps) =>
      useFileTreeReveal({
        activeFilePath,
        sessionId,
        tree,
        setExpandedPaths,
        isLoading: () => false,
        loadChildren,
      }),
    {
      initialProps: {
        activeFilePath: TARGET_PATH,
        sessionId: "session-1",
        tree: srcOnlyTree(),
      } as HookProps,
    },
  );

  expect(expanded.current).toEqual(new Set(["src", "src/main"]));
  expect(loadChildren.mock.calls.map(([node]) => node.path)).toEqual(["src"]);

  rerender({ activeFilePath: TARGET_PATH, sessionId: "session-1", tree: mainLoadedTree() });
  await act(async () => {
    firstLoad.resolve(true);
    await firstLoad.promise;
  });
  expect(loadChildren.mock.calls.map(([node]) => node.path)).toEqual(["src", "src/main"]);

  rerender({
    activeFilePath: TARGET_PATH,
    sessionId: "session-1",
    tree: mainLoadedTree(true),
  });
  await act(async () => {
    secondLoad.resolve(true);
    await secondLoad.promise;
  });
  expect(loadChildren).toHaveBeenCalledTimes(2);
});

it("waits before retrying a transient load failure", async () => {
  vi.useFakeTimers();
  const loadChildren = vi.fn<(node: FileTreeNode) => Promise<boolean>>().mockResolvedValue(false);
  const { setExpandedPaths } = expansionState();
  renderHook(() =>
    useFileTreeReveal({
      activeFilePath: TARGET_PATH,
      sessionId: "session-1",
      tree: srcOnlyTree(),
      setExpandedPaths,
      isLoading: () => false,
      loadChildren,
      retryDelaysMs: [100, 500],
    }),
  );
  await act(async () => Promise.resolve());
  expect(loadChildren).toHaveBeenCalledTimes(1);

  await act(async () => {
    vi.advanceTimersByTime(99);
    await Promise.resolve();
  });
  expect(loadChildren).toHaveBeenCalledTimes(1);

  await act(async () => {
    vi.advanceTimersByTime(1);
    await Promise.resolve();
  });
  expect(loadChildren).toHaveBeenCalledTimes(2);
});

it("starts a fresh reveal generation after the tree resets", async () => {
  const loadChildren = vi.fn<(node: FileTreeNode) => Promise<boolean>>().mockResolvedValue(false);
  const { setExpandedPaths } = expansionState();
  const { rerender } = renderHook(
    ({ activeFilePath, sessionId, tree }: HookProps) =>
      useFileTreeReveal({
        activeFilePath,
        sessionId,
        tree,
        setExpandedPaths,
        isLoading: () => false,
        loadChildren,
        retryDelaysMs: [],
      }),
    {
      initialProps: {
        activeFilePath: TARGET_PATH,
        sessionId: "session-1",
        tree: srcOnlyTree(),
      } as HookProps,
    },
  );
  await act(async () => Promise.resolve());
  expect(loadChildren).toHaveBeenCalledTimes(1);

  rerender({ activeFilePath: TARGET_PATH, sessionId: "session-1", tree: null });
  rerender({ activeFilePath: TARGET_PATH, sessionId: "session-1", tree: srcOnlyTree() });
  await act(async () => Promise.resolve());
  expect(loadChildren).toHaveBeenCalledTimes(2);
});

it("ignores a late completion after the target and session change", async () => {
  vi.useFakeTimers();
  const oldLoad = deferred<boolean>();
  const currentLoad = deferred<boolean>();
  const loadChildren = vi
    .fn<(node: FileTreeNode, shouldApply: () => boolean) => Promise<boolean>>()
    .mockReturnValueOnce(oldLoad.promise)
    .mockReturnValueOnce(currentLoad.promise);
  const { setExpandedPaths } = expansionState();
  const otherTarget = "other/Current.kt";
  const otherTree = root([dir("other", "", [])]);
  const { rerender } = renderHook(
    ({ activeFilePath, sessionId, tree }: HookProps) =>
      useFileTreeReveal({
        activeFilePath,
        sessionId,
        tree,
        setExpandedPaths,
        isLoading: () => false,
        loadChildren,
        retryDelaysMs: [100],
      }),
    {
      initialProps: { activeFilePath: TARGET_PATH, sessionId: "session-1", tree: srcOnlyTree() },
    },
  );

  rerender({ activeFilePath: otherTarget, sessionId: "session-2", tree: otherTree });
  expect(loadChildren.mock.calls.map(([node]) => node.path)).toEqual(["src", "other"]);
  expect(loadChildren.mock.calls[0][1]()).toBe(false);
  expect(loadChildren.mock.calls[1][1]()).toBe(true);
  await act(async () => {
    oldLoad.resolve(false);
    await oldLoad.promise;
    vi.advanceTimersByTime(1_000);
  });
  expect(loadChildren).toHaveBeenCalledTimes(2);

  rerender({
    activeFilePath: otherTarget,
    sessionId: "session-2",
    tree: root([dir("other", "", [file("Current.kt", "other")])]),
  });
  await act(async () => {
    currentLoad.resolve(true);
    await currentLoad.promise;
  });
});

it("does not reopen an active branch after the user collapses it", () => {
  const loadChildren = vi.fn<(node: FileTreeNode) => Promise<boolean>>();
  const { expanded, setExpandedPaths } = expansionState();
  const props = { activeFilePath: TARGET_PATH, sessionId: "session-1" };
  const { rerender } = renderHook(
    ({ tree }: { tree: FileTreeNode }) =>
      useFileTreeReveal({
        ...props,
        tree,
        setExpandedPaths,
        isLoading: () => false,
        loadChildren,
      }),
    { initialProps: { tree: mainLoadedTree(true) } },
  );
  expect(expanded.current).toEqual(new Set(["src", "src/main"]));

  expanded.current = new Set();
  vi.mocked(setExpandedPaths).mockClear();
  rerender({ tree: mainLoadedTree(true) });

  expect(expanded.current.size).toBe(0);
  expect(setExpandedPaths).not.toHaveBeenCalled();
});
