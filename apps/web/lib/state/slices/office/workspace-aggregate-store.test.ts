import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice, WorkspaceAggregate } from "./types";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

function makeAggregate(): WorkspaceAggregate {
  return {
    workspaces: [
      {
        workspace_id: "ws-1",
        name: "Alpha",
        task_count: 5,
        open_tasks: 2,
        in_progress_tasks: 1,
        blocked_tasks: 1,
        done_tasks: 1,
        pending_approvals: 2,
        agent_count: 3,
        running_agents: 1,
      },
    ],
    recentActivity: [],
  };
}

describe("workspace aggregate store action", () => {
  it("defaults to null before any load", () => {
    const store = makeStore();
    expect(store.getState().office.workspaceAggregate).toBeNull();
  });

  it("setWorkspaceAggregate stores the aggregate", () => {
    const store = makeStore();
    const aggregate = makeAggregate();
    store.getState().setWorkspaceAggregate(aggregate);
    expect(store.getState().office.workspaceAggregate?.workspaces).toHaveLength(1);
    expect(store.getState().office.workspaceAggregate?.workspaces[0].name).toBe("Alpha");
  });

  it("setWorkspaceAggregate clears with null", () => {
    const store = makeStore();
    store.getState().setWorkspaceAggregate(makeAggregate());
    store.getState().setWorkspaceAggregate(null);
    expect(store.getState().office.workspaceAggregate).toBeNull();
  });
});
