import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const fetchRepoBranchesMock = vi.fn();
const listProjectBranchesMock = vi.fn();
const listAzureDevOpsBranchesMock = vi.fn();

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchRepoBranches: (...args: unknown[]) => fetchRepoBranchesMock(...args),
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({
  listProjectBranches: (...args: unknown[]) => listProjectBranchesMock(...args),
}));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  listAzureDevOpsBranches: (...args: unknown[]) => listAzureDevOpsBranchesMock(...args),
}));

// Import after mocks so the hook picks up the mocked module.
import { useBranchesByURL } from "./use-branches-by-url";

afterEach(() => {
  cleanup();
  fetchRepoBranchesMock.mockReset();
  listProjectBranchesMock.mockReset();
  listAzureDevOpsBranchesMock.mockReset();
  vi.useRealTimers();
});

const REPO_A = "https://github.com/acme/site";
const REPO_B = "https://github.com/acme/api";
const WORKSPACE_ID = "workspace-1";
const WORKSPACE_B = "workspace-2";
const WORKSPACE_B_BRANCH = "workspace-b";

describe("useBranchesByURL", () => {
  it("does not load GitHub branches without a workspace credential scope", () => {
    const { result } = renderHook(() => useBranchesByURL());

    act(() => result.current.ensure(REPO_A));

    expect(fetchRepoBranchesMock).not.toHaveBeenCalled();
    expect(result.current.branches(REPO_A)).toEqual([]);
  });

  it("fetches branches once per unique URL when ensure() is called", async () => {
    fetchRepoBranchesMock.mockImplementation((_workspace: string, _owner: string, repo: string) => {
      return Promise.resolve({
        branches: [{ name: repo === "site" ? "main" : "develop" }],
      });
    });

    const { result } = renderHook(() => useBranchesByURL("ws-1"));

    act(() => {
      result.current.ensure(REPO_A);
      result.current.ensure(REPO_B);
    });

    await waitFor(() => {
      expect(result.current.branches(REPO_A)).toHaveLength(1);
      expect(result.current.branches(REPO_B)).toHaveLength(1);
    });

    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2);
    expect(result.current.branches(REPO_A)[0]).toMatchObject({ name: "main", type: "remote" });
    expect(result.current.branches(REPO_B)[0]).toMatchObject({ name: "develop", type: "remote" });
  });
});

describe("useBranchesByURL provider routing", () => {
  it("dispatches GitLab and Azure URLs without calling GitHub", async () => {
    listProjectBranchesMock.mockResolvedValue({ branches: [{ name: "develop" }] });
    listAzureDevOpsBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });
    const { result } = renderHook(() => useBranchesByURL(WORKSPACE_ID));
    const gitlab = "https://gitlab.com/acme/platform/api.git";
    const azure = "https://dev.azure.com/acme/Platform/_git/api";

    act(() => {
      result.current.ensure(gitlab);
      result.current.ensure(azure);
    });

    await waitFor(() => {
      expect(result.current.branches(gitlab)).toHaveLength(1);
      expect(result.current.branches(azure)).toHaveLength(1);
    });
    expect(fetchRepoBranchesMock).not.toHaveBeenCalled();
    expect(listProjectBranchesMock).toHaveBeenCalledWith(
      WORKSPACE_ID,
      "acme/platform/api",
      expect.objectContaining({
        expectedHost: "https://gitlab.com",
        init: expect.objectContaining({ signal: expect.any(AbortSignal) }),
      }),
    );
    expect(listAzureDevOpsBranchesMock).toHaveBeenCalledWith(
      WORKSPACE_ID,
      "acme",
      "Platform",
      "api",
      expect.anything(),
    );
  });

  it("passes the organization parsed from an Azure SSH URL", async () => {
    listAzureDevOpsBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });
    const { result } = renderHook(() => useBranchesByURL(WORKSPACE_ID));
    const azure = "git@ssh.dev.azure.com:v3/acme/Platform/api";

    act(() => result.current.ensure(azure));

    await waitFor(() => expect(result.current.branches(azure)).toHaveLength(1));
    expect(listAzureDevOpsBranchesMock).toHaveBeenCalledWith(
      WORKSPACE_ID,
      "acme",
      "Platform",
      "api",
      expect.anything(),
    );
  });

  it("loads branches for a self-managed GitLab URL", async () => {
    listProjectBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });
    const { result } = renderHook(() => useBranchesByURL(WORKSPACE_ID));
    const gitlab = "https://gitlab.internal:8443/acme/platform/api.git";

    act(() => result.current.ensure(gitlab));

    await waitFor(() => {
      expect(listProjectBranchesMock).toHaveBeenCalledWith(
        WORKSPACE_ID,
        "acme/platform/api",
        expect.objectContaining({
          expectedHost: "https://gitlab.internal:8443",
          init: expect.objectContaining({ signal: expect.any(AbortSignal) }),
        }),
      );
      expect(result.current.branches(gitlab)).toEqual([
        expect.objectContaining({ name: "main", type: "remote" }),
      ]);
    });
  });
});

