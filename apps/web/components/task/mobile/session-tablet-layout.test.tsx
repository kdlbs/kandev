import type { ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useLayoutStore } from "@/lib/state/layout-store";
import { SessionTabletLayout } from "./session-tablet-layout";

vi.mock("react-resizable-panels", () => ({
  Group: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  Panel: ({ id, children }: { id: string; children: ReactNode }) => (
    <div data-testid={`tablet-panel-${id}`}>{children}</div>
  ),
}));

vi.mock("./session-task-switcher-sheet", () => ({
  SessionTaskSwitcherSheet: () => null,
}));

vi.mock("../task-center-panel", () => ({
  TaskCenterPanel: () => <div data-testid="tablet-center-panel" />,
}));

vi.mock("../task-right-panel", () => ({
  TaskRightPanel: () => <div data-testid="tablet-right-panel" />,
}));

vi.mock("../task-files-panel", () => ({
  TaskFilesPanel: () => <div data-testid="tablet-files-panel" />,
}));

vi.mock("@/components/task/browser-panel", () => ({
  BrowserPanel: () => <div data-testid="tablet-browser-panel" />,
}));

vi.mock("@/components/task/preview-controller", () => ({
  PreviewController: () => null,
}));

vi.mock("@/lib/layout/use-default-layout", () => ({
  useDefaultLayout: () => ({
    defaultLayout: undefined,
    onLayoutChanged: vi.fn(),
  }),
}));

vi.mock("@/hooks/use-session-layout-state", () => ({
  useSessionLayoutState: () => ({
    activeTaskId: "task-1",
    effectiveSessionId: "session-1",
    sessionKey: "session-1",
    selectedDiff: null,
    handleClearSelectedDiff: vi.fn(),
    openFileRequest: null,
    handleOpenFile: vi.fn(),
    handleFileOpenHandled: vi.fn(),
    isTaskSwitcherOpen: false,
    setMobileSessionTaskSwitcherOpen: vi.fn(),
  }),
}));

vi.mock("../dockview-review-dialog", () => ({
  TaskReviewDialogMount: () => null,
}));

afterEach(cleanup);

describe("SessionTabletLayout right-panel visibility", () => {
  beforeEach(() => {
    useLayoutStore.setState({
      columnsBySessionId: {
        "session-1": {
          left: true,
          chat: true,
          right: false,
          preview: false,
          document: false,
        },
      },
    });
  });

  it("does not render the right column when the session layout hides it", () => {
    render(<SessionTabletLayout workspaceId="workspace-1" workflowId="workflow-1" />);

    expect(screen.queryByTestId("tablet-right-panel")).toBeNull();
    expect(screen.getByTestId("tablet-center-panel")).toBeTruthy();
  });
});
