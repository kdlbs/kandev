import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const openFile = vi.hoisted(() => vi.fn());

const taskChangesPanel = vi.hoisted(() => ({
  props: null as Record<string, unknown> | null,
}));

vi.mock("./task-changes-panel", () => ({
  TaskChangesPanel: (props: Record<string, unknown>) => {
    taskChangesPanel.props = props;
    return <div data-testid="task-changes-panel" />;
  },
}));

vi.mock("@/components/settings/canvas-host-route", () => ({
  CanvasHostRoute: ({ canvasId }: { canvasId: string }) => (
    <div data-testid="canvas-host-route-mock">{canvasId}</div>
  ),
}));

vi.mock("./files-panel", () => ({
  FilesPanel: ({
    onOpenFile,
  }: {
    onOpenFile: (file: {
      path: string;
      name: string;
      repo?: string;
      content: string;
      originalContent: string;
      originalHash: string;
      isDirty: boolean;
    }) => void;
  }) => (
    <button
      onClick={() =>
        onOpenFile({
          path: "README.md",
          name: "README.md",
          repo: "secondary",
          content: "# Secondary",
          originalContent: "# Secondary",
          originalHash: "hash",
          isDirty: false,
        })
      }
    >
      Open secondary repository file
    </button>
  ),
}));

vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => ({ openFile }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      selectedDiff: null,
      setSelectedDiff: vi.fn(),
      api: { getPanel: vi.fn(), removePanel: vi.fn() },
    }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      connection: { status: "disconnected" },
      tasks: { activeSessionId: null, activeTaskId: null },
      taskSessions: { items: {} },
      agentProfiles: { items: [] },
      features: { canvases: true },
      kanban: { tasks: [] },
      kanbanMulti: { snapshots: {} },
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      tasks: { activeSessionId: null, activeTaskId: null },
      kanban: { tasks: [] },
      kanbanMulti: { snapshots: {} },
    }),
  }),
}));

import { renderPanel } from "./dockview-panel-content";
import { renderPanel as renderSharedPanel } from "./dockview-shared";

describe("dockview diff panel content", () => {
  it("passes repositoryName from panel params into the file-mode diff view", () => {
    render(
      <>
        {renderPanel("preview:file-diff", "diff-viewer", {
          kind: "file",
          path: "README.md",
          source: "uncommitted",
          repositoryName: "frontend",
          prKey: "acme/frontend/42",
        })}
      </>,
    );

    expect(taskChangesPanel.props).toMatchObject({
      mode: "file",
      filePath: "README.md",
      sourceFilter: "uncommitted",
      fileRepositoryName: "frontend",
      prKey: "acme/frontend/42",
    });
  });

  it("preserves the PR source in the shared dockview renderer", () => {
    render(
      <>
        {renderSharedPanel("preview:file-diff", "diff-viewer", {
          kind: "file",
          path: "README.md",
          source: "pr",
          repositoryName: "widgets · feat/second",
          prKey: "acme/widgets/42",
        })}
      </>,
    );

    expect(taskChangesPanel.props).toMatchObject({
      mode: "file",
      filePath: "README.md",
      sourceFilter: "pr",
      prKey: "acme/widgets/42",
    });
  });
});

describe("dockview canvas panel content", () => {
  it("gives the portaled canvas host a full-height flex boundary", async () => {
    const { renderPanel } = await import("./dockview-panel-content");
    render(<>{renderPanel("canvas:canvas-1", "canvas", { canvasId: "canvas-1" })}</>);

    const host = screen.getByTestId("canvas-host-route-mock");
    expect(host.parentElement?.className).toContain("h-full");
    expect(host.parentElement?.className).toContain("min-h-0");
    expect(host.parentElement?.className).toContain("flex");
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.13
describe("dockview workspace file repository identity", () => {
  afterEach(() => {
    cleanup();
    openFile.mockReset();
  });
  it.each([
    ["portaled", renderPanel],
    ["shared", renderSharedPanel],
  ] as const)("preserves the selected repository in the %s Files panel", (_label, renderer) => {
    render(<>{renderer("files-panel", "files", {})}</>);
    fireEvent.click(screen.getByRole("button", { name: "Open secondary repository file" }));
    expect(openFile).toHaveBeenCalledWith("README.md", "secondary");
  });
});
