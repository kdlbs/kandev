import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const OFFICE_WS = "office-1";
const OTHER_OFFICE_WS = "office-2";
const KANBAN_WS = "kanban-1";

type Deferred<T> = { promise: Promise<T>; resolve: (value: T) => void };
function deferred<T>(): Deferred<T> {
  let resolve: (value: T) => void = () => {};
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

const workspaces = [
  { id: OFFICE_WS, office_workflow_id: "wf-office" },
  { id: OTHER_OFFICE_WS, office_workflow_id: "wf-office-2" },
  { id: KANBAN_WS, office_workflow_id: null },
];

type StoredProject = { id: string; name: string };

let projectsByWorkspaceId: Record<string, StoredProject[]> = {};

const setOfficeAgentProfiles = vi.fn();
// Writes through, so a store-miss test can observe the crumb filling in after
// the load lands rather than only that a request went out.
const setProjects = vi.fn((workspaceId: string, projects: StoredProject[]) => {
  projectsByWorkspaceId[workspaceId] = projects;
});
const setInboxItems = vi.fn();
const setInboxCount = vi.fn();
const setMeta = vi.fn();

let activeId: string | null = OFFICE_WS;
let officeEnabled = true;

const storeState = {
  workspaces: {
    get items() {
      return workspaces;
    },
    get activeId() {
      return activeId;
    },
  },
  office: {
    get projectsByWorkspaceId() {
      return projectsByWorkspaceId;
    },
  },
  setOfficeAgentProfiles,
  setProjects,
  setInboxItems,
  setInboxCount,
  setMeta,
};
const storeApi = { getState: () => storeState };

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof storeState) => unknown) => selector(storeState),
  useAppStoreApi: () => storeApi,
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => officeEnabled,
}));

vi.mock("@/hooks/use-office-refetch", () => ({
  useOfficeRefetch: vi.fn(),
}));

const listAgentProfiles = vi.hoisted(() => vi.fn());
const listProjects = vi.hoisted(() => vi.fn());
const getInbox = vi.hoisted(() => vi.fn());
const getMeta = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-api", () => ({
  listAgentProfiles,
  listProjects,
  getInbox,
  getMeta,
}));

import { useOfficeProject, useOfficeWorkspaceData } from "./use-office-workspace-data";

beforeEach(() => {
  activeId = OFFICE_WS;
  officeEnabled = true;
  projectsByWorkspaceId = {};
  listAgentProfiles.mockResolvedValue({ agents: [] });
  listProjects.mockResolvedValue({ projects: [] });
  getInbox.mockResolvedValue({ items: [], total_count: 0 });
  getMeta.mockResolvedValue({ version: "test" });
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useOfficeWorkspaceData", () => {
  it("loads the active office workspace's collections", async () => {
    await act(async () => {
      renderHook(() => useOfficeWorkspaceData());
    });

    expect(listAgentProfiles).toHaveBeenCalledWith(OFFICE_WS, { cache: "no-store" });
    expect(listProjects).toHaveBeenCalledWith(OFFICE_WS, { cache: "no-store" });
    expect(getInbox).toHaveBeenCalledWith(OFFICE_WS, { cache: "no-store" });
    expect(setOfficeAgentProfiles).toHaveBeenCalledWith(OFFICE_WS, []);
    expect(setProjects).toHaveBeenCalledWith(OFFICE_WS, []);
    expect(setInboxItems).toHaveBeenCalledWith(OFFICE_WS, []);
    expect(setInboxCount).toHaveBeenCalledWith(OFFICE_WS, 0);
    expect(setMeta).toHaveBeenCalledWith({ version: "test" });
  });

  it("does not touch office endpoints for a kanban workspace", async () => {
    activeId = KANBAN_WS;

    await act(async () => {
      renderHook(() => useOfficeWorkspaceData());
    });

    expect(listAgentProfiles).not.toHaveBeenCalled();
    expect(listProjects).not.toHaveBeenCalled();
    expect(getInbox).not.toHaveBeenCalled();
  });

  it("does not touch office endpoints when the office feature is off", async () => {
    // The backend does not register /api/v1/office/* in that build, so calling
    // them would be a guaranteed 404 per mounted consumer.
    officeEnabled = false;

    await act(async () => {
      renderHook(() => useOfficeWorkspaceData());
    });

    expect(listAgentProfiles).not.toHaveBeenCalled();
  });

  it("does nothing while no workspace is active", async () => {
    activeId = null;

    await act(async () => {
      renderHook(() => useOfficeWorkspaceData());
    });

    expect(listAgentProfiles).not.toHaveBeenCalled();
  });

  it("lets the newer response win when one workspace is loaded twice", async () => {
    // Leave and return, so the same workspace has two loads in flight. Without
    // a per-workspace sequence guard the older response would land last and the
    // sidebar would show pre-change data indefinitely.
    const firstOffice = deferred<{ agents: unknown[] }>();
    const otherWorkspace = deferred<{ agents: unknown[] }>();
    const secondOffice = deferred<{ agents: unknown[] }>();
    listAgentProfiles
      .mockReturnValueOnce(firstOffice.promise)
      .mockReturnValueOnce(otherWorkspace.promise)
      .mockReturnValueOnce(secondOffice.promise);

    const { rerender } = renderHook(() => useOfficeWorkspaceData());
    activeId = OTHER_OFFICE_WS;
    rerender();
    activeId = OFFICE_WS;
    rerender();

    await act(async () => {
      secondOffice.resolve({ agents: [{ id: "newer" }] });
      firstOffice.resolve({ agents: [{ id: "older" }] });
      otherWorkspace.resolve({ agents: [] });
      await Promise.all([secondOffice.promise, firstOffice.promise, otherWorkspace.promise]);
    });

    const officeWrites = setOfficeAgentProfiles.mock.calls.filter(([id]) => id === OFFICE_WS);
    expect(officeWrites).toHaveLength(1);
    expect(officeWrites[0][1]).toEqual([{ id: "newer" }]);
  });

  it("keeps the last known-good data when a refresh fails", async () => {
    // This hook refreshes collections other surfaces already hydrated. A
    // transient failure writing a fallback would blank valid sidebar data and
    // reset the inbox badge until the next successful refresh.
    const refreshFailure = new Error("network down");
    listAgentProfiles.mockRejectedValue(refreshFailure);
    listProjects.mockRejectedValue(refreshFailure);
    getInbox.mockRejectedValue(refreshFailure);
    getMeta.mockRejectedValue(refreshFailure);

    await act(async () => {
      renderHook(() => useOfficeWorkspaceData());
    });

    expect(setOfficeAgentProfiles).not.toHaveBeenCalled();
    expect(setProjects).not.toHaveBeenCalled();
    expect(setInboxItems).not.toHaveBeenCalled();
    expect(setInboxCount).not.toHaveBeenCalled();
    expect(setMeta).not.toHaveBeenCalled();
  });

  it("files a late response under the workspace it was requested for", async () => {
    // A response arriving after the user moved on is not dropped just because
    // another workspace started loading — it lands under its own key, where it
    // is correct and simply unread. Keying is what makes that safe; with a flat
    // field this was the cross-workspace bleed.
    const pending = deferred<{ projects: unknown[] }>();
    listProjects.mockReturnValueOnce(pending.promise);

    const { rerender } = renderHook(() => useOfficeWorkspaceData());
    activeId = OTHER_OFFICE_WS;
    rerender();

    await act(async () => {
      pending.resolve({ projects: [{ id: "p-1" }] });
      await pending.promise;
    });

    expect(setProjects).toHaveBeenCalledWith(OFFICE_WS, [{ id: "p-1" }]);
  });
});

