import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

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

vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => ({ openFile: vi.fn() }),
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
