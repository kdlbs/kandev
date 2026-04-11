import { describe, it, expect } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";
import {
  moveNodesInTree,
  computeMoveTargets,
  findNodeByPath,
  getVisiblePaths,
} from "./file-tree-utils";

function file(name: string, parentPath = ""): FileTreeNode {
  const path = parentPath ? `${parentPath}/${name}` : name;
  return { name, path, is_dir: false, size: 0 };
}

function dir(name: string, children: FileTreeNode[], parentPath = ""): FileTreeNode {
  const path = parentPath ? `${parentPath}/${name}` : name;
  return { name, path, is_dir: true, children };
}

function root(children: FileTreeNode[]): FileTreeNode {
  return { name: "", path: "", is_dir: true, children };
}

const UTILS_TS = "utils.ts";

describe("computeMoveTargets", () => {
  it("computes simple move without collision", () => {
    const tree = root([file("a.ts"), dir("lib", [file("b.ts", "lib")])]);
    const result = computeMoveTargets(tree, ["a.ts"], "lib");
    expect(result).toEqual([{ oldPath: "a.ts", newPath: "lib/a.ts" }]);
  });

  it("deduplicates when target already has a file with the same name", () => {
    const tree = root([dir("src", [file(UTILS_TS, "src")]), dir("lib", [file(UTILS_TS, "lib")])]);
    const result = computeMoveTargets(tree, [`src/${UTILS_TS}`], "lib");
    expect(result).toEqual([{ oldPath: `src/${UTILS_TS}`, newPath: "lib/utils (1).ts" }]);
  });

  it("deduplicates when two source files have the same name", () => {
    const tree = root([
      dir("src", [file(UTILS_TS, "src")]),
      dir("lib", [file(UTILS_TS, "lib")]),
      dir("target", []),
    ]);
    const result = computeMoveTargets(tree, [`src/${UTILS_TS}`, `lib/${UTILS_TS}`], "target");
    expect(result).toEqual([
      { oldPath: `src/${UTILS_TS}`, newPath: `target/${UTILS_TS}` },
      { oldPath: `lib/${UTILS_TS}`, newPath: "target/utils (1).ts" },
    ]);
  });

  it("deduplicates with multiple existing collisions", () => {
    const tree = root([
      file("readme.md"),
      dir("dest", [file("readme.md", "dest"), file("readme (1).md", "dest")]),
    ]);
    const result = computeMoveTargets(tree, ["readme.md"], "dest");
    expect(result).toEqual([{ oldPath: "readme.md", newPath: "dest/readme (2).md" }]);
  });

  it("handles files without extensions", () => {
    const tree = root([file("Makefile"), dir("dest", [file("Makefile", "dest")])]);
    const result = computeMoveTargets(tree, ["Makefile"], "dest");
    expect(result).toEqual([{ oldPath: "Makefile", newPath: "dest/Makefile (1)" }]);
  });
});

describe("moveNodesInTree", () => {
  it("moves a file into a directory", () => {
    const tree = root([file("a.ts"), dir("lib", [])]);
    const result = moveNodesInTree(tree, ["a.ts"], "lib");
    expect(findNodeByPath(result, "a.ts")).toBeNull();
    expect(findNodeByPath(result, "lib/a.ts")).not.toBeNull();
  });

  it("deduplicates on collision during move", () => {
    const tree = root([
      dir("src", [file("x.ts", "src")]),
      dir("lib", [file("x.ts", "lib")]),
      dir("dest", []),
    ]);
    const result = moveNodesInTree(tree, ["src/x.ts", "lib/x.ts"], "dest");
    expect(findNodeByPath(result, "dest/x.ts")).not.toBeNull();
    expect(findNodeByPath(result, "dest/x (1).ts")).not.toBeNull();
    expect(findNodeByPath(result, "src/x.ts")).toBeNull();
    expect(findNodeByPath(result, "lib/x.ts")).toBeNull();
  });
});

describe("getVisiblePaths", () => {
  it("returns all nodes in DFS order including directories", () => {
    const tree = root([dir("lib", [file("config.ts", "lib")], ""), file("readme.md")]);
    const expanded = new Set(["lib"]);
    const paths = getVisiblePaths(tree, expanded);
    // dirs first then files, alphabetical within each group
    expect(paths).toEqual(["lib", "lib/config.ts", "readme.md"]);
  });

  it("excludes collapsed directory children", () => {
    const tree = root([dir("lib", [file("config.ts", "lib")], ""), file("readme.md")]);
    const expanded = new Set<string>();
    const paths = getVisiblePaths(tree, expanded);
    expect(paths).toEqual(["lib", "readme.md"]);
  });
});
