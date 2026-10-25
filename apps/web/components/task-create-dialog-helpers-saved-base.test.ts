import { describe, expect, it } from "vitest";

import { buildRepositoriesPayload } from "./task-create-dialog-helpers";

describe("buildRepositoriesPayload — saved local base branches", () => {
  it("omits checkout when the local checkout already matches a saved base", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [
        { key: "r0", repositoryId: "repo-1", branch: "develop", baseBranch: "develop" },
      ],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: true,
    });

    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "develop", checkout_branch: undefined },
    ]);
  });
});
