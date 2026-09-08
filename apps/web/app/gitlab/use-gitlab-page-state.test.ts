import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { Issue, IssueSearchPage } from "@/lib/types/gitlab";
import { ISSUE_PRESETS, MR_PRESETS } from "@/components/gitlab/my-gitlab/presets";
import {
  __resetSnapshotForTests,
  type SavedPreset,
} from "@/components/gitlab/my-gitlab/use-saved-presets";
import { resetKnownProjectsStore } from "@/components/gitlab/my-gitlab/use-known-projects";
import type { SidebarSelection } from "@/components/gitlab/my-gitlab/presets-sidebar";

const searchUserMRsMock = vi.fn<(input: unknown) => Promise<unknown>>();
const searchUserIssuesMock = vi.fn<(input: unknown) => Promise<IssueSearchPage | null>>();
vi.mock("@/lib/api/domains/gitlab-api", () => ({
  searchUserMRs: (args: unknown) => searchUserMRsMock(args),
  searchUserIssues: (args: unknown) => searchUserIssuesMock(args),
}));

const fetchUserSettingsMock = vi.fn();
const updateUserSettingsMock = vi.fn();
vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: (...args: unknown[]) => fetchUserSettingsMock(...args),
  updateUserSettings: (...args: unknown[]) => updateUserSettingsMock(...args),
}));

import {
  buildProjectOptionsResetKey,
  trimGitLabMilestone,
  useGitLabPageState,
  useProjectOptions,
} from "./use-gitlab-page-state";

afterEach(() => cleanup());

const WORKSPACE_ID = "ws-1";
const CREATED_AT = "2026-01-01T00:00:00Z";

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
    milestone: "",
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function issuePage(items: Issue[]): IssueSearchPage {
  return { issues: items, total_count: items.length, page: 1, per_page: 25 };
}

beforeEach(() => {
  vi.clearAllMocks();
  resetKnownProjectsStore();
  __resetSnapshotForTests();
  fetchUserSettingsMock.mockResolvedValue({ settings: { gitlab_saved_presets: [] } });
  updateUserSettingsMock.mockResolvedValue({ settings: {} });
  searchUserMRsMock.mockResolvedValue({ mrs: [], total_count: 0, page: 1, per_page: 25 });
  searchUserIssuesMock.mockResolvedValue(issuePage([]));
});

describe("trimGitLabMilestone", () => {
  it("trims ordinary leading/trailing whitespace", () => {
    expect(trimGitLabMilestone("  v1.0  ")).toBe("v1.0");
  });

  it("trims a leading BOM (\uFEFF) and a trailing NBSP (\u00A0)", () => {
    expect(trimGitLabMilestone("\uFEFFv1.0\u00A0")).toBe("v1.0");
  });

  it("trims a trailing NEL (\u0085), which the bare \\s regex class omits", () => {
    expect(trimGitLabMilestone("v1.0\u0085")).toBe("v1.0");
  });

  it("preserves internal whitespace", () => {
    expect(trimGitLabMilestone("  Sprint 42  ")).toBe("Sprint 42");
  });

  it("returns empty for a whitespace-only value including a NEL", () => {
    expect(trimGitLabMilestone("   \u0085  ")).toBe("");
  });

  // Mirrors the Go side's TestTrimGitLabWhitespace_MatchesNormativeSet
  // (client_helpers_milestone_test.go) code point for code point: the
  // normative set is Unicode White_Space together with U+FEFF, on BOTH
  // sides, and the two languages' own defaults disagree with it and with
  // each other in both directions (Scenario 27). Asserting only a handful
  // of ad-hoc cases on one side does not establish the cross-language
  // agreement the spec requires.
  it("matches the normative set for every code point in the Go-side conformance table", () => {
    const trimmed = [
      0x0009, 0x000a, 0x000b, 0x000c, 0x000d, 0x0020, 0x0085, 0x00a0, 0x1680, 0x2000, 0x2028,
      0x2029, 0x202f, 0x205f, 0x3000, 0xfeff,
    ];
    for (const cp of trimmed) {
      expect(trimGitLabMilestone(String.fromCodePoint(cp))).toBe("");
    }

    // Preserved: not part of Unicode White_Space or U+FEFF.
    const preserved = [0x200b, 0x180e, 0x2060, 0x00b7];
    for (const cp of preserved) {
      const ch = String.fromCodePoint(cp);
      expect(trimGitLabMilestone(ch)).toBe(ch);
    }
  });
});

