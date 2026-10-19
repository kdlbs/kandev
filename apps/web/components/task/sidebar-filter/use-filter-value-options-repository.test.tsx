import type { PropsWithChildren } from "react";
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";
import { useFilterValueOptions } from "./use-filter-value-options";

afterEach(cleanup);

function wrapper(repositories: Repository[] = []) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <StateProvider
        initialState={{
          repositories: {
            itemsByWorkspaceId: { workspace: repositories },
            loadingByWorkspaceId: {},
            loadedByWorkspaceId: {},
          },
        }}
      >
        {children}
      </StateProvider>
    );
  };
}

// @covers AC-UI-REPOSITORY-FILTER-NONE-001.1
describe("repository filter choices", () => {
  it("offers None when the repository catalog is empty", () => {
    const { result } = renderHook(() => useFilterValueOptions("repository"), {
      wrapper: wrapper(),
    });

    expect(result.current).toEqual([{ value: "undefined", label: "None" }]);
  });

  it("keeps named repository values and order after None", () => {
    const local: Repository = {
      id: repositoryId("local"),
      workspace_id: workspaceId("workspace"),
      name: "local-repo",
      source_type: "local",
      local_path: "/tmp/local-repo",
      provider: "",
      provider_repo_id: "",
      provider_owner: "",
      provider_name: "",
      default_branch: "main",
      worktree_branch_prefix: "",
      pull_before_worktree: false,
      setup_script: "",
      cleanup_script: "",
      dev_script: "",
      copy_files: "",
      created_at: "2026-10-19T00:00:00Z",
      updated_at: "2026-10-19T00:00:00Z",
    };
    const repositories: Repository[] = [
      local,
      {
        ...local,
        id: repositoryId("remote"),
        name: "remote-repo",
        source_type: "provider",
        local_path: "",
        provider: "github",
        provider_repo_id: "repo",
        provider_owner: "org",
        provider_name: "repo",
      },
    ];
    const { result } = renderHook(() => useFilterValueOptions("repository"), {
      wrapper: wrapper(repositories),
    });

    expect(result.current).toEqual([
      { value: "undefined", label: "None" },
      { value: "local-repo", label: "local-repo" },
      { value: "org/repo", label: "org/repo" },
    ]);
  });

  it("keeps the missing-repository option out of other dimensions", () => {
    const { result } = renderHook(() => useFilterValueOptions("executorType"), {
      wrapper: wrapper(),
    });

    expect(result.current).toEqual([]);
  });
});
