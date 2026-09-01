import { describe, expect, it } from "vitest";

import type { Repository, RepositorySet } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";
import {
  applyRepositorySet,
  selectedRepositoryIdsForSet,
} from "@/components/task-create-dialog-repository-sets";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";

function repository(id: string): Repository {
  return { id: repositoryId(id), name: id } as unknown as Repository;
}

const REPO_WEB = "repo-web";
const REPO_GATEWAY = "repo-gateway";
const REPO_ORDERS = "repo-orders";
const ROW_0 = "row-0";
const AVAILABLE = [repository(REPO_WEB), repository(REPO_GATEWAY), repository(REPO_ORDERS)];

function repositorySet(
  ids: string[],
  name = "Full-stack",
  bases: Record<string, string> = {},
): RepositorySet {
  return {
    id: "set-1",
    workspace_id: workspaceId("ws-1"),
    name,
    description: "",
    repositories: ids.map((id, position) => ({
      repository_id: repositoryId(id),
      position,
      base_branch: bases[id] ?? "",
    })),
    created_at: "2026-08-17T09:00:00Z",
    updated_at: "2026-08-17T09:00:00Z",
  };
}

function row(key: string, repositoryIdValue?: string, branch = ""): TaskRepoRow {
  return { key, repositoryId: repositoryIdValue, branch };
}

function idsOf(rows: TaskRepoRow[]) {
  return rows.map((entry) => entry.repositoryId);
}

describe("applyRepositorySet", () => {
  it("adds one row per member in set order", () => {
    const result = applyRepositorySet({
      rows: [],
      set: repositorySet([REPO_ORDERS, REPO_WEB]),
      repositories: AVAILABLE,
    });

    expect(idsOf(result.rows)).toEqual([REPO_ORDERS, REPO_WEB]);
    expect(result.addedCount).toBe(2);
  });

  it("leaves each added row's branch empty so the per-row default fills it", () => {
    const result = applyRepositorySet({
      rows: [],
      set: repositorySet([REPO_WEB]),
      repositories: AVAILABLE,
    });

    expect(result.rows[0].branch).toBe("");
  });

  it("copies a saved base into new row state without using it as the row checkout", () => {
    const result = applyRepositorySet({
      rows: [],
      set: repositorySet([REPO_WEB], "Full-stack", { [REPO_WEB]: "develop" }),
      repositories: AVAILABLE,
    });

    expect(result.rows[0]).toMatchObject({
      repositoryId: REPO_WEB,
      branch: "",
      baseBranch: "develop",
    });
  });

  it("keeps an empty saved base on normal task defaulting", () => {
    const result = applyRepositorySet({
      rows: [],
      set: repositorySet([REPO_WEB]),
      repositories: AVAILABLE,
    });

    expect(result.rows[0]).not.toHaveProperty("baseBranch");
  });

  it("consumes a single blank placeholder row instead of leaving it behind", () => {
    const result = applyRepositorySet({
      rows: [row(ROW_0)],
      set: repositorySet([REPO_WEB, REPO_GATEWAY]),
      repositories: AVAILABLE,
    });

    expect(result.rows).toHaveLength(2);
    expect(idsOf(result.rows)).toEqual([REPO_WEB, REPO_GATEWAY]);
  });

  it("keeps a configured row and appends the set after it", () => {
    const configured = row(ROW_0, REPO_ORDERS, "main");
    const result = applyRepositorySet({
      rows: [configured],
      set: repositorySet([REPO_WEB]),
      repositories: AVAILABLE,
    });

    expect(result.rows[0]).toEqual(configured);
    expect(idsOf(result.rows)).toEqual([REPO_ORDERS, REPO_WEB]);
  });

  it("does not discard a blank row that the user has already given a branch", () => {
    const branchOnly = row(ROW_0, undefined, "develop");
    const result = applyRepositorySet({
      rows: [branchOnly],
      set: repositorySet([REPO_WEB]),
      repositories: AVAILABLE,
    });

    expect(result.rows).toHaveLength(2);
    expect(result.rows[0]).toEqual(branchOnly);
  });

  it("skips members already present in the form", () => {
    const result = applyRepositorySet({
      rows: [row(ROW_0, REPO_WEB, "main")],
      set: repositorySet([REPO_WEB, REPO_GATEWAY]),
      repositories: AVAILABLE,
    });

    expect(idsOf(result.rows)).toEqual([REPO_WEB, REPO_GATEWAY]);
    expect(result.addedCount).toBe(1);
    expect(result.alreadyPresentCount).toBe(1);
  });

  it("is idempotent: applying the same set twice changes nothing", () => {
    const set = repositorySet([REPO_WEB, REPO_GATEWAY]);
    const first = applyRepositorySet({ rows: [], set, repositories: AVAILABLE });
    const second = applyRepositorySet({ rows: first.rows, set, repositories: AVAILABLE });

    expect(second.rows).toEqual(first.rows);
    expect(second.addedCount).toBe(0);
    expect(second.alreadyPresentCount).toBe(2);
  });

  it("yields the union when two overlapping sets are applied", () => {
    const first = applyRepositorySet({
      rows: [],
      set: repositorySet([REPO_WEB, REPO_GATEWAY]),
      repositories: AVAILABLE,
    });
    const second = applyRepositorySet({
      rows: first.rows,
      set: repositorySet([REPO_GATEWAY, REPO_ORDERS], "Backend"),
      repositories: AVAILABLE,
    });

    expect(idsOf(second.rows)).toEqual([REPO_WEB, REPO_GATEWAY, REPO_ORDERS]);
  });
});