describe("useBranchesByURL request state", () => {
  it("dedupes concurrent ensure() calls for the same URL into a single fetch", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });

    const { result } = renderHook(() => useBranchesByURL("ws-1"));

    act(() => {
      result.current.ensure(REPO_A);
      result.current.ensure(REPO_A);
      result.current.ensure(REPO_A);
    });

    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
  });

  it("reports loading(url) true during fetch and false after settle", async () => {
    let resolveFetch: ((v: { branches: { name: string }[] }) => void) | null = null;
    fetchRepoBranchesMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const { result } = renderHook(() => useBranchesByURL("ws-1"));

    act(() => {
      result.current.ensure(REPO_A);
    });

    await waitFor(() => expect(result.current.loading(REPO_A)).toBe(true));

    act(() => {
      resolveFetch?.({ branches: [{ name: "main" }] });
    });

    await waitFor(() => expect(result.current.loading(REPO_A)).toBe(false));
    expect(result.current.branches(REPO_A)).toHaveLength(1);
  });
});

describe("useBranchesByURL cache behavior", () => {
  it("ignores ensure() with empty string and treats it as a clear", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });

    const { result } = renderHook(() => useBranchesByURL("ws-1"));

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));

    act(() => {
      result.current.ensure("");
    });

    // Passing "" should not trigger an additional fetch.
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
  });

  it("returns an empty array for an unknown URL", () => {
    const { result } = renderHook(() => useBranchesByURL("ws-1"));
    expect(result.current.branches("https://github.com/who/what")).toEqual([]);
    expect(result.current.loading("https://github.com/who/what")).toBe(false);
  });

  it("does not re-fetch when ensure() is called for an already-loaded URL", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });

    const { result } = renderHook(() => useBranchesByURL("ws-1"));

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));

    act(() => {
      result.current.ensure(REPO_A);
    });

    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
  });
});

describe("useBranchesByURL — failure & invalidation", () => {
  it("retains a failure for its URL without losing a successful sibling", async () => {
    const failure = new Error("GitHub rate limit exceeded");
    fetchRepoBranchesMock
      .mockResolvedValueOnce({ branches: [{ name: "main" }] })
      .mockRejectedValueOnce(failure);
    const { result } = renderHook(() => useBranchesByURL(WORKSPACE_ID));

    act(() => {
      result.current.ensure(REPO_A);
      result.current.ensure(REPO_B);
    });

    await waitFor(() => expect(result.current.loading(REPO_B)).toBe(false));
    expect(result.current.branches(REPO_A)).toMatchObject([{ name: "main" }]);
    expect(result.current.error(REPO_B)).toBe(failure);
  });
});

describe("useBranchesByURL — workspace scope", () => {
  it("clears branch data before fetching with a newly selected workspace credential", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useBranchesByURL(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );

    act(() => result.current.ensure(REPO_A));
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));

    rerender({ workspaceId: WORKSPACE_B });
    expect(result.current.branches(REPO_A)).toEqual([]);

    act(() => result.current.ensure(REPO_A));
    await waitFor(() => expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2));
    expect(fetchRepoBranchesMock).toHaveBeenNthCalledWith(
      1,
      WORKSPACE_ID,
      "acme",
      "site",
      expect.anything(),
    );
    expect(fetchRepoBranchesMock).toHaveBeenNthCalledWith(
      2,
      WORKSPACE_B,
      "acme",
      "site",
      expect.anything(),
    );
  });

  it("ignores metadata from a previous workspace after the replacement request starts", async () => {
    let resolveWorkspaceA: ((value: { branches: { name: string }[] }) => void) | undefined;
    let resolveWorkspaceB: ((value: { branches: { name: string }[] }) => void) | undefined;
    fetchRepoBranchesMock
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveWorkspaceA = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveWorkspaceB = resolve;
          }),
      );
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useBranchesByURL(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );

    act(() => result.current.ensure(REPO_A));
    await waitFor(() => expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1));

    rerender({ workspaceId: WORKSPACE_B });
    act(() => result.current.ensure(REPO_A));
    await waitFor(() => expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2));

    act(() => resolveWorkspaceB?.({ branches: [{ name: WORKSPACE_B_BRANCH }] }));
    await waitFor(() =>
      expect(result.current.branches(REPO_A)).toMatchObject([{ name: WORKSPACE_B_BRANCH }]),
    );

    act(() => resolveWorkspaceA?.({ branches: [{ name: "workspace-a" }] }));
    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.branches(REPO_A)).toMatchObject([{ name: WORKSPACE_B_BRANCH }]);
  });
});

