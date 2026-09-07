import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const bootstrapMock = vi.hoisted(() => vi.fn());
const searchMock = vi.hoisted(() => ({ value: "" }));
const snapshotsMock = vi.hoisted(() => vi.fn());
const displaySettingsMock = vi.hoisted(() => ({
  activeWorkspaceId: "stored-workspace" as string | null,
  activeWorkflowId: null as string | null,
  workspaces: [] as Array<{ id: string }>,
  workflows: [] as Array<{ id: string; workspaceId: string }>,
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(searchMock.value),
}));
vi.mock("@/src/kanban-route", () => ({ useKanbanRouteBootstrap: bootstrapMock }));
vi.mock("@/components/kanban/kanban-header", () => ({ KanbanHeader: () => null }));
vi.mock("@/components/threads/threads-board", () => ({ ThreadsBoard: () => null }));
vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: snapshotsMock,
}));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => displaySettingsMock,
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ setView: vi.fn() }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ kanbanMulti: { snapshots: {}, isLoading: false } }),
}));

import { scopeSnapshotsToWorkspace, ThreadsPageClient } from "./threads-page-client";

afterEach(() => {
  cleanup();
  bootstrapMock.mockReset();
  snapshotsMock.mockReset();
  searchMock.value = "";
});

beforeEach(() => {
  snapshotsMock.mockImplementation(() => ({ refresh: vi.fn() }));
  displaySettingsMock.workspaces = [];
  displaySettingsMock.workflows = [];
});

describe("ThreadsPageClient — workspace deep links", () => {
  it("bootstraps the workspace the link asked for, not the stored one", () => {
    searchMock.value = "workspace=requested-workspace";

    render(<ThreadsPageClient />);

    expect(bootstrapMock).toHaveBeenCalledWith(
      expect.objectContaining({ workspaceId: "requested-workspace" }),
      false,
    );
  });

  it("uses the stored workspace when the link names an unknown workspace", () => {
    displaySettingsMock.workspaces = [{ id: "ws-a" }];
    searchMock.value = "workspace=UNKNOWN";

    render(<ThreadsPageClient />);

    expect(snapshotsMock).toHaveBeenCalledWith("stored-workspace");
  });

  it("scopes snapshots to the requested workspace before effects clear stale data", () => {
    const snapshots = {
      "wf-old": { workflowId: "wf-old", workflowName: "Old", steps: [], tasks: [] },
      "wf-new": { workflowId: "wf-new", workflowName: "New", steps: [], tasks: [] },
    };

    expect(
      Object.keys(
        scopeSnapshotsToWorkspace(
          snapshots,
          [
            { id: "wf-old", workspaceId: "workspace-old" },
            { id: "wf-new", workspaceId: "workspace-new" },
          ],
          "workspace-new",
        ),
      ),
    ).toEqual(["wf-new"]);
  });

  it("leaves the workspace unset when the link names none", () => {
    render(<ThreadsPageClient />);

    expect(bootstrapMock).toHaveBeenCalledWith(
      expect.objectContaining({ workspaceId: undefined }),
      false,
    );
  });

  it("keeps one bootstrap route identity across re-renders of the same link", () => {
    searchMock.value = "workspace=requested-workspace";
    const { rerender } = render(<ThreadsPageClient />);
    rerender(<ThreadsPageClient />);

    const [first] = bootstrapMock.mock.calls[0];
    const [second] = bootstrapMock.mock.calls[bootstrapMock.mock.calls.length - 1];
    expect(first).toBe(second);
  });
});
