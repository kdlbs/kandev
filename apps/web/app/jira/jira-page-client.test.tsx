import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { useJiraFilterState } from "@/components/jira/my-jira/use-jira-filter-state";
import {
  reconcileStatusesForQuery,
  useProjectStatuses,
} from "@/components/jira/my-jira/use-project-statuses";

const PROJECT_KEY = "PROJ";
const JIRA_PROJECT_KEY = "CLIP";
const DEFAULT_VIEW_ID = "custom:default";
const DEFAULT_VIEW_NAME = "Default";
const UNASSIGNED_VIEW_ID = "builtin:unassigned";
const READY_STATUS = "Ready";
const CUSTOM_VIEW_JQL = `project = ${JIRA_PROJECT_KEY} ORDER BY priority DESC`;
const CUSTOM_STATUS_JQL = `project = ${JIRA_PROJECT_KEY} AND status = ${READY_STATUS} ORDER BY priority DESC`;

const listJiraProjectStatusesMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));

vi.mock("@/lib/api/domains/jira-api", () => ({
  listJiraProjectStatuses: listJiraProjectStatusesMock,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

function makeDefaultView(customJql?: string) {
  return {
    id: DEFAULT_VIEW_ID,
    name: DEFAULT_VIEW_NAME,
    filters: {
      projectKeys: [JIRA_PROJECT_KEY],
      statuses: [READY_STATUS],
      assignee: "anyone",
      searchText: "",
      sort: "priority",
    },
    customJql,
  };
}

function mockDefaultViewSettings(customJql?: string) {
  vi.mocked(fetchUserSettings).mockResolvedValueOnce({
    settings: {
      jira_saved_views: [makeDefaultView(customJql)],
      jira_default_view_id: DEFAULT_VIEW_ID,
    },
  } as Awaited<ReturnType<typeof fetchUserSettings>>);
}

function resetPageMocks() {
  listJiraProjectStatusesMock.mockReset();
  vi.mocked(updateUserSettings).mockResolvedValue({ settings: {} } as Awaited<
    ReturnType<typeof updateUserSettings>
  >);
}

describe("Jira page saved view hydration", () => {
  beforeEach(resetPageMocks);

  it("does not replace a manual selection when settings hydrate later", async () => {
    const settings = deferred<Awaited<ReturnType<typeof fetchUserSettings>>>();
    vi.mocked(fetchUserSettings).mockReturnValueOnce(settings.promise);
    const { result } = renderHook(() => useJiraFilterState(PROJECT_KEY));

    expect(result.current.initialSelectionResolved).toBe(false);
    act(() => result.current.selectView(UNASSIGNED_VIEW_ID));
    settings.resolve({
      settings: {
        jira_saved_views: [makeDefaultView('project = OTHER AND text ~ "custom query"')],
        jira_default_view_id: DEFAULT_VIEW_ID,
      },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    await waitFor(() => expect(result.current.initialSelectionResolved).toBe(true));
    expect(result.current.activeViewId).toBe(UNASSIGNED_VIEW_ID);
    expect(result.current.filters.assignee).toBe("unassigned");
    expect(result.current.customJql).toBeNull();
  });

  it("restores custom JQL before marking initial selection resolved", async () => {
    mockDefaultViewSettings(CUSTOM_VIEW_JQL);
    const { result } = renderHook(() => useJiraFilterState(PROJECT_KEY));

    expect(result.current.initialSelectionResolved).toBe(false);
    await waitFor(() => expect(result.current.initialSelectionResolved).toBe(true));
    expect(result.current.activeViewId).toBe(DEFAULT_VIEW_ID);
    expect(result.current.filters).toEqual({
      projectKeys: [JIRA_PROJECT_KEY],
      statuses: [READY_STATUS],
      assignee: "anyone",
      searchText: "",
      sort: "priority",
    });
    expect(result.current.customJql).toBe(CUSTOM_VIEW_JQL);
    expect(result.current.showJqlEditor).toBe(true);
  });
});

describe("Jira page status lookup", () => {
  beforeEach(resetPageMocks);

  it("preserves a saved custom-JQL default when its status lookup fails", async () => {
    listJiraProjectStatusesMock.mockRejectedValueOnce(new Error("status lookup unavailable"));
    mockDefaultViewSettings(CUSTOM_STATUS_JQL);

    const { result } = renderHook(() => {
      const filterState = useJiraFilterState(PROJECT_KEY);
      const statuses = useProjectStatuses(filterState.filters.projectKeys, "workspace");
      return { filterState, statuses };
    });
    await waitFor(() => {
      expect(result.current.filterState.initialSelectionResolved).toBe(true);
      expect(result.current.statuses.loaded).toBe(true);
    });
    expect(result.current.statuses.options).toEqual([]);

    const reconciled = reconcileStatusesForQuery(
      result.current.statuses.loaded,
      result.current.filterState.customJql,
      result.current.filterState.filters.statuses,
      result.current.statuses.options,
      result.current.statuses.authoritative,
    );
    if (reconciled !== result.current.filterState.filters.statuses) {
      act(() =>
        result.current.filterState.updateFilters({
          ...result.current.filterState.filters,
          statuses: reconciled,
        }),
      );
    }

    expect(result.current.filterState.customJql).toBe(CUSTOM_STATUS_JQL);
    expect(result.current.filterState.effectiveJql).toBe(CUSTOM_STATUS_JQL);
    expect(result.current.filterState.filters.statuses).toEqual([READY_STATUS]);
  });

  it("preserves structured saved statuses when their status lookup fails", async () => {
    listJiraProjectStatusesMock.mockRejectedValueOnce(new Error("status lookup unavailable"));
    mockDefaultViewSettings();

    const { result } = renderHook(() => {
      const filterState = useJiraFilterState(PROJECT_KEY);
      const statuses = useProjectStatuses(filterState.filters.projectKeys, "workspace");
      return { filterState, statuses };
    });
    await waitFor(() => {
      expect(result.current.filterState.initialSelectionResolved).toBe(true);
      expect(result.current.statuses.loaded).toBe(true);
    });
    expect(result.current.statuses.authoritative).toBe(false);

    const reconciled = reconcileStatusesForQuery(
      result.current.statuses.loaded,
      result.current.filterState.customJql,
      result.current.filterState.filters.statuses,
      result.current.statuses.options,
      result.current.statuses.authoritative,
    );
    if (reconciled !== result.current.filterState.filters.statuses) {
      act(() =>
        result.current.filterState.updateFilters({
          ...result.current.filterState.filters,
          statuses: reconciled,
        }),
      );
    }

    expect(result.current.filterState.customJql).toBeNull();
    expect(result.current.filterState.effectiveJql).toContain(`status in ("${READY_STATUS}")`);
    expect(result.current.filterState.filters.statuses).toEqual([READY_STATUS]);
  });
});

describe("Jira page default mutations", () => {
  beforeEach(resetPageMocks);

  it("changes the future default without changing the current view", async () => {
    mockDefaultViewSettings();
    const { result } = renderHook(() => useJiraFilterState(PROJECT_KEY));
    await waitFor(() => expect(result.current.activeViewId).toBe(DEFAULT_VIEW_ID));

    await act(async () => {
      await result.current.setDefaultView(UNASSIGNED_VIEW_ID);
    });

    expect(result.current.defaultViewId).toBe(UNASSIGNED_VIEW_ID);
    expect(result.current.activeViewId).toBe(DEFAULT_VIEW_ID);
    expect(result.current.filters).toMatchObject({
      projectKeys: [JIRA_PROJECT_KEY],
      assignee: "anyone",
    });
  });

  it("returns to Assigned to me after deleting the active default", async () => {
    mockDefaultViewSettings();
    const { result } = renderHook(() => useJiraFilterState(PROJECT_KEY));
    await waitFor(() => expect(result.current.activeViewId).toBe(DEFAULT_VIEW_ID));

    await act(async () => {
      await result.current.deleteView(DEFAULT_VIEW_ID);
    });

    expect(updateUserSettings).toHaveBeenCalledWith({
      jira_saved_views: [],
      jira_default_view_id: "",
    });
    expect(result.current.defaultViewId).toBe("");
    expect(result.current.activeViewId).toBe("builtin:assigned");
    expect(result.current.filters).toMatchObject({
      projectKeys: [PROJECT_KEY],
      assignee: "me",
    });
  });

  it("keeps a later view selection when saving the prior view finishes", async () => {
    mockDefaultViewSettings();
    const { result } = renderHook(() => useJiraFilterState(PROJECT_KEY));
    await waitFor(() => expect(result.current.activeViewId).toBe(DEFAULT_VIEW_ID));

    const write = deferred<Awaited<ReturnType<typeof updateUserSettings>>>();
    vi.mocked(updateUserSettings).mockReturnValueOnce(write.promise);
    let savePromise!: Promise<unknown>;
    act(() => {
      savePromise = result.current.saveCurrentAsView("Saved snapshot");
    });
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalled());

    act(() => result.current.selectView(UNASSIGNED_VIEW_ID));
    await act(async () => {
      write.resolve({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);
      await savePromise;
    });

    expect(result.current.activeViewId).toBe(UNASSIGNED_VIEW_ID);
    expect(result.current.filters.assignee).toBe("unassigned");
  });

  it("keeps a later view selection when deleting the active default finishes", async () => {
    mockDefaultViewSettings();
    const { result } = renderHook(() => useJiraFilterState(PROJECT_KEY));
    await waitFor(() => expect(result.current.activeViewId).toBe(DEFAULT_VIEW_ID));

    const write = deferred<Awaited<ReturnType<typeof updateUserSettings>>>();
    vi.mocked(updateUserSettings).mockReturnValueOnce(write.promise);
    let deletePromise!: Promise<boolean>;
    act(() => {
      deletePromise = result.current.deleteView(DEFAULT_VIEW_ID);
    });
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalled());

    act(() => result.current.selectView(UNASSIGNED_VIEW_ID));
    await act(async () => {
      write.resolve({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);
      await deletePromise;
    });

    expect(result.current.activeViewId).toBe(UNASSIGNED_VIEW_ID);
    expect(result.current.filters.assignee).toBe("unassigned");
  });
});