describe("applyRepositorySet with unavailable members", () => {
  it("skips members missing from the live repository list and counts them", () => {
    const result = applyRepositorySet({
      rows: [],
      set: repositorySet([REPO_WEB, "repo-deleted"]),
      repositories: AVAILABLE,
    });

    expect(idsOf(result.rows)).toEqual([REPO_WEB]);
    expect(result.addedCount).toBe(1);
    expect(result.missingCount).toBe(1);
  });

  it("adds nothing and reports every member missing when the workspace has no repositories", () => {
    const result = applyRepositorySet({
      rows: [row(ROW_0)],
      set: repositorySet([REPO_WEB, REPO_GATEWAY]),
      repositories: [],
    });

    // The placeholder row survives: consuming it would leave the form with no row
    // at all.
    expect(result.rows).toHaveLength(1);
    expect(result.rows[0].repositoryId).toBeUndefined();
    expect(result.addedCount).toBe(0);
    expect(result.missingCount).toBe(2);
  });

  it("allocates keys that cannot collide with existing rows", () => {
    const result = applyRepositorySet({
      rows: [row("row-1", REPO_ORDERS, "main")],
      set: repositorySet([REPO_WEB, REPO_GATEWAY]),
      repositories: AVAILABLE,
    });

    const keys = result.rows.map((entry) => entry.key);
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys).toContain("row-1");
  });

  it("does not reuse a key that a later addRepository() would hand out", () => {
    // useRepositoriesState's counter starts at 0 and hands out row-1, row-2, ...
    // so applied rows must not squat on those names.
    const result = applyRepositorySet({
      rows: [],
      set: repositorySet([REPO_WEB, REPO_GATEWAY]),
      repositories: AVAILABLE,
    });

    for (const entry of result.rows) {
      expect(entry.key.startsWith("set-row-")).toBe(true);
    }
  });

  it("treats a discovered local-path row as occupying that repository", () => {
    const discovered: TaskRepoRow = { key: ROW_0, localPath: "/src/web", branch: "main" };
    const result = applyRepositorySet({
      rows: [discovered],
      set: repositorySet([REPO_WEB]),
      repositories: AVAILABLE,
    });

    // A local path is not a workspace repository id, so the member is still added
    // and the configured row is left untouched.
    expect(result.rows).toHaveLength(2);
    expect(result.rows[0]).toEqual(discovered);
  });
});

describe("selectedRepositoryIdsForSet", () => {
  it("collects workspace repository ids in row order", () => {
    const ids = selectedRepositoryIdsForSet([
      row(ROW_0, REPO_WEB, "main"),
      row("row-1", REPO_GATEWAY, "develop"),
    ]);

    expect(ids).toEqual([REPO_WEB, REPO_GATEWAY]);
  });

  it("ignores blank rows", () => {
    expect(selectedRepositoryIdsForSet([row(ROW_0), row("row-1", REPO_WEB)])).toEqual([REPO_WEB]);
  });

  it("ignores discovered local-path rows, which are not workspace repositories", () => {
    const ids = selectedRepositoryIdsForSet([
      { key: ROW_0, localPath: "/src/web", branch: "main" },
      row("row-1", REPO_WEB),
    ]);

    expect(ids).toEqual([REPO_WEB]);
  });

  it("dedupes a repository the user added on two rows for different branches", () => {
    const ids = selectedRepositoryIdsForSet([
      row(ROW_0, REPO_WEB, "main"),
      row("row-1", REPO_WEB, "release"),
    ]);

    expect(ids).toEqual([REPO_WEB]);
  });
});