describe("buildProjectOptionsResetKey", () => {
  it("keeps delimiter-containing query and milestone values distinct", () => {
    const selection: SidebarSelection = { kind: "issue", source: "preset", id: "assigned" };

    const queryWithDelimiter = buildProjectOptionsResetKey(selection, "labels=backend:v1", "Q2");
    const milestoneWithDelimiter = buildProjectOptionsResetKey(
      selection,
      "labels=backend",
      "v1:Q2",
    );

    expect(queryWithDelimiter).not.toBe(milestoneWithDelimiter);
  });
});

describe("useProjectOptions — ordinary pagination (key unchanged)", () => {
  beforeEach(() => resetKnownProjectsStore());

  const SELECTION: SidebarSelection = { kind: "issue", source: "preset", id: "assigned" };

  it("keeps projects seen on an earlier page while a same-key page turn is loading", () => {
    const items = [fakeIssue({ project_path: "acme/api" })];
    const { result, rerender } = renderHook(
      (props: { loading: boolean; items: Issue[] }) =>
        useProjectOptions({
          selection: SELECTION,
          committedQuery: "",
          milestone: "",
          items: props.items,
          loading: props.loading,
          projectFilter: "",
        }),
      { initialProps: { loading: false, items } },
    );
    expect(result.current).toEqual(["acme/api"]);

    // Page 2 is loading: same key, so the accumulator must not clear.
    rerender({ loading: true, items: [] });
    expect(result.current).toEqual(["acme/api"]);

    // Page 2 resolves with a different project; it accumulates on top.
    rerender({ loading: false, items: [fakeIssue({ project_path: "acme/web" })] });
    expect(result.current).toEqual(["acme/api", "acme/web"]);
  });
});

describe("useGitLabPageState", () => {
  it("does not leak the previous milestone's projects into the dropdown while the new milestone's fetch is in flight (Scenario 29)", async () => {
    searchUserIssuesMock.mockResolvedValueOnce(
      issuePage([fakeIssue({ project_path: "acme/api" })]),
    );
    let resolveNext: (v: IssueSearchPage) => void = () => {};
    searchUserIssuesMock.mockReturnValueOnce(
      new Promise<IssueSearchPage>((res) => {
        resolveNext = res;
      }),
    );

    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    await waitFor(() => expect(result.current.projectOptions).toEqual(["acme/api"]));

    act(() => result.current.setMilestone("Next"));
    act(() => result.current.onCommitMilestone());

    // The new milestone's fetch is in flight; the previous milestone's
    // project must not still be offered.
    expect(result.current.projectOptions).toEqual([]);

    await act(async () => {
      resolveNext(issuePage([fakeIssue({ project_path: "acme/web" })]));
      await Promise.resolve();
    });
    await waitFor(() => expect(result.current.projectOptions).toEqual(["acme/web"]));
    expect(result.current.projectOptions).not.toContain("acme/api");
  });

  it("clears the milestone draft and committed value when the selection changes, including switching to the MR view", async () => {
    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    act(() => result.current.setMilestone("Next"));
    act(() => result.current.onCommitMilestone());
    await waitFor(() => expect(result.current.committedMilestone).toBe("Next"));

    act(() => result.current.onSelect({ kind: "mr", source: "preset", id: MR_PRESETS[0].value }));
    expect(result.current.milestone).toBe("");
    expect(result.current.committedMilestone).toBe("");
    expect(result.current.showMilestoneFilter).toBe(false);
  });

  it("onCommitMilestone normalizes the visible input via the trim helper, resets to page 1, and issues exactly one request (Scenario 7)", async () => {
    searchUserIssuesMock.mockResolvedValue(issuePage([]));
    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    await waitFor(() => expect(result.current.search.loading).toBe(false));

    act(() => result.current.search.setPage(3));
    await waitFor(() => expect(result.current.search.page).toBe(3));

    const callsBeforeCommit = searchUserIssuesMock.mock.calls.length;
    act(() => result.current.setMilestone("  Next  "));
    act(() => result.current.onCommitMilestone());

    expect(result.current.milestone).toBe("Next");
    expect(result.current.committedMilestone).toBe("Next");
    await waitFor(() => expect(result.current.search.page).toBe(1));

    // Committing and resetting the page together must fire exactly one
    // request for the new value — adding milestone to the existing
    // [preset, customQuery, kind] reset effect would instead fire twice
    // (once for the stale page, once for page 1); see spec "Client
    // function, and where trimming happens".
    await waitFor(() => expect(searchUserIssuesMock).toHaveBeenCalledTimes(callsBeforeCommit + 1));
    const lastCall = searchUserIssuesMock.mock.calls.at(-1)?.[0] as {
      page: number;
      milestone?: string;
    };
    expect(lastCall.page).toBe(1);
    expect(lastCall.milestone).toBe("Next");
  });

  it("resets to page 1 when selecting a sidebar preset clears a committed milestone (Scenario 6, clause 3)", async () => {
    searchUserIssuesMock.mockResolvedValue(issuePage([]));
    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    act(() => result.current.setMilestone("Next"));
    act(() => result.current.onCommitMilestone());
    await waitFor(() => expect(result.current.committedMilestone).toBe("Next"));

    act(() => result.current.search.setPage(3));
    await waitFor(() => expect(result.current.search.page).toBe(3));

    // Re-select the SAME preset: effectivePreset/customQuery/kind are all
    // unchanged, so useGitLabSearch's `[preset, customQuery, kind]` reset
    // effect will not fire on its own — only the milestone clears.
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    expect(result.current.committedMilestone).toBe("");
    await waitFor(() => expect(result.current.search.page).toBe(1));
  });
});

