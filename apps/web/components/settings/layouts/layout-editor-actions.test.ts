import { afterEach, describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  disposeDockview,
  mountDockview,
} from "@/lib/state/layout-manager/dockview-test-host.test-helpers";
import {
  addReusablePanel,
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
    api: { moveTo: vi.fn() },
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

  it("adds PR Details through the same reusable-panel catalog", () => {
    const { api, group } = fakeApi(["chat"]);

    expect(addReusablePanel(api, "pr-detail")).toBe(true);
    expect(api.addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "pr-detail",
        component: "pr-detail",
        title: "PR Details",
        position: { referenceGroup: group },
      }),
    );
  });

  it("protects Agent while removing another reusable panel", () => {
    const { api, panels } = fakeApi();

    expect(removeReusablePanel(api, "chat")).toBe(false);
    expect(removeReusablePanel(api, "files")).toBe(true);
    expect(api.removePanel).toHaveBeenCalledWith(panels[1]);
  });

  it("reorders tabs within their group", () => {
    const { api, panels, group } = fakeApi();

    expect(reorderTab(api, "files", 1)).toBe(true);
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

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.3 */
describe("addReusablePanel with plugin panels in a real Dockview", () => {
  const PLUGIN_ID = "kandev-plugin-prompt-history";
  const PANEL_ID = `plugin:${PLUGIN_ID}:prompt-history`;
  let api: DockviewApi | null = null;

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
    if (api) disposeDockview(api);
    api = null;
  });

  function register(): void {
    pluginRegistry
      .forPlugin(PLUGIN_ID)
      .registerTaskPanel({ id: "prompt-history", title: "Prompt History", Component: () => null });
  }

  /** Agent alone in the first group, Files alone in a second group to its right. */
  function twoGroupEditor(): DockviewApi {
    const dockview = mountDockview(1000, 600);
    dockview.addPanel({ id: "chat", component: "chat", title: "Agent" });
    dockview.addPanel({
      id: "files",
      component: "files",
      title: "Files",
      position: { direction: "right", referencePanel: "chat" },
    });
    api = dockview;
    return dockview;
  }

  it("adds the registered panel once, in the selected split, with its generic identity", () => {
    const dockview = twoGroupEditor();
    register();
    const filesGroup = dockview.getPanel("files")!.group;

    expect(addReusablePanel(dockview, PANEL_ID, filesGroup.id)).toBe(true);
    expect(addReusablePanel(dockview, PANEL_ID, filesGroup.id)).toBe(false);

    const added = dockview.panels.filter((item) => item.id === PANEL_ID);
    expect(added).toHaveLength(1);
    expect(added[0].group.id).toBe(filesGroup.id);
    expect(filesGroup.panels.map((item) => item.id)).toEqual(["files", PANEL_ID]);
    expect(added[0].params).toEqual({ pluginId: PLUGIN_ID, panelKey: "prompt-history" });
    expect(added[0].view.contentComponent).toBe("plugin-panel");
    expect(added[0].view.tabComponent).toBe("pluginPanelTab");
  });

  it("falls back to the active group when no split is selected", () => {
    const dockview = twoGroupEditor();
    register();
    const activeGroup = dockview.activeGroup!;

    expect(addReusablePanel(dockview, PANEL_ID)).toBe(true);

    expect(dockview.getPanel(PANEL_ID)!.group.id).toBe(activeGroup.id);
  });

  it("adds nothing when the registration is lost before the option is selected", () => {
    const dockview = twoGroupEditor();
    register();
    pluginRegistry.unregisterPlugin(PLUGIN_ID);

    expect(addReusablePanel(dockview, PANEL_ID)).toBe(false);

    expect(dockview.panels.map((item) => item.id)).toEqual(["chat", "files"]);
  });

  it("adds nothing for an identity that is neither built-in nor registered", () => {
    const dockview = twoGroupEditor();

    expect(addReusablePanel(dockview, "not-a-panel")).toBe(false);

    expect(dockview.panels.map((item) => item.id)).toEqual(["chat", "files"]);
  });
});
