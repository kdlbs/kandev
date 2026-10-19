import type { PropsWithChildren } from "react";
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";
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
    const repositories = [
      { id: "local", name: "local-repo", local_path: "/tmp/local-repo" },
      { id: "remote", name: "remote-repo", provider_owner: "org", provider_name: "repo" },
    ] as Repository[];
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
