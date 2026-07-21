import { describe, it, expect, vi } from "vitest";
import { toggleFolderExpand } from "./file-browser-hooks";
import type { FileTreeNode } from "@/lib/types/backend";
import type { useFileBrowserTree } from "./file-browser-hooks";

type TreeState = ReturnType<typeof useFileBrowserTree>;

function makeTreeState(initialExpanded: Set<string> = new Set()): {
  state: TreeState;
  expanded: { current: Set<string> };
} {
  const expanded = { current: initialExpanded };
  const setExpandedPaths = vi.fn((updater: unknown) => {
    if (typeof updater === "function") {
      expanded.current = (updater as (p: Set<string>) => Set<string>)(expanded.current);
    } else {
      expanded.current = updater as Set<string>;
    }
  });
  const state = {
    tree: null,
    setTree: vi.fn(),
    get expandedPaths() {
      return expanded.current as ReadonlySet<string>;
    },
    setExpandedPaths,
    visibleRows: [],
    visibleLoadingPaths: new Set<string>(),
    isLoadingTree: false,
    loadState: "loaded",
    loadError: null,
    loadTree: vi.fn(),
    showLoading: vi.fn(),
    hideLoading: vi.fn(),
    isLoading: vi.fn(() => false),
    collapseAll: vi.fn(),
  } as unknown as TreeState;
  return { state, expanded };
}

const FOLDER: FileTreeNode = { name: "src", path: "src", is_dir: true, size: 0 };
const FILE: FileTreeNode = { name: "a.ts", path: "src/a.ts", is_dir: false, size: 0 };

describe("toggleFolderExpand", () => {
  it("expands a collapsed folder synchronously, before the async load resolves", async () => {
    const { state, expanded } = makeTreeState();
    let resolveLoad: () => void = () => {};
    const loadChildren = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const setActiveFolderPath = vi.fn();

    const promise = toggleFolderExpand({
      node: FOLDER,
      sessionId: "session-1",
      treeState: state,
      setActiveFolderPath,
      loadChildren,
    });

    // The expanded set must already include the folder even though loadChildren
    // has not resolved yet — that is the regression we are guarding against.
    expect(expanded.current.has("src")).toBe(true);
    expect(setActiveFolderPath).toHaveBeenCalledWith("src");
    expect(loadChildren).toHaveBeenCalledTimes(1);

    resolveLoad();
    await promise;
    expect(expanded.current.has("src")).toBe(true);
  });

  it("collapses an already-expanded folder and skips the async load", async () => {
    const { state, expanded } = makeTreeState(new Set(["src"]));
    const loadChildren = vi.fn(() => Promise.resolve());

    await toggleFolderExpand({
      node: FOLDER,
      sessionId: "session-1",
      treeState: state,
      setActiveFolderPath: vi.fn(),
      loadChildren,
    });

    expect(expanded.current.has("src")).toBe(false);
    expect(loadChildren).not.toHaveBeenCalled();
  });

  it("ignores file nodes", async () => {
    const { state, expanded } = makeTreeState();
    const loadChildren = vi.fn(() => Promise.resolve());
    const setActiveFolderPath = vi.fn();

    await toggleFolderExpand({
      node: FILE,
      sessionId: "session-1",
      treeState: state,
      setActiveFolderPath,
      loadChildren,
    });

    expect(expanded.current.size).toBe(0);
    expect(setActiveFolderPath).not.toHaveBeenCalled();
    expect(loadChildren).not.toHaveBeenCalled();
  });

  it("uses functional setState so the latest expanded set is mutated", async () => {
    const { state, expanded } = makeTreeState();
    const loadChildren = vi.fn(() => Promise.resolve());

    // Simulate another path being added while the toggle is in flight.
    const promise = toggleFolderExpand({
      node: FOLDER,
      sessionId: "session-1",
      treeState: state,
      setActiveFolderPath: vi.fn(),
      loadChildren,
    });
    // The toggle ran setExpandedPaths via a functional setter — adding another
    // path concurrently must not be lost.
    state.setExpandedPaths((prev) => new Set(prev).add("docs"));
    await promise;

    expect(expanded.current.has("src")).toBe(true);
    expect(expanded.current.has("docs")).toBe(true);
  });
});