describe("useOfficeProject", () => {
  const PROJECT = { id: "p-1", name: "Checkout" };

  it("shares an in-flight project load with the workspace chrome", async () => {
    const pending = deferred<{ projects: StoredProject[] }>();
    listProjects.mockReturnValueOnce(pending.promise);

    renderHook(() => {
      useOfficeWorkspaceData();
      return useOfficeProject(PROJECT.id);
    });

    expect(listProjects).toHaveBeenCalledTimes(1);

    await act(async () => {
      pending.resolve({ projects: [PROJECT] });
      await pending.promise;
    });
    expect(setProjects).toHaveBeenCalledWith(OFFICE_WS, [PROJECT]);
  });

  it("reads a warm project straight from the store without a request", async () => {
    projectsByWorkspaceId = { [OFFICE_WS]: [PROJECT] };

    const { result } = renderHook(() => useOfficeProject(PROJECT.id));
    await act(async () => {});

    expect(result.current).toEqual(PROJECT);
    expect(listProjects).not.toHaveBeenCalled();
  });

  it("fills a store miss through the shared workspace load", async () => {
    // A cold /t/:id hydrates the task but not the office collections. The crumb
    // must not fetch into component state: it goes through the same action the
    // office chrome uses, so both callers share one record and one write path.
    listProjects.mockResolvedValue({ projects: [PROJECT] });

    const { result, rerender } = renderHook(() => useOfficeProject(PROJECT.id));
    expect(result.current).toBeUndefined();

    await act(async () => {});
    expect(listProjects).toHaveBeenCalledWith(OFFICE_WS, { cache: "no-store" });
    expect(setProjects).toHaveBeenCalledWith(OFFICE_WS, [PROJECT]);

    rerender();
    expect(result.current).toEqual(PROJECT);
  });

  it("asks once per project even when the load never fills the store", async () => {
    // A missing crumb is the harmless failure mode. Re-requesting on every
    // render because the store is still empty is not.
    listProjects.mockResolvedValue({ projects: [] });

    const { rerender } = renderHook(() => useOfficeProject(PROJECT.id));
    await act(async () => {});
    rerender();
    await act(async () => {});
    rerender();

    expect(listProjects).toHaveBeenCalledTimes(1);
  });

  it("requests nothing for a task with no project", async () => {
    renderHook(() => useOfficeProject(null));
    await act(async () => {});

    expect(listProjects).not.toHaveBeenCalled();
  });

  it("requests nothing outside an office workspace", async () => {
    activeId = KANBAN_WS;

    renderHook(() => useOfficeProject(PROJECT.id));
    await act(async () => {});

    expect(listProjects).not.toHaveBeenCalled();
  });
});
