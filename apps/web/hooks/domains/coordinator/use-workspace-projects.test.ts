import { describe, expect, it } from "vitest";
import type { Repository, RepositorySet } from "@/lib/types/http";
import { projectChoices } from "./use-workspace-projects";

const repo = (id: string, name: string) => ({ id, name }) as Repository;
const set = (id: string, name: string, ids: string[]) =>
  ({
    id,
    name,
    repositories: ids.map((repository_id, position) => ({ repository_id, position })),
  }) as RepositorySet;

describe("projectChoices", () => {
  it("orders by lowercase name then id and drops repositories already in a set", () => {
    const { sets, loose } = projectChoices(
      [set("s2", "payments", ["r1", "r2"]), set("s1", "Mobile", ["r3"])],
      [repo("r4", "Zed"), repo("r1", "a"), repo("r5", "zed"), repo("r3", "c")],
    );
    expect(sets.map((s) => s.id)).toEqual(["s1", "s2"]);
    expect(sets.map((s) => s.repositoryCount)).toEqual([1, 2]);
    expect(loose.map((r) => r.id)).toEqual(["r4", "r5"]);
  });
});
