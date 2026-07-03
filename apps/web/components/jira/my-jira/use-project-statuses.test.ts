import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { JiraStatus } from "@/lib/types/jira";

const listJiraProjectStatusesMock = vi.fn<[string], Promise<{ statuses: JiraStatus[] }>>();

vi.mock("@/lib/api/domains/jira-api", () => ({
  listJiraProjectStatuses: (key: string) => listJiraProjectStatusesMock(key),
}));

import { reconcileStatuses, useProjectStatuses } from "./use-project-statuses";

afterEach(() => {
  cleanup();
  listJiraProjectStatusesMock.mockReset();
});

function status(id: string, name: string): JiraStatus {
  return { id, name, statusCategory: "indeterminate" };
}

const IN_DEV = "In Development";

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
});

describe("useProjectStatuses", () => {
  it("reports loaded=true with empty options when no project is selected", async () => {
    const { result } = renderHook(() => useProjectStatuses([]));
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.options).toEqual([]);
    expect(listJiraProjectStatusesMock).not.toHaveBeenCalled();
  });

  it("stays unloaded until the fetch resolves, then exposes the options", async () => {
    let resolve: ((v: { statuses: JiraStatus[] }) => void) | undefined;
    listJiraProjectStatusesMock.mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );

    const { result } = renderHook(() => useProjectStatuses(["CLIP"]));

    // Before the fetch resolves the hook must not claim to be loaded, otherwise
    // callers would reconcile a saved status selection against empty options.
    expect(result.current.loaded).toBe(false);
    expect(result.current.options).toEqual([]);

    resolve?.({ statuses: [status("1", IN_DEV)] });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.options).toEqual([status("1", IN_DEV)]);
  });
});
