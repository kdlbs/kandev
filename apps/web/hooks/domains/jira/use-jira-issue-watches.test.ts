import { describe, it, expect, vi, beforeEach } from "vitest";
import { waitFor, act } from "@testing-library/react";
import { renderHookWithQueryClient } from "@/test-utils/render-with-query";
import type { JiraIssueWatch } from "@/lib/types/jira";

const listMock = vi.fn<[string | undefined], Promise<JiraIssueWatch[]>>();
const createMock = vi.fn<[unknown], Promise<JiraIssueWatch>>();
const updateMock = vi.fn<[string, string, unknown], Promise<JiraIssueWatch>>();
const deleteMock = vi.fn<[string, string], Promise<{ deleted: boolean }>>();
const triggerMock = vi.fn<[string, string], Promise<{ newIssues: number }>>();

vi.mock("@/lib/api/domains/jira-api", () => ({
  listJiraIssueWatches: (...args: [string | undefined]) => listMock(...args),
  createJiraIssueWatch: (...args: [unknown]) => createMock(...args),
  updateJiraIssueWatch: (...args: [string, string, unknown]) => updateMock(...args),
  deleteJiraIssueWatch: (...args: [string, string]) => deleteMock(...args),
  triggerJiraIssueWatch: (...args: [string, string]) => triggerMock(...args),
}));

import { useJiraIssueWatches } from "./use-jira-issue-watches";

function makeWatch(id: string, overrides: Partial<JiraIssueWatch> = {}): JiraIssueWatch {
  return {
    id,
    workspaceId: "ws-1",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    jql: "project = PROJ",
    agentProfileId: "",
    executorProfileId: "",
    prompt: "",
    enabled: true,
    pollIntervalSeconds: 300,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function resetMocks() {
  listMock.mockReset();
  createMock.mockReset();
  updateMock.mockReset();
  deleteMock.mockReset();
  triggerMock.mockReset();
}

describe("useJiraIssueWatches — list fetching", () => {
  beforeEach(resetMocks);

  it("fetches the all-workspaces list when workspaceId is undefined", async () => {
    const watches = [makeWatch("a"), makeWatch("b")];
    listMock.mockResolvedValue(watches);

    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches());

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items).toEqual(watches);
    expect(listMock).toHaveBeenCalledWith(undefined);
  });

  it("fetches the workspace-scoped list when workspaceId is provided", async () => {
    const watches = [makeWatch("a")];
    listMock.mockResolvedValue(watches);

    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches("ws-1"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.items).toEqual(watches);
    expect(listMock).toHaveBeenCalledWith("ws-1");
  });

  it("does not fetch when workspaceId is null", async () => {
    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches(null));

    // With enabled:false the hook stays in loading=true. Give it a tick.
    await act(async () => {});
    expect(listMock).not.toHaveBeenCalled();
    expect(result.current.items).toEqual([]);
  });
});

describe("useJiraIssueWatches — mutations", () => {
  beforeEach(resetMocks);

  it("create calls the API and invalidates the cache", async () => {
    listMock.mockResolvedValue([]);
    const created = makeWatch("new");
    createMock.mockResolvedValue(created);

    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches());
    await waitFor(() => expect(result.current.loading).toBe(false));

    // Prepare the refetch to return the new watch.
    listMock.mockResolvedValue([created]);

    await act(async () => {
      await result.current.create({
        workspaceId: "ws-1",
        workflowId: "wf-1",
        workflowStepId: "step-1",
        jql: "project = PROJ",
      });
    });

    expect(createMock).toHaveBeenCalledTimes(1);
    // After invalidation the list refetches; wait for items to include the new watch.
    await waitFor(() => expect(result.current.items).toContainEqual(created));
  });

  it("update forwards workspaceId from the watch row", async () => {
    listMock.mockResolvedValue([makeWatch("a")]);
    const updated = makeWatch("a", { jql: "project = NEW" });
    updateMock.mockResolvedValue(updated);
    listMock.mockResolvedValueOnce([makeWatch("a")]).mockResolvedValue([updated]);

    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.update("a", { jql: "project = NEW" }, "ws-1");
    });

    expect(updateMock).toHaveBeenCalledWith("ws-1", "a", { jql: "project = NEW" });
  });

  it("update uses hook-level workspaceId as IDOR fallback", async () => {
    listMock.mockResolvedValue([]);
    const updated = makeWatch("a");
    updateMock.mockResolvedValue(updated);

    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches("ws-bound"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.update("a", { enabled: false });
    });

    // No rowWorkspaceId supplied — must fall back to hook's own workspaceId.
    expect(updateMock).toHaveBeenCalledWith("ws-bound", "a", { enabled: false });
  });

  it("update throws when no workspaceId is available", async () => {
    listMock.mockResolvedValue([]);

    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await expect(
      act(async () => {
        await result.current.update("a", { enabled: false });
      }),
    ).rejects.toThrow("workspaceId required");
  });

  it("remove deletes and invalidates the cache", async () => {
    listMock.mockResolvedValue([makeWatch("a")]);
    deleteMock.mockResolvedValue({ deleted: true });

    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    listMock.mockResolvedValue([]);

    await act(async () => {
      await result.current.remove("a");
    });

    expect(deleteMock).toHaveBeenCalledWith("ws-1", "a");
    await waitFor(() => expect(result.current.items).toEqual([]));
  });

  it("trigger calls the API with the correct ids", async () => {
    listMock.mockResolvedValue([]);
    triggerMock.mockResolvedValue({ newIssues: 3 });

    const { result } = renderHookWithQueryClient(() => useJiraIssueWatches("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    let res: { newIssues: number } | undefined;
    await act(async () => {
      res = await result.current.trigger("watch-id");
    });

    expect(triggerMock).toHaveBeenCalledWith("ws-1", "watch-id");
    expect(res?.newIssues).toBe(3);
  });
});
