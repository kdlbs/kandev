import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor, act } from "@testing-library/react";
import { IconInbox } from "@tabler/icons-react";
import type { Issue, MR, MRSearchPage, IssueSearchPage } from "@/lib/types/gitlab";
import type { PresetOption } from "./presets";

const searchUserMRsMock = vi.fn<[unknown], Promise<MRSearchPage | null>>();
const searchUserIssuesMock = vi.fn<[unknown], Promise<IssueSearchPage | null>>();

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  searchUserMRs: (args: unknown) => searchUserMRsMock(args),
  searchUserIssues: (args: unknown) => searchUserIssuesMock(args),
}));

import { pickFilter, useGitLabSearch } from "./use-gitlab-search";

afterEach(() => cleanup());

const PRESET_REVIEW = "review_requested";
const PRESET_ASSIGNED = "assigned";
const CUSTOM_QUERY = "labels=bug";

const PRESETS: PresetOption[] = [
  {
    value: PRESET_REVIEW,
    label: "Review requested",
    filter: PRESET_REVIEW,
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: PRESET_ASSIGNED,
    label: "Assigned",
    filter: "assigned_to_me",
    group: "inbox",
    icon: IconInbox,
  },
];

const EMPTY_PAGE: MRSearchPage = { mrs: [], total_count: 0, page: 1, per_page: 25 };

describe("pickFilter", () => {
  it("uses preset filter when custom query is empty", () => {
    expect(pickFilter(PRESETS, PRESET_REVIEW, "")).toEqual({
      filter: PRESET_REVIEW,
      customQuery: "",
    });
  });

  it("routes custom query and drops preset filter", () => {
    expect(pickFilter(PRESETS, PRESET_REVIEW, CUSTOM_QUERY)).toEqual({
      filter: "",
      customQuery: CUSTOM_QUERY,
    });
  });

  it("trims whitespace-only custom query and falls back to preset", () => {
    expect(pickFilter(PRESETS, PRESET_ASSIGNED, "   ")).toEqual({
      filter: "assigned_to_me",
      customQuery: "",
    });
  });

  it("falls back to empty filter when preset is unknown", () => {
    expect(pickFilter(PRESETS, "missing", "")).toEqual({ filter: "", customQuery: "" });
  });
});

function fakeMR(overrides: Partial<MR> = {}): MR {
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
    ...overrides,
  };
}

function fakeIssue(overrides: Partial<Issue> = {}): Issue {
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
    ...overrides,
  };
}

function resetMocks() {
  searchUserMRsMock.mockReset();
  searchUserIssuesMock.mockReset();
}

describe("useGitLabSearch — fetch wiring", () => {
  beforeEach(resetMocks);

  it("forwards preset filter to MR API", async () => {
    searchUserMRsMock.mockResolvedValue(EMPTY_PAGE);
    renderHook(() => useGitLabSearch<MR>("mr", PRESETS, PRESET_REVIEW, ""));
    await waitFor(() => expect(searchUserMRsMock).toHaveBeenCalled());
    const args = searchUserMRsMock.mock.calls[0][0] as Record<string, unknown>;
    expect(args.filter).toBe(PRESET_REVIEW);
    expect(args.customQuery).toBe("");
    expect(args.page).toBe(1);
  });

  it("forwards custom query and drops preset filter", async () => {
    searchUserMRsMock.mockResolvedValue(EMPTY_PAGE);
    renderHook(() => useGitLabSearch<MR>("mr", PRESETS, PRESET_REVIEW, CUSTOM_QUERY));
    await waitFor(() => expect(searchUserMRsMock).toHaveBeenCalled());
    const args = searchUserMRsMock.mock.calls[0][0] as Record<string, unknown>;
    expect(args.filter).toBe("");
    expect(args.customQuery).toBe(CUSTOM_QUERY);
  });

  it("dispatches to issues endpoint when kind is issue", async () => {
    const issue = fakeIssue();
    searchUserIssuesMock.mockResolvedValue({
      issues: [issue],
      total_count: 1,
      page: 1,
      per_page: 25,
    });
    const { result } = renderHook(() =>
      useGitLabSearch<Issue>("issue", PRESETS, PRESET_ASSIGNED, ""),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items).toEqual([issue]);
    expect(searchUserMRsMock).not.toHaveBeenCalled();
  });
});

describe("useGitLabSearch — state", () => {
  beforeEach(resetMocks);

  it("populates items and total on success", async () => {
    const mr = fakeMR();
    searchUserMRsMock.mockResolvedValue({ mrs: [mr], total_count: 1, page: 1, per_page: 25 });
    const { result } = renderHook(() => useGitLabSearch<MR>("mr", PRESETS, PRESET_ASSIGNED, ""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items).toEqual([mr]);
    expect(result.current.total).toBe(1);
    expect(result.current.error).toBeNull();
  });

  it("surfaces error message without items", async () => {
    searchUserMRsMock.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => useGitLabSearch<MR>("mr", PRESETS, PRESET_ASSIGNED, ""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe("boom");
    expect(result.current.items).toEqual([]);
  });

  it("filters items by project_path client-side", async () => {
    const a = fakeMR({ project_path: "acme/api" });
    const b = fakeMR({ project_path: "acme/web" });
    searchUserMRsMock.mockResolvedValue({ mrs: [a, b], total_count: 2, page: 1, per_page: 25 });
    const { result } = renderHook(() =>
      useGitLabSearch<MR>("mr", PRESETS, PRESET_ASSIGNED, "", "acme/web"),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items.map((m) => m.project_path)).toEqual(["acme/web"]);
    expect(result.current.rawItems.length).toBe(2);
  });
});

describe("useGitLabSearch — pagination & sequencing", () => {
  beforeEach(resetMocks);

  it("resets page to 1 when preset changes", async () => {
    searchUserMRsMock.mockResolvedValue(EMPTY_PAGE);
    const { result, rerender } = renderHook(
      ({ p }: { p: string }) => useGitLabSearch<MR>("mr", PRESETS, p, ""),
      { initialProps: { p: PRESET_ASSIGNED } },
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.setPage(3));
    await waitFor(() => expect(result.current.page).toBe(3));
    rerender({ p: PRESET_REVIEW });
    await waitFor(() => expect(result.current.page).toBe(1));
  });

  it("drops stale responses when filter changes mid-flight", async () => {
    let resolveFirst: (v: MRSearchPage) => void = () => {};
    searchUserMRsMock.mockReturnValueOnce(
      new Promise<MRSearchPage>((res) => {
        resolveFirst = res;
      }),
    );
    const second = fakeMR({ title: "second" });
    searchUserMRsMock.mockResolvedValueOnce({
      mrs: [second],
      total_count: 1,
      page: 1,
      per_page: 25,
    });

    const { result, rerender } = renderHook(
      ({ p }: { p: string }) => useGitLabSearch<MR>("mr", PRESETS, p, ""),
      { initialProps: { p: PRESET_ASSIGNED } },
    );
    rerender({ p: PRESET_REVIEW });
    await waitFor(() => expect(result.current.items).toEqual([second]));
    resolveFirst({
      mrs: [fakeMR({ title: "first" }), fakeMR()],
      total_count: 2,
      page: 1,
      per_page: 25,
    });
    await new Promise((r) => setTimeout(r, 10));
    expect(result.current.items).toEqual([second]);
  });
});
