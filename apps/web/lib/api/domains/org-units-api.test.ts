import { describe, expect, it } from "vitest";
import { withDepth, type OrgUnit } from "./org-units-api";

function unit(id: string, path: string, name = id): OrgUnit {
  return {
    id,
    org_id: "org-1",
    parent_id: "",
    kind: "standard",
    name,
    path,
    created_at: "",
    updated_at: "",
  };
}

describe("withDepth", () => {
  // Depth is what indents the tree. Deriving it from the materialized path
  // rather than walking parents keeps the render a single pass, but it means
  // an off-by-one here flattens a department into its own parent.
  it("derives indentation depth from the materialized path", () => {
    const rows = withDepth([
      unit("root", "/root/"),
      unit("dept", "/root/dept/"),
      unit("team", "/root/dept/team/"),
      unit("squad", "/root/dept/team/squad/"),
    ]);

    expect(rows.map((r) => r.depth)).toEqual([0, 1, 2, 3]);
  });

  it("keeps the backend's parent-before-child order", () => {
    const rows = withDepth([unit("root", "/root/"), unit("dept", "/root/dept/")]);
    expect(rows.map((r) => r.id)).toEqual(["root", "dept"]);
  });

  it("treats a malformed path as top level rather than throwing", () => {
    expect(withDepth([unit("odd", "")])[0]!.depth).toBe(0);
  });
});
