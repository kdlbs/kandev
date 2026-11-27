import { expect, it, vi } from "vitest";
import type { DockviewApi, SerializedDockview } from "dockview-react";
import { useDockviewStore } from "./dockview-store";
import { fromDockviewApi } from "./layout-manager";

vi.mock("./layout-manager", async (original) => ({
  ...(await original<typeof import("./layout-manager")>()),
  fromDockviewApi: vi.fn(),
}));

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
it("keeps a maximized group's logical identity for its restored conversation owner", () => {
  const original = useDockviewStore.getState();
  const api = {
    width: 800,
    height: 600,
    panels: [],
    groups: [],
    fromJSON: vi.fn(),
    toJSON: vi.fn(() => ({})),
    activeGroup: null,
    onDidActivePanelChange: vi.fn(() => ({ dispose: vi.fn() })),
    addPanel: vi.fn(),
    hasMaximizedGroup: vi.fn(() => false),
    layout: vi.fn(),
    getPanel: vi.fn(),
  } as unknown as DockviewApi;
  const frames: FrameRequestCallback[] = [];
  const raf = vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    frames.push(callback);
    return frames.length;
  });
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: "center",
        groups: [
          {
            id: "group-center",
            activePanel: "session:a",
            panels: [{ id: "session:a", component: "chat", title: "A" }],
          },
        ],
      },
    ],
  });
  useDockviewStore.setState({ api, preMaximizeLayout: null, currentLayoutEnvId: null });
  try {
    useDockviewStore.getState().maximizeGroup("group-center");
    const applied = vi.mocked(api.fromJSON).mock.lastCall![0];
    const ids: string[] = [];
    const visit = (node: SerializedDockview["grid"]["root"]) => {
      if (Array.isArray(node.data)) node.data.forEach(visit);
      else ids.push(node.data.id);
    };
    visit(applied.grid.root);
    expect(ids).toContain("group-center");
  } finally {
    raf.mockRestore();
    useDockviewStore.setState(original);
  }
});
