import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { Issue, MR, MRSearchPage, IssueSearchPage } from "@/lib/types/gitlab";

const searchUserMRsMock = vi.fn<[unknown], Promise<MRSearchPage | null>>();
const searchUserIssuesMock = vi.fn<[unknown], Promise<IssueSearchPage | null>>();

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  searchUserMRs: (args: unknown) => searchUserMRsMock(args),
  searchUserIssues: (args: unknown) => searchUserIssuesMock(args),
}));

import { useGitLabUserIssues, useGitLabUserMRs } from "./use-user-search";

afterEach(() => cleanup());

function fakeMR(): MR {
  return {
    id: 1,
    iid: 1,
    project_id: 1,
    title: "",
    url: "",
    web_url: "",
    state: "opened",
    head_branch: "feat",
    head_sha: "",
    base_branch: "main",
    author_username: "alice",
    project_namespace: "acme",
    project_path: "acme/api",
    body: "",
    draft: false,
    merge_status: "",
    has_conflicts: false,
    additions: 0,
    deletions: 0,
    reviewers: [],
    assignees: [],
    created_at: "",
    updated_at: "",
  };
}

function fakeIssue(): Issue {
  return {
    id: 1,
    iid: 1,
    project_id: 1,
    title: "",
    body: "",
    url: "",
    web_url: "",
    state: "opened",
    author_username: "alice",
    project_namespace: "acme",
    project_path: "acme/api",
    labels: [],
    assignees: [],
    created_at: "",
    updated_at: "",
  };
}

describe("useGitLabUserMRs", () => {
  beforeEach(() => {
    searchUserMRsMock.mockReset();
  });

  it("forwards filter, query, and perPage to the API", async () => {
    searchUserMRsMock.mockResolvedValueOnce({ mrs: [], total_count: 0, page: 1, per_page: 25 });
    renderHook(() => useGitLabUserMRs("authored", "labels=bug", 25));
    await waitFor(() => expect(searchUserMRsMock).toHaveBeenCalledTimes(1));
    expect(searchUserMRsMock).toHaveBeenCalledWith({
      filter: "authored",
      customQuery: "labels=bug",
      perPage: 25,
    });
  });

  it("populates items on success and clears loading", async () => {
    const mr = fakeMR();
    searchUserMRsMock.mockResolvedValueOnce({ mrs: [mr], total_count: 1, page: 1, per_page: 50 });
    const { result } = renderHook(() => useGitLabUserMRs("a", ""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items).toEqual([mr]);
    expect(result.current.error).toBeNull();
  });

  it("surfaces an error message and empty items on rejection", async () => {
    searchUserMRsMock.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useGitLabUserMRs("a", ""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe("boom");
    expect(result.current.items).toEqual([]);
  });

  it("ignores stale responses when inputs change mid-flight", async () => {
    let resolveFirst: (v: MRSearchPage) => void = () => {};
    searchUserMRsMock.mockReturnValueOnce(
      new Promise<MRSearchPage>((res) => {
        resolveFirst = res;
      }),
    );
    const second = fakeMR();
    searchUserMRsMock.mockResolvedValueOnce({
      mrs: [second],
      total_count: 1,
      page: 1,
      per_page: 50,
    });

    const { result, rerender } = renderHook(
      ({ filter }: { filter: string }) => useGitLabUserMRs(filter, ""),
      { initialProps: { filter: "a" } },
    );
    rerender({ filter: "b" });
    await waitFor(() => expect(result.current.items).toEqual([second]));

    // The first request resolves *after* the swap — it must NOT clobber
    // the newer items list.
    resolveFirst({
      mrs: [fakeMR(), fakeMR()],
      total_count: 2,
      page: 1,
      per_page: 50,
    });
    await new Promise((r) => setTimeout(r, 10));
    expect(result.current.items).toEqual([second]);
  });
});

describe("useGitLabUserIssues", () => {
  beforeEach(() => {
    searchUserIssuesMock.mockReset();
  });

  it("forwards args and returns items", async () => {
    const issue = fakeIssue();
    searchUserIssuesMock.mockResolvedValueOnce({
      issues: [issue],
      total_count: 1,
      page: 1,
      per_page: 50,
    });
    const { result } = renderHook(() => useGitLabUserIssues("assigned_to_me", ""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items).toEqual([issue]);
  });

  it("handles rejection without populating items", async () => {
    searchUserIssuesMock.mockRejectedValueOnce(new Error("net"));
    const { result } = renderHook(() => useGitLabUserIssues("a", ""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe("net");
    expect(result.current.items).toEqual([]);
  });
});
