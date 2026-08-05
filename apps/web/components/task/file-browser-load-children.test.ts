import { it, expect, vi, beforeEach } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";
import type {
  useFileBrowserTree,
  loadNodeChildren as LoadNodeChildren,
} from "./file-browser-hooks";

type TreeState = ReturnType<typeof useFileBrowserTree>;

const requestFileTreeMock = vi.fn();
const getWebSocketClientMock = vi.fn(() => ({}) as unknown);

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => getWebSocketClientMock(),
}));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileTree: (...args: unknown[]) => requestFileTreeMock(...args),
  requestFileContent: vi.fn(),
  searchWorkspaceFiles: vi.fn(),
}));

let loadNodeChildren: typeof LoadNodeChildren;
beforeEach(async () => {
  vi.resetModules();
  requestFileTreeMock.mockReset();
  getWebSocketClientMock.mockReset().mockReturnValue({});
  ({ loadNodeChildren } = await import("./file-browser-hooks"));
});

function makeTreeState(opts: { tree?: FileTreeNode | null; loadingPaths?: Set<string> } = {}): {
  state: TreeState;
  loadingPaths: Set<string>;
} {
  const loadingPaths = opts.loadingPaths ?? new Set<string>();
  const state = {
    tree: opts.tree ?? { name: "", path: "", is_dir: true, size: 0, children: [] },
    setTree: vi.fn(),
    expandedPaths: new Set<string>(),
    setExpandedPaths: vi.fn(),
    visibleRows: [],
    visibleLoadingPaths: new Set<string>(),
    isLoadingTree: false,
    loadState: "loaded",
    loadError: null,
    loadTree: vi.fn(),
    showLoading: vi.fn((p: string) => loadingPaths.add(p)),
    hideLoading: vi.fn((p: string) => loadingPaths.delete(p)),
    isLoading: vi.fn((p: string) => loadingPaths.has(p)),
    collapseAll: vi.fn(),
  } as unknown as TreeState;
  return { state, loadingPaths };
}

const FOLDER: FileTreeNode = { name: "src", path: "src", is_dir: true, size: 0, children: [] };

it("skips duplicate fetches while a load is in flight for the same path", async () => {
  const { state } = makeTreeState();
  let resolveFetch: (v: { root: FileTreeNode }) => void = () => {};
  requestFileTreeMock.mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
  );

  const p1 = loadNodeChildren(FOLDER, "session-1", state);
  const p2 = loadNodeChildren(FOLDER, "session-1", state);
  const p3 = loadNodeChildren(FOLDER, "session-1", state);

  // Only the first call should reach the WS request — the dedupe guard
  // catches the rest before they fire.
  expect(requestFileTreeMock).toHaveBeenCalledTimes(1);
  expect(state.showLoading).toHaveBeenCalledTimes(1);

  resolveFetch({ root: { name: "src", path: "src", is_dir: true, size: 0, children: [] } });
  await Promise.all([p1, p2, p3]);

  expect(state.hideLoading).toHaveBeenCalledTimes(1);
});

it("allows a fresh fetch once the previous one has finished", async () => {
  const { state } = makeTreeState();
  requestFileTreeMock.mockResolvedValue({
    root: { name: "src", path: "src", is_dir: true, size: 0, children: [] },
  });

  await loadNodeChildren(FOLDER, "session-1", state);
  await loadNodeChildren(FOLDER, "session-1", state);

  // First call clears showLoading/hideLoading state, so the second one
  // (still operating on a node with no children) issues its own fetch.
  expect(requestFileTreeMock).toHaveBeenCalledTimes(2);
});

it("short-circuits when the node already has children", async () => {
  const { state } = makeTreeState();
  const loaded: FileTreeNode = {
    ...FOLDER,
    children: [{ name: "a.ts", path: "src/a.ts", is_dir: false, size: 0 }],
  };

  await loadNodeChildren(loaded, "session-1", state);

  expect(requestFileTreeMock).not.toHaveBeenCalled();
  expect(state.showLoading).not.toHaveBeenCalled();
});

