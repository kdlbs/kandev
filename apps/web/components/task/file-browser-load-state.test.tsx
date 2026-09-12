import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { renderSessionOrLoadState } from "./file-browser-load-state";

afterEach(cleanup);

describe("renderSessionOrLoadState", () => {
  it("uses the compact workspace failure state for failed sessions", () => {
    const result = renderSessionOrLoadState({
      isSessionFailed: true,
      sessionError: "raw environment preparation failure",
      loadState: "idle",
      isLoadingTree: false,
      tree: null,
      loadError: null,
      onRetry: () => {},
    });

    render(<>{result}</>);

    expect(screen.getByTestId("workspace-unavailable")).toBeTruthy();
    expect(screen.getByText("Workspace unavailable")).toBeTruthy();
    expect(screen.getByText("Technical details")).toBeTruthy();
    expect(screen.getByText("raw environment preparation failure")).toBeTruthy();
    expect(screen.queryByText("Session failed")).toBeNull();
  });

  it("surfaces workspace restoration failures instead of waiting forever", () => {
    const onRestoreWorkspace = () => {};
    const result = renderSessionOrLoadState({
      isSessionFailed: false,
      sessionError: null,
      loadState: "waiting",
      isLoadingTree: true,
      tree: null,
      loadError: null,
      onRetry: () => {},
      workspaceRestoration: {
        taskId: "task-1",
        sessionId: "session-1",
        environmentId: "environment-1",
        revision: 1,
        status: "error",
        details: "workspace restore failed",
      },
      onRestoreWorkspace,
    });

    render(<>{result}</>);

    expect(screen.getByText("Couldn't reconnect to this task's workspace.")).toBeTruthy();
    expect(screen.queryByTestId("file-tree-waiting")).toBeNull();
    expect(screen.getByTestId("workspace-retry")).toBeTruthy();
  });
});
