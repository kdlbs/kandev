import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useTree } from "./use-tree";

interface TestNode {
  path: string;
  isDir: boolean;
  children?: TestNode[];
}

const getPath = (n: TestNode) => n.path;
const getChildren = (n: TestNode) => n.children;
const isDir = (n: TestNode) => n.isDir;

function leaf(path: string): TestNode {
  return { path, isDir: false };
}
function dir(path: string, children: TestNode[] = []): TestNode {
  return { path, isDir: true, children };
}

const SRC = "src";
const SRC_UI = "src/ui";
const SRC_API = "src/api";
const BUTTON_TSX = "src/ui/Button.tsx";
const CARD_TSX = "src/ui/Card.tsx";
const INDEX_TS = "src/index.ts";
const USERS_TS = "src/api/users.ts";
const README = "README.md";

const TREE: TestNode[] = [
  dir(SRC, [dir(SRC_UI, [leaf(BUTTON_TSX), leaf(CARD_TSX)]), leaf(INDEX_TS)]),
  leaf(README),
];

type Override = Partial<Parameters<typeof useTree<TestNode>>[0]>;

function renderUseTree(override: Override = {}) {
  return renderHook(
    (props: Override) => useTree<TestNode>({ nodes: TREE, getPath, getChildren, isDir, ...props }),
    { initialProps: override },
  ).result;
}

describe("useTree — visible rows and expansion", () => {
  it("renders only top-level rows when nothing is expanded", () => {
    const r = renderUseTree();
    expect(r.current.visibleRows.map((row) => row.path)).toEqual([SRC, README]);
  });

  it("toggle reveals a dir's children", () => {
    const r = renderUseTree();
    act(() => r.current.toggle(SRC));
    expect(r.current.visibleRows.map((row) => row.path)).toEqual([SRC, SRC_UI, INDEX_TS, README]);
  });

  it("toggle collapses an already-expanded dir", () => {
    const r = renderUseTree();
    act(() => r.current.toggle(SRC));
    act(() => r.current.toggle(SRC));
    expect(r.current.visibleRows.map((row) => row.path)).toEqual([SRC, README]);
  });

  it("expand and collapse are idempotent and explicit", () => {
    const r = renderUseTree();
    act(() => r.current.expand(SRC));
    act(() => r.current.expand(SRC));
    expect(r.current.isExpanded(SRC)).toBe(true);
    act(() => r.current.collapse(SRC));
    act(() => r.current.collapse(SRC));
    expect(r.current.isExpanded(SRC)).toBe(false);
  });

  it("defaultExpanded='all' pre-expands every dir", () => {
    const r = renderUseTree({ defaultExpanded: "all" });
    expect(r.current.visibleRows.map((row) => row.path)).toEqual([
      SRC,
      SRC_UI,
      BUTTON_TSX,
      CARD_TSX,
      INDEX_TS,
      README,
    ]);
  });

  it("defaultExpanded as iterable seeds the expanded set", () => {
    const r = renderUseTree({ defaultExpanded: [SRC] });
    expect(r.current.isExpanded(SRC)).toBe(true);
    expect(r.current.isExpanded(SRC_UI)).toBe(false);
  });

  it("expandAll walks the current nodes once", () => {
    const r = renderUseTree();
    act(() => r.current.expandAll());
    expect(r.current.isExpanded(SRC)).toBe(true);
    expect(r.current.isExpanded(SRC_UI)).toBe(true);
  });

  it("collapseAll clears the expanded set", () => {
    const r = renderUseTree({ defaultExpanded: "all" });
    act(() => r.current.collapseAll());
    expect(r.current.expanded.size).toBe(0);
  });

  it("expandAncestorsOf adds every ancestor prefix", () => {
    const r = renderUseTree();
    act(() => r.current.expandAncestorsOf(BUTTON_TSX));
    expect(r.current.isExpanded(SRC)).toBe(true);
    expect(r.current.isExpanded(SRC_UI)).toBe(true);
    expect(r.current.isExpanded(BUTTON_TSX)).toBe(false);
  });
});

describe("useTree — visible-row depth and metadata", () => {
  it("depth is 0 for roots and increments for each level", () => {
    const r = renderUseTree({ defaultExpanded: "all" });
    const byPath = Object.fromEntries(r.current.visibleRows.map((row) => [row.path, row.depth]));
    expect(byPath[SRC]).toBe(0);
    expect(byPath[SRC_UI]).toBe(1);
    expect(byPath[BUTTON_TSX]).toBe(2);
    expect(byPath[README]).toBe(0);
  });

  it("displayName for non-chain-collapsed rows is the last path segment", () => {
    const r = renderUseTree({ defaultExpanded: "all" });
    const byPath = Object.fromEntries(
      r.current.visibleRows.map((row) => [row.path, row.displayName]),
    );
    expect(byPath[SRC]).toBe(SRC);
    expect(byPath[BUTTON_TSX]).toBe("Button.tsx");
  });
});

