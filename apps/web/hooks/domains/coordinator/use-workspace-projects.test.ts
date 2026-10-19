import { renderHook, waitFor, act } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Repository, RepositorySet } from "@/lib/types/http";
import { projectChoices, useWorkspaceProjects } from "./use-workspace-projects";

const listSets = vi.fn();
const listRepos = vi.fn();
vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositorySets: (...a: unknown[]) => listSets(...a),
  listRepositories: (...a: unknown[]) => listRepos(...a),
}));

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

describe("useWorkspaceProjects", () => {
  beforeEach(() => {
    listSets.mockReset();
    listRepos.mockReset();
  });

  it("loads the choices", async () => {
    listSets.mockResolvedValue({ repository_sets: [set("s1", "Payments", ["r1"])] });
    listRepos.mockResolvedValue({ repositories: [repo("r1", "a"), repo("r2", "b")] });
    const { result } = renderHook(() => useWorkspaceProjects("w1"));
    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.sets.map((s) => s.id)).toEqual(["s1"]);
    expect(result.current.loose.map((r) => r.id)).toEqual(["r2"]);
  });

  it("leaves loading on a rejected read and recovers on retry", async () => {
    listSets.mockRejectedValueOnce(new Error("down"));
    listRepos.mockResolvedValue({ repositories: [] });
    const { result } = renderHook(() => useWorkspaceProjects("w1"));
    await waitFor(() => expect(result.current.status).toBe("error"));
    listSets.mockResolvedValue({ repository_sets: [set("s1", "Payments", ["r1"])] });
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.sets).toHaveLength(1);
  });

  it("ignores a response that resolves after unmount", async () => {
    let resolveSets: (v: unknown) => void = () => {};
    listSets.mockReturnValue(new Promise((r) => (resolveSets = r)));
    listRepos.mockResolvedValue({ repositories: [] });
    const { result, unmount } = renderHook(() => useWorkspaceProjects("w1"));
    unmount();
    resolveSets({ repository_sets: [set("s1", "Payments", ["r1"])] });
    await Promise.resolve();
    expect(result.current.status).toBe("loading");
    expect(result.current.sets).toHaveLength(0);
  });
});
