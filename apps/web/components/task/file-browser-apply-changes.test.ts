import { describe, it, expect, vi, beforeEach } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";

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

type ApplyFileChanges = (typeof import("./file-browser-hooks"))["applyFileChanges"];
let applyFileChanges: ApplyFileChanges;

const SESSION_ID = "sess";
const REFRESH_OP = "refresh";
const THM_OLD = "thm/old.txt";
const THM_NEW = "thm/new.txt";

beforeEach(async () => {
  vi.resetModules();
  requestFileTreeMock.mockReset();
  getWebSocketClientMock.mockReset().mockReturnValue({});
  ({ applyFileChanges } = await import("./file-browser-hooks"));
});

function makeTree(): FileTreeNode {
  return {
    name: "",
    path: "",
    is_dir: true,
    size: 0,
    children: [
      {
        name: "thm",
        path: "thm",
        is_dir: true,
        size: 0,
        children: [{ name: "old.txt", path: THM_OLD, is_dir: false, size: 0 }],
      },
      { name: "kandev", path: "kandev", is_dir: true, size: 0, children: [] },
    ],
  };
}

function mockEmptyTree() {
  // depth=1 responses omit `children` for placeholder nodes (omitempty).
  requestFileTreeMock.mockImplementation((_client: unknown, _sid: string, folder: string) =>
    Promise.resolve({ root: { name: folder, path: folder, is_dir: true } }),
  );
}

function client() {
  return {} as ReturnType<typeof import("@/lib/ws/connection").getWebSocketClient>;
}

// Regression for #982: refresh events must expand all affected-repo folders, not just root.
describe("applyFileChanges — refresh operation expands to all expanded folders", () => {
  it("refreshes every expanded folder under the affected repo on a refresh event", async () => {
    mockEmptyTree();
    applyFileChanges({
      client: client(),
      sessionId: SESSION_ID,
      expandedPaths: new Set(["thm", "thm/rooms", "kandev"]),
      changes: [{ path: "", operation: REFRESH_OP, repository_name: "thm" }],
      setTree: vi.fn(),
      setLoadState: vi.fn(),
    });
    await new Promise<void>((r) => setTimeout(r, 0));
    // Root + every expanded path under "thm" (not "kandev" — different repo).
    expect(requestFileTreeMock.mock.calls.map((c) => c[2]).sort()).toEqual([
      "",
      "thm",
      "thm/rooms",
    ]);
  });

  it("refreshes every expanded folder when repository_name is missing", async () => {
    mockEmptyTree();
    applyFileChanges({
      client: client(),
      sessionId: SESSION_ID,
      expandedPaths: new Set(["src", "src/components"]),
      changes: [{ path: "", operation: REFRESH_OP }],
      setTree: vi.fn(),
      setLoadState: vi.fn(),
    });
    await new Promise<void>((r) => setTimeout(r, 0));
    expect(requestFileTreeMock.mock.calls.map((c) => c[2]).sort()).toEqual([
      "",
      "src",
      "src/components",
    ]);
  });

  it("preserves the targeted-path behavior for specific operations", async () => {
    mockEmptyTree();
    applyFileChanges({
      client: client(),
      sessionId: SESSION_ID,
      expandedPaths: new Set(["thm", "kandev"]),
      changes: [{ path: THM_NEW, operation: "create" }],
      setTree: vi.fn(),
      setLoadState: vi.fn(),
    });
    await new Promise<void>((r) => setTimeout(r, 0));
    // create event: parent "thm" is expanded, path itself is not — only "thm" refreshed.
    expect(requestFileTreeMock.mock.calls.map((c) => c[2]).sort()).toEqual(["thm"]);
  });

  it("merges fresh children into the expanded subtree", async () => {
    requestFileTreeMock.mockImplementation((_c: unknown, _s: string, folder: string) =>
      folder === "thm"
        ? Promise.resolve({ root: thmChildrenAfter() })
        : Promise.resolve({ root: rootChildrenAfter() }),
    );
    const setTree = vi.fn();
    applyFileChanges({
      client: client(),
      sessionId: SESSION_ID,
      expandedPaths: new Set(["thm"]),
      changes: [{ path: "", operation: REFRESH_OP, repository_name: "thm" }],
      setTree,
      setLoadState: vi.fn(),
    });
    await new Promise<void>((r) => setTimeout(r, 0));
    expect(setTree).toHaveBeenCalled();
    // Apply setTree's reducer to the prev tree to inspect the merged shape.
    const reducer = setTree.mock.calls[0][0] as (prev: FileTreeNode) => FileTreeNode;
    const next = reducer(makeTree());
    const thmNode = next.children?.find((c) => c.path === "thm");
    expect(thmNode?.children?.map((c) => c.path).sort()).toEqual([THM_NEW, THM_OLD]);
  });
});

// Regression (#982): refresh scoped to one repo must not wipe sibling repos' loaded subtrees.
describe("applyFileChanges — cross-repo subtree preservation", () => {
  it("preserves kandev's loaded children when refresh is scoped to thm", async () => {
    requestFileTreeMock.mockImplementation((_c: unknown, _s: string, folder: string) => {
      if (folder === "thm") {
        return Promise.resolve({ root: thmChildrenAfter() });
      }
      return Promise.resolve({ root: rootChildrenAfter() });
    });
    const prevTree: FileTreeNode = {
      name: "",
      path: "",
      is_dir: true,
      size: 0,
      children: [
        {
          name: "kandev",
          path: "kandev",
          is_dir: true,
          size: 0,
          children: [{ name: "README.md", path: "kandev/README.md", is_dir: false, size: 0 }],
        },
        {
          name: "thm",
          path: "thm",
          is_dir: true,
          size: 0,
          children: [{ name: "old.txt", path: THM_OLD, is_dir: false, size: 0 }],
        },
      ],
    };
    const setTree = vi.fn();
    applyFileChanges({
      client: client(),
      sessionId: SESSION_ID,
      expandedPaths: new Set(["thm"]),
      changes: [{ path: "", operation: REFRESH_OP, repository_name: "thm" }],
      setTree,
      setLoadState: vi.fn(),
    });
    await new Promise<void>((r) => setTimeout(r, 0));
    const reducer = setTree.mock.calls[0][0] as (prev: FileTreeNode) => FileTreeNode;
    const next = reducer(prevTree);
    const kandevNode = next.children?.find((c) => c.path === "kandev");
    // kandev's existing child must survive a refresh scoped to a different repo.
    expect(kandevNode?.children?.map((c) => c.path)).toEqual(["kandev/README.md"]);
    const thmNode = next.children?.find((c) => c.path === "thm");
    expect(thmNode?.children?.map((c) => c.path).sort()).toEqual([THM_NEW, THM_OLD]);
  });
});

function thmChildrenAfter(): FileTreeNode {
  return {
    name: "thm",
    path: "thm",
    is_dir: true,
    size: 0,
    children: [
      { name: "old.txt", path: THM_OLD, is_dir: false, size: 0 },
      { name: "new.txt", path: THM_NEW, is_dir: false, size: 0 },
    ],
  };
}

function rootChildrenAfter(): FileTreeNode {
  // depth=1 backend responses omit `children` (omitempty) — mergeTreeNodes uses absence to preserve loaded subtrees.
  return {
    name: "",
    path: "",
    is_dir: true,
    size: 0,
    children: [
      { name: "thm", path: "thm", is_dir: true, size: 0 },
      { name: "kandev", path: "kandev", is_dir: true, size: 0 },
    ],
  };
}
