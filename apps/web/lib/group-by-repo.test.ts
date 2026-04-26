import { describe, it, expect } from "vitest";
import { groupByRepositoryName } from "./group-by-repo";

describe("groupByRepositoryName", () => {
  it("buckets by name in first-seen order, preserving item order within each bucket", () => {
    const items = [
      { id: "a", repo: "frontend" },
      { id: "b", repo: "backend" },
      { id: "c", repo: "frontend" },
      { id: "d", repo: "backend" },
    ];
    const groups = groupByRepositoryName(items, (i) => i.repo);
    expect(groups.map((g) => g.repositoryName)).toEqual(["frontend", "backend"]);
    expect(groups[0].items.map((i) => i.id)).toEqual(["a", "c"]);
    expect(groups[1].items.map((i) => i.id)).toEqual(["b", "d"]);
  });

  it("collapses missing/undefined names into a single empty-name bucket", () => {
    const items = [{ id: "x" }, { id: "y", repo: "named" }, { id: "z" }];
    const groups = groupByRepositoryName(items, (i) => (i as { repo?: string }).repo);
    expect(groups.map((g) => g.repositoryName)).toEqual(["", "named"]);
    expect(groups[0].items.map((i) => i.id)).toEqual(["x", "z"]);
    expect(groups[1].items.map((i) => i.id)).toEqual(["y"]);
  });

  it("returns an empty array for an empty input", () => {
    expect(groupByRepositoryName([], () => "x")).toEqual([]);
  });
});