describe("useBranchesByURL — workspace failure scope", () => {
  it("ignores a rejected request from a previous workspace after the replacement succeeds", async () => {
    let rejectWorkspaceA: ((reason?: unknown) => void) | undefined;
    let resolveWorkspaceB: ((value: { branches: { name: string }[] }) => void) | undefined;
    fetchRepoBranchesMock
      .mockImplementationOnce(
        () =>
          new Promise((_resolve, reject) => {
            rejectWorkspaceA = reject;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveWorkspaceB = resolve;
          }),
      );
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useBranchesByURL(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );

    act(() => result.current.ensure(REPO_A));
    await waitFor(() => expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1));

    rerender({ workspaceId: WORKSPACE_B });
    act(() => result.current.ensure(REPO_A));
    await waitFor(() => expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2));

    act(() => resolveWorkspaceB?.({ branches: [{ name: WORKSPACE_B_BRANCH }] }));
    await waitFor(() =>
      expect(result.current.branches(REPO_A)).toMatchObject([{ name: WORKSPACE_B_BRANCH }]),
    );

    act(() => rejectWorkspaceA?.(new Error("workspace A failed")));
    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.branches(REPO_A)).toMatchObject([{ name: WORKSPACE_B_BRANCH }]);
    expect(result.current.error(REPO_A)).toBeUndefined();
    expect(result.current.loading(REPO_A)).toBe(false);
  });
});

describe("useBranchesByURL — cache invalidation", () => {
  it("retries a failed fetch only after clear()", async () => {
    fetchRepoBranchesMock.mockRejectedValueOnce(new Error("network boom"));

    const { result } = renderHook(() => useBranchesByURL("ws-1"));

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.loading(REPO_A)).toBe(false));
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
    expect(result.current.branches(REPO_A)).toEqual([]);

    fetchRepoBranchesMock.mockResolvedValueOnce({ branches: [{ name: "main" }] });
    act(() => {
      result.current.ensure(REPO_A);
    });
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.clear(REPO_A);
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2);
  });

  it("accepts a PR URL and fetches branches against the underlying repo", async () => {
    // Regression: the hook used to call parseGitHubRepoUrl which rejects
    // `/pull/N` paths, so pasting a PR URL marked the URL as loaded with an
    // empty branches list — the branch picker stayed permanently empty even
    // though the repo itself has branches.
    fetchRepoBranchesMock.mockImplementation((workspace: string, owner: string, repo: string) => {
      expect(workspace).toBe("ws-1");
      expect(owner).toBe("acme");
      expect(repo).toBe("site");
      return Promise.resolve({ branches: [{ name: "main" }, { name: "feature/x" }] });
    });
    const { result } = renderHook(() => useBranchesByURL("ws-1"));
    const PR_URL = "https://github.com/acme/site/pull/42";

    act(() => {
      result.current.ensure(PR_URL);
    });

    await waitFor(() => expect(result.current.branches(PR_URL)).toHaveLength(2));
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
  });

  it("accepts an issue URL and fetches branches against the underlying repo", async () => {
    fetchRepoBranchesMock.mockImplementation((workspace: string, owner: string, repo: string) => {
      expect(workspace).toBe("ws-1");
      expect(owner).toBe("acme");
      expect(repo).toBe("site");
      return Promise.resolve({ branches: [{ name: "main" }, { name: "fix/issue" }] });
    });
    const { result } = renderHook(() => useBranchesByURL("ws-1"));
    const issueURL = "https://github.com/acme/site/issues/1456";

    act(() => {
      result.current.ensure(issueURL);
    });

    await waitFor(() => expect(result.current.branches(issueURL)).toHaveLength(2));
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
  });

  it("clear(url) lets the next ensure() refetch a successfully loaded URL", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });

    const { result } = renderHook(() => useBranchesByURL("ws-1"));

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));

    act(() => {
      result.current.clear(REPO_A);
    });
    expect(result.current.branches(REPO_A)).toEqual([]);

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2));
  });
});
