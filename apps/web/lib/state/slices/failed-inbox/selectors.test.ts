import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import {
  selectFailedInboxCount,
  selectFailedInboxCountIsKnown,
  selectFailedInboxRows,
  selectFailedInboxStatus,
  selectFailedInboxTruncated,
} from "./selectors";

const WORKSPACE_ID = "workspace-1";

function storeWithActiveWorkspace() {
  const store = createAppStore();
  store.setState((state) => ({
    ...state,
    workspaces: { ...state.workspaces, activeId: WORKSPACE_ID },
  }));
  return store;
}

describe("failed-inbox selectors", () => {
  it("reports no count and unknown status before any read (AC .16, .23)", () => {
    const store = storeWithActiveWorkspace();
    const state = store.getState();
    expect(selectFailedInboxCount(state)).toBe(0);
    expect(selectFailedInboxCountIsKnown(state)).toBe(false);
    expect(selectFailedInboxStatus(state)).toBe("idle");
    expect(selectFailedInboxRows(state)).toEqual([]);
  });

  it("reports no active workspace as the empty workspace state", () => {
    const store = createAppStore();
    const state = store.getState();
    expect(selectFailedInboxCount(state)).toBe(0);
    expect(selectFailedInboxCountIsKnown(state)).toBe(false);
  });

  it("reflects an applied page: known count, rows, and truncation", () => {
    const store = storeWithActiveWorkspace();
    const generation = store.getState().beginFailedInboxRead(WORKSPACE_ID);
    store.getState().setFailedInboxPage(WORKSPACE_ID, generation, {
      rows: [
        {
          task_id: "t1",
          title: "Task",
          workspace_id: WORKSPACE_ID,
          origin: "manual",
          failure_instant: "2026-09-14T01:00:00Z",
          reason: "boom",
        },
      ],
      count: 1,
      truncated: true,
    });

    const state = store.getState();
    expect(selectFailedInboxCount(state)).toBe(1);
    expect(selectFailedInboxCountIsKnown(state)).toBe(true);
    expect(selectFailedInboxTruncated(state)).toBe(true);
    expect(selectFailedInboxRows(state)).toHaveLength(1);
  });

  // AC .23: the count is not "known" while the last read for the active
  // workspace failed, even though a generation was applied.
  it("reports the count as unknown after a failed read", () => {
    const store = storeWithActiveWorkspace();
    const generation = store.getState().beginFailedInboxRead(WORKSPACE_ID);
    store.getState().setFailedInboxError(WORKSPACE_ID, generation);

    const state = store.getState();
    expect(selectFailedInboxStatus(state)).toBe("error");
    expect(selectFailedInboxCountIsKnown(state)).toBe(false);
    expect(selectFailedInboxCount(state)).toBe(0);
  });
});
