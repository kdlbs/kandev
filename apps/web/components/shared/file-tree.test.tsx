import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { FileTree, type FileTreeNode } from "./file-tree";

afterEach(cleanup);

function file(name: string, parentPath = ""): FileTreeNode {
  return {
    name,
    path: parentPath ? `${parentPath}/${name}` : name,
    isDir: false,
    children: [],
  };
}

type RenderOpts = Partial<Parameters<typeof FileTree>[0]>;

function renderTree(nodes: FileTreeNode[], overrides: RenderOpts = {}) {
  const onSelectPath = vi.fn();
  const onCheckedPathsChange = vi.fn();
  render(
    <FileTree
      nodes={nodes}
      onSelectPath={onSelectPath}
      onCheckedPathsChange={onCheckedPathsChange}
      {...overrides}
    />,
  );
  return { onSelectPath, onCheckedPathsChange };
}

describe("FileTree rendering and selection", () => {
  it("renders top-level nodes by default with dirs collapsed", () => {
    const tree: FileTreeNode[] = [
      {
        name: "src",
        path: "src",
        isDir: true,
        children: [file("a.ts", "src"), file("b.ts", "src")],
      },
      file("README.md"),
    ];
    renderTree(tree);
    expect(screen.getByText("src")).toBeTruthy();
    expect(screen.getByText("README.md")).toBeTruthy();
    // Children of "src" are hidden until expanded.
    expect(screen.queryByText("a.ts")).toBeNull();
    expect(screen.queryByText("b.ts")).toBeNull();
  });

  it("clicking a directory expands it and reveals children", () => {
    const tree: FileTreeNode[] = [
      {
        name: "src",
        path: "src",
        isDir: true,
        children: [file("a.ts", "src")],
      },
    ];
    renderTree(tree);
    fireEvent.click(screen.getByText("src"));
    expect(screen.getByText("a.ts")).toBeTruthy();
  });

  it("clicking a file fires onSelectPath with its path", () => {
    const tree: FileTreeNode[] = [file("README.md")];
    const { onSelectPath } = renderTree(tree);
    fireEvent.click(screen.getByText("README.md"));
    expect(onSelectPath).toHaveBeenCalledWith("README.md");
  });

  it("defaultExpanded=true expands all directories on mount", () => {
    const tree: FileTreeNode[] = [
      {
        name: "src",
        path: "src",
        isDir: true,
        children: [
          {
            name: "ui",
            path: "src/ui",
            isDir: true,
            children: [file("Button.tsx", "src/ui")],
          },
        ],
      },
    ];
    renderTree(tree, { defaultExpanded: true });
    // The src/ui chain collapses to a single display row "src/ui",
    // and the deepest dir's children are visible because all dirs
    // are pre-expanded.
    expect(screen.getByText("Button.tsx")).toBeTruthy();
  });

  it("collapses single-child directory chains into 'a/b' display names", () => {
    const tree: FileTreeNode[] = [
      {
        name: "src",
        path: "src",
        isDir: true,
        children: [
          {
            name: "ui",
            path: "src/ui",
            isDir: true,
            children: [file("Button.tsx", "src/ui")],
          },
        ],
      },
    ];
    renderTree(tree);
    // Initial render: "src/ui" appears as one row (chain collapse), no
    // separate "src" or "ui" rows.
    expect(screen.getByText("src/ui")).toBeTruthy();
    expect(screen.queryByText("src")).toBeNull();
    expect(screen.queryByText("ui")).toBeNull();
  });

  it("selectedPath highlights the matching file row", () => {
    const tree: FileTreeNode[] = [file("a.ts")];
    renderTree(tree, { selectedPath: "a.ts" });
    const row = screen.getByText("a.ts").parentElement!;
    expect(row.className).toContain("bg-accent");
  });

  it("renderExtra slot is invoked per row and its output appears in the DOM", () => {
    const tree: FileTreeNode[] = [file("a.ts"), file("b.ts")];
    const renderExtra = vi.fn((n: FileTreeNode) => <span>{`x:${n.name}`}</span>);
    renderTree(tree, { renderExtra });
    expect(screen.getByText("x:a.ts")).toBeTruthy();
    expect(screen.getByText("x:b.ts")).toBeTruthy();
    expect(renderExtra).toHaveBeenCalledTimes(2);
  });
});

describe("FileTree checkboxes", () => {
  it("does not render checkboxes when showCheckboxes is false", () => {
    const tree: FileTreeNode[] = [file("a.ts")];
    renderTree(tree);
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("renders a checkbox per node when showCheckboxes and checkedPaths are set", () => {
    const tree: FileTreeNode[] = [file("a.ts"), file("b.ts")];
    renderTree(tree, { showCheckboxes: true, checkedPaths: new Set<string>() });
    expect(screen.getAllByRole("checkbox")).toHaveLength(2);
  });

  it("clicking a directory checkbox propagates the toggle to every leaf under it", () => {
    const tree: FileTreeNode[] = [
      {
        name: "src",
        path: "src",
        isDir: true,
        children: [file("a.ts", "src"), file("b.ts", "src")],
      },
    ];
    const { onCheckedPathsChange } = renderTree(tree, {
      showCheckboxes: true,
      checkedPaths: new Set<string>(),
      defaultExpanded: true,
    });
    // There are three checkboxes: src, a.ts, b.ts. The first one is the dir.
    const boxes = screen.getAllByRole("checkbox");
    fireEvent.click(boxes[0]);
    expect(onCheckedPathsChange).toHaveBeenCalledTimes(1);
    const next = onCheckedPathsChange.mock.calls[0][0] as Set<string>;
    expect(next.has("src/a.ts")).toBe(true);
    expect(next.has("src/b.ts")).toBe(true);
  });

  it("dir checkbox renders indeterminate when only some descendants are checked", () => {
    const tree: FileTreeNode[] = [
      {
        name: "src",
        path: "src",
        isDir: true,
        children: [file("a.ts", "src"), file("b.ts", "src")],
      },
    ];
    renderTree(tree, {
      showCheckboxes: true,
      checkedPaths: new Set<string>(["src/a.ts"]),
      defaultExpanded: true,
    });
    const [dirBox] = screen.getAllByRole("checkbox");
    expect(dirBox.getAttribute("data-state")).toBe("indeterminate");
  });

  it("checkbox click does not bubble to the row's onSelectPath", () => {
    const tree: FileTreeNode[] = [file("a.ts")];
    const { onSelectPath } = renderTree(tree, {
      showCheckboxes: true,
      checkedPaths: new Set<string>(),
    });
    const box = screen.getByRole("checkbox");
    fireEvent.click(box);
    expect(onSelectPath).not.toHaveBeenCalled();
  });
});
