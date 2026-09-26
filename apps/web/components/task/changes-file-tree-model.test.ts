import { describe, expect, it } from "vitest";
import { buildChangesTree } from "./changes-file-tree-model";

describe("buildChangesTree", () => {
  it("sorts directories before files and keeps pathless entries out", () => {
    const tree = buildChangesTree([
      { path: "z.ts", status: "modified", staged: false, plus: 1, minus: 0, oldPath: undefined },
      {
        path: "src/a.ts",
        status: "modified",
        staged: false,
        plus: 1,
        minus: 0,
        oldPath: undefined,
      },
      {
        path: "src/b.ts",
        status: "modified",
        staged: false,
        plus: 1,
        minus: 0,
        oldPath: undefined,
      },
      { path: "", status: "modified", staged: false, plus: 1, minus: 0, oldPath: undefined },
    ]);

    expect(tree.map((node) => node.path)).toEqual(["src", "z.ts"]);
    expect(tree[0].children?.map((node) => node.path)).toEqual(["src/a.ts", "src/b.ts"]);
  });
});
