import { act, renderHook } from "@testing-library/react";
import { useState } from "react";
import { expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";
import { useFileTreeRootReveal } from "./file-tree-root-reveal";

const WORKTREES_PATH = ".claude/worktrees";
const ADOPTED_ROOT = `${WORKTREES_PATH}/fix-b`;
const SECOND_ROOT = `${WORKTREES_PATH}/feature-c`;

function dir(name: string, parentPath = "", children?: FileTreeNode[]): FileTreeNode {
  return {
    name,
    path: parentPath ? `${parentPath}/${name}` : name,
    is_dir: true,
    size: 0,
    children,
  };
}

function tree(): FileTreeNode {
  return dir("", "", [
    dir(".claude", "", [
      dir("worktrees", ".claude", [dir("fix-b", WORKTREES_PATH), dir("feature-c", WORKTREES_PATH)]),
    ]),
  ]);
}

function useHarness({ rootPaths }: { rootPaths: string[] }) {
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  useFileTreeRootReveal({
    rootPaths,
    tree: tree(),
    expandedPaths,
    setExpandedPaths,
    isLoadingTree: false,
    loadChildren: vi.fn(async () => true),
  });
  return { expandedPaths, setExpandedPaths };
}

it("reveals ancestors once per root-list change and keeps a later collapse", () => {
  const { result, rerender } = renderHook(useHarness, {
    initialProps: { rootPaths: [ADOPTED_ROOT] },
  });
  expect([...result.current.expandedPaths]).toEqual([".claude", ".claude/worktrees"]);

  act(() => result.current.setExpandedPaths(new Set()));
  expect(result.current.expandedPaths.size).toBe(0);

  rerender({ rootPaths: [SECOND_ROOT] });
  expect([...result.current.expandedPaths]).toEqual([".claude", ".claude/worktrees"]);
});
