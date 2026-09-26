import { describe, expect, it } from "vitest";
import {
  fileTreeGeometryIssues,
  type FileTreeViewportGeometry,
} from "../../e2e/tests/task/file-tree-geometry";

function geometry(overrides: Partial<FileTreeViewportGeometry>): FileTreeViewportGeometry {
  return {
    bottom: 100,
    clientHeight: 100,
    rows: [{ path: "row-0", top: 0, bottom: 28 }],
    scrollTop: 0,
    top: 0,
    treeHeight: 28,
    ...overrides,
  };
}

describe("file-tree viewport geometry assertions", () => {
  it("rejects a blank top edge", () => {
    expect(
      fileTreeGeometryIssues(
        geometry({
          rows: [{ path: "row-4", top: 30, bottom: 58 }],
          treeHeight: 300,
        }),
      ),
    ).toContainEqual(expect.stringContaining("blank gap at the viewport top"));
  });

  it("rejects a blank bottom edge when overflowing content continues below", () => {
    expect(
      fileTreeGeometryIssues(
        geometry({
          rows: [
            { path: "row-0", top: 0, bottom: 28 },
            { path: "row-1", top: 28, bottom: 80 },
          ],
          treeHeight: 300,
        }),
      ),
    ).toContainEqual(expect.stringContaining("blank gap at the viewport bottom"));
  });

  it("allows trailing space for a short tree", () => {
    expect(
      fileTreeGeometryIssues(
        geometry({
          rows: [{ path: "folder", top: 0, bottom: 28 }],
          treeHeight: 99,
        }),
      ),
    ).toEqual([]);
  });

  it("allows the configured end padding at the end of overflowing content", () => {
    expect(
      fileTreeGeometryIssues(
        geometry({
          rows: [
            { path: "row-4", top: -20, bottom: 8 },
            { path: "row-5", top: 8, bottom: 92 },
          ],
          scrollTop: 200,
          treeHeight: 300,
        }),
      ),
    ).toEqual([]);
  });
});