it("merges loaded children into the latest tree state", async () => {
  const initialTree: FileTreeNode = {
    name: "",
    path: "",
    is_dir: true,
    children: [FOLDER],
  };
  const latestTree: FileTreeNode = {
    ...initialTree,
    children: [FOLDER, { name: "README.md", path: "README.md", is_dir: false, size: 0 }],
  };
  const { state } = makeTreeState({ tree: initialTree });
  let updatedTree: FileTreeNode | null = latestTree;
  state.setTree = vi.fn((update) => {
    updatedTree = typeof update === "function" ? update(updatedTree) : update;
  });
  requestFileTreeMock.mockResolvedValue({
    root: {
      ...FOLDER,
      children: [{ name: "main.ts", path: "src/main.ts", is_dir: false, size: 0 }],
    },
  });

  await expect(loadNodeChildren(FOLDER, "session-1", state)).resolves.toBe(true);

  expect(state.setTree).toHaveBeenCalledWith(expect.any(Function));
  expect(updatedTree?.children?.map((node) => node.path)).toEqual(["src", "README.md"]);
  expect(updatedTree?.children?.[0].children?.[0].path).toBe("src/main.ts");
});

it("force-refreshes a populated folder when an expected child is missing", async () => {
  const existingFile: FileTreeNode = {
    name: "existing.ts",
    path: "src/existing.ts",
    is_dir: false,
    size: 0,
  };
  const populatedFolder: FileTreeNode = {
    ...FOLDER,
    children: [existingFile],
  };
  const tree = { name: "", path: "", is_dir: true, size: 0, children: [populatedFolder] };
  const { state } = makeTreeState({ tree });
  requestFileTreeMock.mockResolvedValue({
    root: {
      ...FOLDER,
      children: [
        existingFile,
        { name: "target.ts", path: "src/target.ts", is_dir: false, size: 0 },
      ],
    },
  });

  await expect(
    loadNodeChildren(populatedFolder, "session-1", state, { force: true }),
  ).resolves.toBe(true);

  expect(requestFileTreeMock).toHaveBeenCalledTimes(1);
  expect(state.setTree).toHaveBeenCalledWith(expect.any(Function));
});

it("does not apply a late response to a replacement tree", async () => {
  const initialTree: FileTreeNode = {
    name: "",
    path: "",
    is_dir: true,
    children: [FOLDER],
  };
  const replacementTree: FileTreeNode = {
    name: "",
    path: "",
    is_dir: true,
    children: [{ name: "other", path: "other", is_dir: true, size: 0, children: [] }],
  };
  const { state } = makeTreeState({ tree: initialTree });
  let currentTree: FileTreeNode | null = replacementTree;
  state.setTree = vi.fn((update) => {
    currentTree = typeof update === "function" ? update(currentTree) : update;
  });
  let resolveFetch: (value: { root: FileTreeNode }) => void = () => {};
  requestFileTreeMock.mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
  );
  let generationIsCurrent = true;

  const load = loadNodeChildren(FOLDER, "session-1", state, {
    force: true,
    shouldApply: () => generationIsCurrent,
  });
  generationIsCurrent = false;
  resolveFetch({
    root: {
      ...FOLDER,
      children: [{ name: "stale.ts", path: "src/stale.ts", is_dir: false, size: 0 }],
    },
  });
  await load;

  expect(currentTree).toBe(replacementTree);
});

it("reports a failed fetch so automatic reveal can retry", async () => {
  const { state } = makeTreeState();
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  requestFileTreeMock.mockRejectedValue(new Error("temporary failure"));

  await expect(loadNodeChildren(FOLDER, "session-1", state)).resolves.toBe(false);

  expect(state.hideLoading).toHaveBeenCalledWith(FOLDER.path);
  consoleError.mockRestore();
});
