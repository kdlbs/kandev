import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import {
  addReusablePanel,
  activatePanel,
  mergeGroup,
  moveGroup,
  movePanelToGroup,
  removeReusablePanel,
  reorderTab,
  resizeGroup,
  splitPanel,
} from "./layout-editor-actions";

type FakeGroup = {
  id: string;
  panels: FakePanel[];
  api: {
    width: number;
    height: number;
    moveTo: ReturnType<typeof vi.fn>;
    setSize: ReturnType<typeof vi.fn>;
  };
};

type FakePanel = {
  id: string;
  group: FakeGroup;
  api: {
    setActive: ReturnType<typeof vi.fn>;
    moveTo: ReturnType<typeof vi.fn>;
  };
};

function fakeApi(panelIds = ["chat", "files", "changes"]) {
  const group = {
    id: "group-one",
    panels: [] as FakePanel[],
    api: { width: 320, height: 240, moveTo: vi.fn(), setSize: vi.fn() },
  };
  group.panels = panelIds.map((id) => ({
    id,
    group,
    api: { setActive: vi.fn(), moveTo: vi.fn() },
  }));
  const second = {
    id: "group-two",
    panels: [] as FakePanel[],
    api: { width: 280, height: 200, moveTo: vi.fn(), setSize: vi.fn() },
  };
  const api = {
    groups: [group, second],
    panels: group.panels,
    activeGroup: group,
    getPanel: vi.fn((id: string) => group.panels.find((panel) => panel.id === id)),
    addPanel: vi.fn(),
    removePanel: vi.fn(),
  } as unknown as DockviewApi;
  return { api, group, second, panels: group.panels };
}

describe("layout editor actions", () => {
  it("adds only missing reusable panels to the active group", () => {
    const { api, group } = fakeApi(["chat"]);

    expect(addReusablePanel(api, "files")).toBe(true);
    expect(api.addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "files",
        component: "files",
        position: { referenceGroup: group },
      }),
    );
    expect(addReusablePanel(api, "chat")).toBe(false);
  });

  it("protects Agent while removing another reusable panel", () => {
    const { api, panels } = fakeApi();

    expect(removeReusablePanel(api, "chat")).toBe(false);
    expect(removeReusablePanel(api, "files")).toBe(true);
    expect(api.removePanel).toHaveBeenCalledWith(panels[1]);
  });

  it("activates and reorders tabs within their group", () => {
    const { api, panels, group } = fakeApi();

    expect(activatePanel(api, "files")).toBe(true);
    expect(reorderTab(api, "files", 1)).toBe(true);
    expect(panels[1].api.setActive).toHaveBeenCalledOnce();
    expect(panels[1].api.moveTo).toHaveBeenCalledWith({
      group,
      position: "center",
      index: 2,
      skipSetActive: true,
    });
  });

  it("moves a panel to another group and creates a split", () => {
    const { api, panels, second, group } = fakeApi();

    expect(movePanelToGroup(api, "files", second.id)).toBe(true);
    expect(splitPanel(api, "changes", "right")).toBe(true);
    expect(panels[1].api.moveTo).toHaveBeenCalledWith({ group: second, position: "center" });
    expect(panels[2].api.moveTo).toHaveBeenCalledWith({ group, position: "right" });
  });

  it("does not claim to create a split from a single-tab group", () => {
    const { api } = fakeApi(["chat"]);

    expect(splitPanel(api, "chat", "right")).toBe(false);
  });

  it("moves and merges complete groups relative to another group", () => {
    const { api, group, second } = fakeApi();

    expect(moveGroup(api, group.id, second.id, "below")).toBe(true);
    expect(mergeGroup(api, group.id, second.id)).toBe(true);
    expect(group.api.moveTo).toHaveBeenNthCalledWith(1, {
      group: second,
      position: "bottom",
    });
    expect(group.api.moveTo).toHaveBeenNthCalledWith(2, {
      group: second,
      position: "center",
    });
  });

  it("resizes a group in bounded keyboard-friendly steps", () => {
    const { api, group } = fakeApi();

    expect(resizeGroup(api, group.id, "width", -500)).toBe(true);
    expect(resizeGroup(api, group.id, "height", 40)).toBe(true);
    expect(group.api.setSize).toHaveBeenNthCalledWith(1, { width: 120 });
    expect(group.api.setSize).toHaveBeenNthCalledWith(2, { height: 280 });
  });
});