describe("useGitLabPageState — saved query save", () => {
  it("canSaveCurrent and suggestedLabel consider the milestone alongside the query and project filter", async () => {
    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    expect(result.current.canSaveCurrent).toBe(false);

    act(() => result.current.setMilestone("Next"));
    act(() => result.current.onCommitMilestone());
    expect(result.current.canSaveCurrent).toBe(true);
    expect(result.current.suggestedLabel).toBe("Next");
  });

  it("suggestedLabel precedence ladder: custom query beats project filter beats milestone beats the default (Scenario 25)", async () => {
    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    expect(result.current.suggestedLabel).toBe("Saved query");

    act(() => result.current.setMilestone("Next"));
    act(() => result.current.onCommitMilestone());
    expect(result.current.suggestedLabel).toBe("Next");

    act(() => result.current.setProjectFilter("group/app"));
    expect(result.current.suggestedLabel).toBe("In group/app");

    act(() => result.current.setCustomQuery("state=closed"));
    act(() => result.current.commitCustomQuery());
    await waitFor(() => expect(result.current.committedQuery).toBe("state=closed"));
    expect(result.current.suggestedLabel).toBe("state=closed");
  });

  it("onConfirmSave writes the committed (trimmed) milestone and the effective preset, never the raw draft", async () => {
    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    act(() => result.current.setMilestone("  Next  "));
    act(() => result.current.onCommitMilestone());
    await waitFor(() => expect(result.current.committedMilestone).toBe("Next"));

    await act(async () => {
      result.current.onConfirmSave("My saved query");
      await Promise.resolve();
    });

    await waitFor(() => expect(updateUserSettingsMock).toHaveBeenCalled());
    const payload = updateUserSettingsMock.mock.calls.at(-1)?.[0] as {
      gitlab_saved_presets: SavedPreset[];
    };
    const saved = payload.gitlab_saved_presets.at(-1);
    expect(saved?.label).toBe("My saved query");
    expect(saved?.milestone).toBe("Next");
    expect(saved?.preset).toBe("assigned");
    expect(saved?.customQuery).toBe("");
  });
});

