import { describe, expect, it } from "vitest";
import type { TaskCreateLastUsedSourceApi } from "@/lib/types/http-user-settings";
import {
  hasLastUsedWorkspaceSnapshot,
  selectionsFromLastUsedSources,
} from "./task-create-dialog-workspace-defaults";

describe("last-used workspace source defaults", () => {
  it("rehydrates folders and repository sources in their saved order", () => {
    const selections = selectionsFromLastUsedSources([
      { kind: "folder", local_path: "/work/assets", display_name: "assets" },
      {
        kind: "repository",
        repository_id: "repo-local",
        base_branch: "main",
        checkout_branch: "feature/local",
        branch_policy_id: "policy-1",
      },
      {
        kind: "repository",
        remote_url: "https://git.example.test/acme/api.git",
        provider: "fixture",
        provider_repo_id: "remote-1",
        provider_owner: "acme",
        provider_name: "api",
        base_branch: "main",
        checkout_branch: "release",
        pr_number: 42,
      },
    ]);

    expect(selections).toEqual([
      expect.objectContaining({ kind: "folder", localPath: "/work/assets", displayName: "assets" }),
      expect.objectContaining({
        kind: "local",
        repositoryId: "repo-local",
        branch: "feature/local",
        baseBranch: "main",
        branchPolicyId: "policy-1",
      }),
      expect.objectContaining({
        kind: "remote",
        url: "https://git.example.test/acme/api.git",
        source: "picker",
        branch: "release",
        provider: "fixture",
        providerRepoId: "remote-1",
        prNumber: 42,
      }),
    ]);
    expect(selections.map((selection) => selection.kind)).toEqual(["folder", "local", "remote"]);
  });

  it("keeps an explicit empty snapshot distinct from a missing snapshot", () => {
    const snapshots: Record<string, TaskCreateLastUsedSourceApi[]> = {
      "workspace-empty": [],
    };

    expect(hasLastUsedWorkspaceSnapshot(snapshots, "workspace-empty")).toBe(true);
    expect(selectionsFromLastUsedSources(snapshots["workspace-empty"])).toEqual([]);
    expect(hasLastUsedWorkspaceSnapshot(snapshots, "workspace-missing")).toBe(false);
  });

  it("drops malformed source kinds without changing valid saved rows", () => {
    const selections = selectionsFromLastUsedSources([
      { kind: "future-source", local_path: "/work/future" },
      { kind: "folder", local_path: "/work/docs" },
    ]);

    expect(selections).toEqual([
      expect.objectContaining({ kind: "folder", localPath: "/work/docs" }),
    ]);
  });
});
