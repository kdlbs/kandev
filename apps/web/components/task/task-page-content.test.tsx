import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { TaskLoadErrorState } from "./task-page-content";
import { TaskRemovalBoundary } from "./task-removal-boundary";

afterEach(cleanup);

function renderErrorState(activeId: string | null) {
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId } }}>
      <TaskLoadErrorState />
    </StateProvider>,
  );
}

describe("TaskLoadErrorState", () => {
  it("preserves the active workspace in the overview destination", () => {
    renderErrorState("ws-1");

    const link = screen.getByTestId("task-unavailable-overview-link");
    expect(link.getAttribute("href")).toBe("/?home=overview&workspaceId=ws-1");
    expect(link.className).toContain("min-h-11");
  });

  it("falls back to the unscoped overview when no workspace is active", () => {
    renderErrorState(null);

    expect(screen.getByTestId("task-unavailable-overview-link").getAttribute("href")).toBe(
      "/?home=overview",
    );
  });
});

function StartRemoval() {
  const store = useAppStoreApi();
  useEffect(() => {
    store.getState().beginTaskRemoval({
      action: "delete",
      workspaceId: "ws-1",
      taskIds: ["task-1"],
      requestIds: ["task-1"],
      departure: null,
    });
  }, [store]);
  return null;
}

describe("TaskRemovalBoundary", () => {
  it("unmounts outgoing content while a removal operation is pending", async () => {
    render(
      <StateProvider>
        <StartRemoval />
        <TaskRemovalBoundary taskId="task-1">
          <div data-testid="outgoing-task-content">Outgoing task</div>
        </TaskRemovalBoundary>
      </StateProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("task-removal-status")).toBeTruthy());
    expect(screen.queryByTestId("outgoing-task-content")).toBeNull();
  });
});
