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
        children: [{ name: "old.txt", path: "thm/old.txt", is_dir: false, size: 0 }],
      },
      { name: "kandev", path: "kandev", is_dir: true, size: 0, children: [] },
    ],
  };
}

function mockEmptyTree() {
  requestFileTreeMock.mockImplementation((_client: unknown, _sid: string, folder: string) =>
    Promise.resolve({ root: { name: folder, path: folder, is_dir: true, children: [] } }),
  );
}

function client() {
  return {} as ReturnType<typeof import("@/lib/ws/connection").getWebSocketClient>;
}

// Polling-detected refresh events carry empty path + operation: "refresh".
// Before the fix, applyFileChanges only refreshed root, so files added under
// an expanded subdir never appeared in the tree (regression for #982).
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
      changes: [{ path: "thm/new.txt", operation: "create" }],
      setTree: vi.fn(),
      setLoadState: vi.fn(),
    });
    await new Promise<void>((r) => setTimeout(r, 0));
    // create event for "thm/new.txt" — parent is "thm" (expanded), the path
    // itself isn't expanded. Only "thm" should be refreshed.
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
    expect(thmNode?.children?.map((c) => c.path).sort()).toEqual(["thm/new.txt", "thm/old.txt"]);
  });
});

function thmChildrenAfter(): FileTreeNode {
  return {
    name: "thm",
    path: "thm",
    is_dir: true,
    size: 0,
    children: [
      { name: "old.txt", path: "thm/old.txt", is_dir: false, size: 0 },
      { name: "new.txt", path: "thm/new.txt", is_dir: false, size: 0 },
    ],
  };
}

function rootChildrenAfter(): FileTreeNode {
  return {
    name: "",
    path: "",
    is_dir: true,
    size: 0,
    children: [
      { name: "thm", path: "thm", is_dir: true, size: 0, children: [] },
      { name: "kandev", path: "kandev", is_dir: true, size: 0, children: [] },
    ],
  };
}
