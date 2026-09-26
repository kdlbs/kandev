import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { createElement, useEffect } from "react";
import type { JiraStatus } from "@/lib/types/jira";

const listJiraProjectStatusesMock =
  vi.fn<(key: string, options?: { workspaceId?: string }) => Promise<{ statuses: JiraStatus[] }>>();

vi.mock("@/lib/api/domains/jira-api", () => ({
  listJiraProjectStatuses: (key: string, options?: { workspaceId?: string }) =>
    listJiraProjectStatusesMock(key, options),
}));

import {
  reconcileStatuses,
  reconcileStatusesForQuery,
  useProjectStatuses,
} from "./use-project-statuses";

afterEach(() => {
  cleanup();
  listJiraProjectStatusesMock.mockReset();
});

function status(id: string, name: string): JiraStatus {
  return { id, name, statusCategory: "indeterminate" };
}

const IN_DEV = "In Development";
const WORKSPACE_ID = "workspace-1";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

describe("reconcileStatuses", () => {
  it("returns the same reference when nothing is selected", () => {
    const selected: string[] = [];
    expect(reconcileStatuses(selected, [status("1", "Open")])).toBe(selected);
  });

  it("keeps selected statuses that are still available", () => {
    const selected = [IN_DEV, "Done"];
    const available = [status("1", IN_DEV), status("2", "Done"), status("3", "To Do")];
    expect(reconcileStatuses(selected, available)).toEqual([IN_DEV, "Done"]);
  });

  it("drops selected statuses no longer present in the union", () => {
    const selected = [IN_DEV, "Ready for review"];
    const available = [status("1", IN_DEV)];
    expect(reconcileStatuses(selected, available)).toEqual([IN_DEV]);
  });

  it("drops all when none remain (e.g. project deselected)", () => {
    expect(reconcileStatuses([IN_DEV], [])).toEqual([]);
  });

  it("returns the same reference when every selection is still valid", () => {
    const selected = ["Open"];
    const result = reconcileStatuses(selected, [status("1", "Open")]);
    expect(result).toBe(selected);
  });

  it("preserves structured statuses when saved custom JQL owns the query", () => {
    const selected = ["Ready"];
    const customJql = "project = CLIP AND status = Ready ORDER BY priority DESC";

    expect(reconcileStatusesForQuery(true, customJql, selected, [])).toBe(selected);
  });

  it("does not reconcile while the current status lookup is pending", () => {
    const selected = ["Ready"];

    expect(reconcileStatusesForQuery(false, null, selected, [])).toBe(selected);
  });

  it("does not reconcile against an empty list from a failed status lookup", () => {
    const selected = ["Ready"];

    expect(reconcileStatusesForQuery(true, null, selected, [], false)).toBe(selected);
  });
});

describe("useProjectStatuses", () => {
  it("reports loaded=true with empty options when no project is selected", async () => {
    const { result } = renderHook(() => useProjectStatuses([]));
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.options).toEqual([]);
    expect(result.current.authoritative).toBe(true);
    expect(listJiraProjectStatusesMock).not.toHaveBeenCalled();
  });

  it("stays unloaded until the fetch resolves, then exposes the options", async () => {
    let resolve: ((v: { statuses: JiraStatus[] }) => void) | undefined;
    listJiraProjectStatusesMock.mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );

    const { result } = renderHook(() => useProjectStatuses(["CLIP"], WORKSPACE_ID));

    // Before the fetch resolves the hook must not claim to be loaded, otherwise
    // callers would reconcile a saved status selection against empty options.
    expect(result.current.loaded).toBe(false);
    expect(result.current.options).toEqual([]);

    resolve?.({ statuses: [status("1", IN_DEV)] });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.options).toEqual([status("1", IN_DEV)]);
    expect(listJiraProjectStatusesMock).toHaveBeenCalledWith("CLIP", {
      workspaceId: WORKSPACE_ID,
    });
  });

  it("marks failed project lookups as non-authoritative", async () => {
    listJiraProjectStatusesMock.mockRejectedValueOnce(new Error("status lookup unavailable"));

    const { result } = renderHook(() => useProjectStatuses(["CLIP"], WORKSPACE_ID));

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.authoritative).toBe(false);
    expect(result.current.options).toEqual([]);
  });

  it("does not report stale options as loaded after project keys change", async () => {
    listJiraProjectStatusesMock.mockResolvedValueOnce({ statuses: [status("1", "Old status")] });
    const next = deferred<{ statuses: JiraStatus[] }>();
    listJiraProjectStatusesMock.mockReturnValueOnce(next.promise);
    const { result, rerender } = renderHook(
      ({ keys }: { keys: string[] }) => useProjectStatuses(keys, WORKSPACE_ID),
      { initialProps: { keys: ["OLD"] } },
    );
    await waitFor(() => expect(result.current.loaded).toBe(true));

    rerender({ keys: ["NEW"] });

    expect(result.current.loaded).toBe(false);
    expect(result.current.options).toEqual([status("1", "Old status")]);
    next.resolve({ statuses: [status("2", "New status")] });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.options).toEqual([status("2", "New status")]);
  });

  it("does not expose the prior loaded state to effects on the first render for new keys", async () => {
    listJiraProjectStatusesMock.mockResolvedValueOnce({ statuses: [status("1", "Old status")] });
    const next = deferred<{ statuses: JiraStatus[] }>();
    listJiraProjectStatusesMock.mockReturnValueOnce(next.promise);
    const loadedSnapshots: Array<{ key: string; loaded: boolean }> = [];

    function StatusConsumer({ projectKey }: { projectKey: string }) {
      const { loaded } = useProjectStatuses([projectKey], WORKSPACE_ID);
      useEffect(() => {
        loadedSnapshots.push({ key: projectKey, loaded });
      }, [loaded, projectKey]);
      return null;
    }

    const { rerender } = render(createElement(StatusConsumer, { projectKey: "OLD" }));
    await waitFor(() => expect(loadedSnapshots.at(-1)).toEqual({ key: "OLD", loaded: true }));
    loadedSnapshots.length = 0;

    rerender(createElement(StatusConsumer, { projectKey: "NEW" }));

    expect(loadedSnapshots).toEqual([{ key: "NEW", loaded: false }]);
    next.resolve({ statuses: [status("2", "New status")] });
    await waitFor(() => expect(loadedSnapshots.at(-1)).toEqual({ key: "NEW", loaded: true }));
  });
});