describe("useTree — chain collapse", () => {
  it("merges single-child dir chains into one row with a slash-joined displayName", () => {
    const nodes: TestNode[] = [dir("a", [dir("a/b", [dir("a/b/c", [leaf("a/b/c/file.ts")])])])];
    const r = renderHook(() =>
      useTree({ nodes, getPath, getChildren, isDir, chainCollapse: true }),
    ).result;
    // Top-level row is "a/b/c" (the whole chain collapsed). It is not expanded
    // yet, so the leaf below is hidden.
    expect(r.current.visibleRows.map((row) => ({ path: row.path, name: row.displayName }))).toEqual(
      [{ path: "a/b/c", name: "a/b/c" }],
    );
  });

  it("does NOT merge when a chain member has multiple children", () => {
    const nodes: TestNode[] = [dir("a", [dir("a/b", [leaf("a/b/x.ts"), leaf("a/b/y.ts")])])];
    const r = renderHook(() =>
      useTree({
        nodes,
        getPath,
        getChildren,
        isDir,
        chainCollapse: true,
        defaultExpanded: "all",
      }),
    ).result;
    // "a" has one child dir "a/b" — that chain merges. But "a/b" has two
    // leaf children, so the chain stops there.
    const paths = r.current.visibleRows.map((row) => row.path);
    expect(paths).toEqual(["a/b", "a/b/x.ts", "a/b/y.ts"]);
    expect(r.current.visibleRows[0].displayName).toBe("a/b");
  });

  it("expanding a chain-collapsed row uses the deepest node's path", () => {
    const nodes: TestNode[] = [dir("a", [dir("a/b", [leaf("a/b/file.ts")])])];
    const r = renderHook(() =>
      useTree({ nodes, getPath, getChildren, isDir, chainCollapse: true }),
    ).result;
    // Toggling the chain-row's path "a/b" must expand the deepest dir.
    act(() => r.current.toggle("a/b"));
    const paths = r.current.visibleRows.map((row) => row.path);
    expect(paths).toEqual(["a/b", "a/b/file.ts"]);
  });
});

describe("useTree — search modes", () => {
  const SEARCH_TREE: TestNode[] = [
    dir(SRC, [dir(SRC_UI, [leaf(BUTTON_TSX), leaf(CARD_TSX)]), dir(SRC_API, [leaf(USERS_TS)])]),
    leaf(README),
  ];

  it("hide mode: removes non-matching subtrees and force-expands ancestors of matches", () => {
    const r = renderHook(() =>
      useTree({
        nodes: SEARCH_TREE,
        getPath,
        getChildren,
        isDir,
        search: "button",
        searchMode: "hide",
      }),
    ).result;
    const paths = r.current.visibleRows.map((row) => row.path);
    // src and src/ui are visible because they have a matching descendant;
    // src/api and README.md are hidden because nothing under them matches.
    // Ancestors are force-expanded so Button.tsx is reachable.
    expect(paths).toEqual([SRC, SRC_UI, BUTTON_TSX]);
  });

  it("expand mode: keeps all rows visible, force-expands matching dirs and ancestors", () => {
    const r = renderHook(() =>
      useTree({
        nodes: SEARCH_TREE,
        getPath,
        getChildren,
        isDir,
        search: "users",
        searchMode: "expand",
      }),
    ).result;
    const paths = r.current.visibleRows.map((row) => row.path);
    // src is force-expanded (descendant match), src/api is force-expanded
    // (descendant match). src/ui is NOT expanded (no match under it), so its
    // children stay hidden.
    expect(paths).toEqual([SRC, SRC_UI, SRC_API, USERS_TS, README]);
  });

  it("collapse mode: keeps everything visible, collapses non-matching dirs", () => {
    const r = renderHook(() =>
      useTree({
        nodes: SEARCH_TREE,
        getPath,
        getChildren,
        isDir,
        search: "button",
        searchMode: "collapse",
        defaultExpanded: "all", // start fully open; collapse mode should override
      }),
    ).result;
    const paths = r.current.visibleRows.map((row) => row.path);
    // src/api collapses (no match under it); src and src/ui stay expanded
    // because there's a match under them.
    expect(paths).toEqual([SRC, SRC_UI, BUTTON_TSX, CARD_TSX, SRC_API, README]);
  });

  it("empty search reverts to standard expand behavior", () => {
    const r = renderHook(() =>
      useTree({
        nodes: SEARCH_TREE,
        getPath,
        getChildren,
        isDir,
        search: "",
        searchMode: "hide",
      }),
    ).result;
    // Nothing expanded, no search — only top level visible.
    expect(r.current.visibleRows.map((row) => row.path)).toEqual([SRC, README]);
  });
});

describe("useTree — node-shape agnosticism via adapter functions", () => {
  it("works with snake_case fields (file-browser's backend shape)", () => {
    interface SnakeNode {
      path: string;
      is_dir: boolean;
      children?: SnakeNode[];
    }
    const snake: SnakeNode[] = [
      { path: SRC, is_dir: true, children: [{ path: "src/a.ts", is_dir: false }] },
    ];
    const r = renderHook(() =>
      useTree<SnakeNode>({
        nodes: snake,
        getPath: (n) => n.path,
        getChildren: (n) => n.children,
        isDir: (n) => n.is_dir,
      }),
    ).result;
    act(() => r.current.toggle(SRC));
    expect(r.current.visibleRows.map((row) => row.path)).toEqual([SRC, "src/a.ts"]);
  });
});
