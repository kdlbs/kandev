import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

import { createRepositoryAction, updateRepositoryAction } from "./workspaces";

describe("repository secret binding actions", () => {
  const fetchSpy = vi.fn<typeof fetch>();

  beforeEach(() => {
    fetchSpy.mockReset();
    fetchSpy.mockResolvedValue(
      Response.json({ id: "repo-1", secret_bindings: [] }, { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchSpy);
  });

  afterEach(() => vi.unstubAllGlobals());

  it("includes bindings when creating a repository", async () => {
    await createRepositoryAction({
      workspace_id: "workspace-1",
      name: "repo",
      source_type: "local",
      local_path: "/tmp/repo",
      provider: "",
      provider_repo_id: "",
      provider_owner: "",
      provider_name: "",
      default_branch: "main",
      worktree_branch_prefix: "feature/",
      worktree_branch_template: "feature/{title}-{suffix}",
      pull_before_worktree: true,
      setup_script: "",
      cleanup_script: "",
      dev_script: "",
      copy_files: "",
      secret_bindings: [{ key: "TOKEN", secret_id: "secret-1" }],
    });

    const [, init] = fetchSpy.mock.calls[0]!;
    expect(JSON.parse(String(init?.body)).secret_bindings).toEqual([
      { key: "TOKEN", secret_id: "secret-1" },
    ]);
  });

  it("sends an explicit empty list when clearing bindings", async () => {
    await updateRepositoryAction("repo-1", { secret_bindings: [] });

    const [, init] = fetchSpy.mock.calls[0]!;
    expect(JSON.parse(String(init?.body))).toEqual({ secret_bindings: [] });
  });
});
