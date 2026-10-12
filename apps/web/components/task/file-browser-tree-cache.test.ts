import { describe, expect, it } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";
import { FileBrowserTreeCache } from "./file-browser-tree-cache";

function tree(path: string, count = 0): FileTreeNode {
  return {
    path,
    name: path,
    is_dir: true,
    size: 0,
    children: Array.from({ length: count }, (_, index) => ({
      path: `${path}/${index}`,
      name: String(index),
      is_dir: false,
      size: 0,
    })),
  };
}

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
describe("recent file trees", () => {
  it("retains tree identity and evicts least recently used snapshots beyond four", () => {
    const cache = new FileBrowserTreeCache();
    const first = tree("a");
    cache.set("a", first);
    for (const key of ["b", "c", "d"]) cache.set(key, tree(key));
    expect(cache.get("a")).toBe(first);
    cache.set("e", tree("e"));
    expect(cache.get("b")).toBeNull();
    expect(cache.get("a")).toBe(first);
  });

  it("bounds total retained nodes and rejects an oversized replacement", () => {
    const cache = new FileBrowserTreeCache();
    cache.set("a", tree("a", 12000));
    cache.set("b", tree("b", 12000));
    expect(cache.get("a")).toBeNull();
    expect(cache.get("b")?.children).toHaveLength(12000);
    cache.set("b", tree("too-large", 20000));
    expect(cache.get("b")).toBeNull();
  });

  it("removes an authoritative empty tree without affecting other contexts", () => {
    const cache = new FileBrowserTreeCache();
    const kept = tree("kept");
    cache.set("a", tree("a"));
    cache.set("b", kept);
    cache.set("a", null);
    expect(cache.get("a")).toBeNull();
    expect(cache.get("b")).toBe(kept);
  });
});
