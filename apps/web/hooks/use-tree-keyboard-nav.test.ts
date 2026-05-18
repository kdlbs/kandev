import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useTree } from "./use-tree";
import { useTreeKeyboardNav } from "./use-tree-keyboard-nav";

interface TestNode {
  path: string;
  isDir: boolean;
  children?: TestNode[];
}

const getPath = (n: TestNode) => n.path;
const getChildren = (n: TestNode) => n.children;
const isDir = (n: TestNode) => n.isDir;

const SRC = "src";
const SRC_UI = "src/ui";
const SRC_API = "src/api";
const BUTTON = "src/ui/Button.tsx";
const CARD = "src/ui/Card.tsx";
const USERS = "src/api/users.ts";
const README = "README.md";

const TREE: TestNode[] = [
  {
    path: SRC,
    isDir: true,
    children: [
      {
        path: SRC_UI,
        isDir: true,
        children: [
          { path: BUTTON, isDir: false },
          { path: CARD, isDir: false },
        ],
      },
      {
        path: SRC_API,
        isDir: true,
        children: [{ path: USERS, isDir: false }],
      },
    ],
  },
  { path: README, isDir: false },
];

/**
 * Renders useTree + useTreeKeyboardNav together. Use the returned `result`
 * to drive arrow keys and read focused-path state.
 */
function setup(defaultExpanded?: "all" | string[]) {
  const onActivate = vi.fn();
  const { result } = renderHook(() => {
    const tree = useTree<TestNode>({
      nodes: TREE,
      getPath,
      getChildren,
      isDir,
      defaultExpanded,
    });
    const nav = useTreeKeyboardNav<TestNode>({
      visibleRows: tree.visibleRows,
      toggle: tree.toggle,
      expand: tree.expand,
      collapse: tree.collapse,
      isExpanded: tree.isExpanded,
      onActivate,
    });
    return { tree, nav };
  });
  return { result, onActivate };
}

describe("useTreeKeyboardNav", () => {
  it("ArrowDown moves focus to the first row when nothing is focused", () => {
    const { result } = setup();
    act(() => result.current.nav.dispatchKey("ArrowDown"));
    expect(result.current.nav.focusedPath).toBe(SRC);
  });

  it("ArrowDown advances by one visible row, ArrowUp goes back", () => {
    const { result } = setup("all");
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src/ui
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src/ui/Button.tsx
    expect(result.current.nav.focusedPath).toBe(BUTTON);
    act(() => result.current.nav.dispatchKey("ArrowUp"));
    expect(result.current.nav.focusedPath).toBe(SRC_UI);
  });

  it("ArrowDown clamps at the last row", () => {
    const { result } = setup();
    // 2 rows visible: src, README.md
    act(() => result.current.nav.dispatchKey("ArrowDown"));
    act(() => result.current.nav.dispatchKey("ArrowDown"));
    act(() => result.current.nav.dispatchKey("ArrowDown"));
    expect(result.current.nav.focusedPath).toBe(README);
  });

  it("ArrowUp clamps at the first row", () => {
    const { result } = setup();
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src
    act(() => result.current.nav.dispatchKey("ArrowUp"));
    act(() => result.current.nav.dispatchKey("ArrowUp"));
    expect(result.current.nav.focusedPath).toBe(SRC);
  });

  it("ArrowRight on a collapsed dir expands it without moving focus", () => {
    const { result } = setup();
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src
    act(() => result.current.nav.dispatchKey("ArrowRight"));
    expect(result.current.tree.isExpanded(SRC)).toBe(true);
    expect(result.current.nav.focusedPath).toBe(SRC);
  });

  it("ArrowRight on an expanded dir moves focus to the next row", () => {
    const { result } = setup("all");
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src
    act(() => result.current.nav.dispatchKey("ArrowRight"));
    expect(result.current.nav.focusedPath).toBe(SRC_UI);
  });

  it("ArrowLeft on an expanded dir collapses it", () => {
    const { result } = setup("all");
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src
    act(() => result.current.nav.dispatchKey("ArrowLeft"));
    expect(result.current.tree.isExpanded(SRC)).toBe(false);
    expect(result.current.nav.focusedPath).toBe(SRC);
  });

  it("ArrowLeft on a leaf moves focus to its parent dir", () => {
    const { result } = setup("all");
    // Walk down to Button.tsx
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src/ui
    act(() => result.current.nav.dispatchKey("ArrowDown")); // Button.tsx
    expect(result.current.nav.focusedPath).toBe(BUTTON);
    act(() => result.current.nav.dispatchKey("ArrowLeft"));
    expect(result.current.nav.focusedPath).toBe(SRC_UI);
  });

  it("Enter on a dir toggles it; Enter on a file fires onActivate", () => {
    const { result, onActivate } = setup("all");
    act(() => result.current.nav.dispatchKey("ArrowDown")); // src
    act(() => result.current.nav.dispatchKey("Enter"));
    expect(result.current.tree.isExpanded(SRC)).toBe(false);

    // Re-expand and walk to README.
    act(() => result.current.tree.expandAll());
    // Focus on README (last row)
    act(() => result.current.nav.setFocusedPath(README));
    act(() => result.current.nav.dispatchKey("Enter"));
    expect(onActivate).toHaveBeenCalledTimes(1);
    expect(onActivate.mock.calls[0][0].path).toBe(README);
  });

  it("Space activates the same way as Enter", () => {
    const { result, onActivate } = setup("all");
    act(() => result.current.nav.setFocusedPath(BUTTON));
    act(() => result.current.nav.dispatchKey(" "));
    expect(onActivate).toHaveBeenCalledTimes(1);
    expect(onActivate.mock.calls[0][0].path).toBe(BUTTON);
  });

  it("ignores unrelated keys and reports no effect", () => {
    const { result, onActivate } = setup();
    act(() => result.current.nav.setFocusedPath(SRC));
    act(() => result.current.nav.dispatchKey("a"));
    expect(onActivate).not.toHaveBeenCalled();
    expect(result.current.nav.focusedPath).toBe(SRC);
  });
});