describe("useGitLabPageState — saved query restore", () => {
  it("restores both the milestone and the effective preset (including a preset-only query with no custom text) when the saved query is selected", async () => {
    const created: SavedPreset = {
      id: "g_1",
      kind: "issue",
      label: "Next sprint",
      customQuery: "",
      projectFilter: "",
      milestone: "Next",
      preset: "assigned",
      createdAt: CREATED_AT,
    };
    fetchUserSettingsMock.mockResolvedValue({ settings: { gitlab_saved_presets: [created] } });
    searchUserIssuesMock.mockResolvedValue(issuePage([fakeIssue({ project_path: "acme/api" })]));

    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    await waitFor(() => expect(result.current.savedPresets).toHaveLength(1));

    act(() => result.current.onSelect({ kind: "issue", source: "saved", id: "g_1" }));
    expect(result.current.committedMilestone).toBe("Next");
    expect(result.current.committedQuery).toBe("");
    // Preset-only saved query with no custom query text: the original
    // preset filter (persisted as `preset`) must still drive the fetch.
    await waitFor(() => expect(searchUserIssuesMock).toHaveBeenCalled());
    const lastCall = searchUserIssuesMock.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(lastCall.filter).toBe("assigned_to_me");
  });

  it("resets to page 1 when selecting a saved query that differs from the current state only in its milestone (Scenario 6, clause 2)", async () => {
    const created: SavedPreset = {
      id: "g_1",
      kind: "issue",
      label: "Next sprint",
      customQuery: "",
      projectFilter: "",
      milestone: "Next",
      preset: "assigned",
      createdAt: CREATED_AT,
    };
    fetchUserSettingsMock.mockResolvedValue({ settings: { gitlab_saved_presets: [created] } });

    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    await waitFor(() => expect(result.current.savedPresets).toHaveLength(1));

    // Same effective preset ("assigned"), same empty custom query, same kind
    // as the saved query below — only the milestone will differ, so the
    // `[preset, customQuery, kind]` reset effect in useGitLabSearch will not
    // fire on its own.
    act(() => result.current.onSelect({ kind: "issue", source: "preset", id: "assigned" }));
    await waitFor(() => expect(result.current.search.loading).toBe(false));
    act(() => result.current.search.setPage(3));
    await waitFor(() => expect(result.current.search.page).toBe(3));

    act(() => result.current.onSelect({ kind: "issue", source: "saved", id: "g_1" }));
    expect(result.current.committedMilestone).toBe("Next");
    await waitFor(() => expect(result.current.search.page).toBe(1));
  });
});

describe("useGitLabPageState — saved query delete", () => {
  it("clears milestone and resets to page 1 only when the deleted saved query was the active selection", async () => {
    const other: SavedPreset = {
      id: "g_other",
      kind: "issue",
      label: "Other",
      customQuery: "labels=bug",
      projectFilter: "",
      milestone: "",
      preset: "",
      createdAt: CREATED_AT,
    };
    const selected: SavedPreset = {
      id: "g_selected",
      kind: "issue",
      label: "Selected",
      customQuery: "",
      projectFilter: "",
      milestone: "Next",
      preset: "assigned",
      createdAt: CREATED_AT,
    };
    fetchUserSettingsMock.mockResolvedValue({
      settings: { gitlab_saved_presets: [other, selected] },
    });

    const { result } = renderHook(() => useGitLabPageState(true, WORKSPACE_ID));
    await waitFor(() => expect(result.current.savedPresets).toHaveLength(2));

    act(() => result.current.onSelect({ kind: "issue", source: "saved", id: "g_selected" }));
    await waitFor(() => expect(result.current.committedMilestone).toBe("Next"));
    act(() => result.current.search.setPage(2));
    await waitFor(() => expect(result.current.search.page).toBe(2));

    // Deleting an unrelated saved query leaves the active scope untouched.
    act(() => result.current.onDeleteSaved("g_other"));
    expect(result.current.committedMilestone).toBe("Next");
    expect(result.current.search.page).toBe(2);

    // Deleting the currently-selected saved query clears the scope and
    // resets the page in the same batch.
    act(() => result.current.onDeleteSaved("g_selected"));
    expect(result.current.committedMilestone).toBe("");
    await waitFor(() => expect(result.current.search.page).toBe(1));
    expect(result.current.selection).toEqual({
      kind: "issue",
      source: "preset",
      id: ISSUE_PRESETS[0]?.value ?? "",
    });
  });
});
