import { describe, expect, it } from "vitest";
import type { TaskCreateLastUsedSourceApi } from "@/lib/types/http-user-settings";
import {
  hasLastUsedWorkspaceSnapshot,
  selectionsFromLastUsedSources,
} from "./task-create-dialog-workspace-defaults";

const FOLDER_KIND = "folder" as const;
const REPOSITORY_KIND = "repository" as const;
const REPOSITORY_ID = "repo-local";
const MAIN_BRANCH = "main";

describe("last-used workspace source defaults", () => {
  it("rehydrates folders and repository sources in their saved order", () => {
    const selections = selectionsFromLastUsedSources([
      { kind: FOLDER_KIND, local_path: "/work/assets", display_name: "assets" },
      {
        kind: REPOSITORY_KIND,
        repository_id: REPOSITORY_ID,
        base_branch: MAIN_BRANCH,
        checkout_branch: "feature/local",
        branch_policy_id: "policy-1",
      },
      {
        kind: REPOSITORY_KIND,
        remote_url: "https://git.example.test/acme/api.git",
        provider: "fixture",
        provider_repo_id: "remote-1",
        provider_owner: "acme",
        provider_name: "api",
        base_branch: MAIN_BRANCH,
        checkout_branch: "release",
        pr_number: 42,
      },
    ]);

    expect(selections).toEqual([
      expect.objectContaining({
        kind: FOLDER_KIND,
        localPath: "/work/assets",
        displayName: "assets",
      }),
      expect.objectContaining({
        kind: "local",
        repositoryId: REPOSITORY_ID,
        branch: "feature/local",
        baseBranch: MAIN_BRANCH,
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

  it("restores remote-origin provenance without reusing a stale branch cache", () => {
    const [selection] = selectionsFromLastUsedSources([
      {
        kind: REPOSITORY_KIND,
        repository_id: REPOSITORY_ID,
        base_branch: MAIN_BRANCH,
        checkout_branch: "feature/api",
        checkout_source: "remote_origin",
        expected_origin: "https://github.com/acme/api.git",
      },
    ]);

    expect(selection).toEqual(
      expect.objectContaining({
        kind: "local",
        repositoryId: REPOSITORY_ID,
        checkoutSource: "remote_origin",
        expectedOrigin: "https://github.com/acme/api.git",
      }),
    );
    expect(selection).not.toHaveProperty("remoteBranches");
  });

  it("drops malformed source kinds without changing valid saved rows", () => {
    const selections = selectionsFromLastUsedSources([
      { kind: "future-source", local_path: "/work/future" },
      { kind: FOLDER_KIND, local_path: "/work/docs" },
    ]);

    expect(selections).toEqual([
      expect.objectContaining({ kind: FOLDER_KIND, localPath: "/work/docs" }),
    ]);
  });
});
