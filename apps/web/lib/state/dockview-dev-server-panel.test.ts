import { describe, it, expect } from "vitest";
import { makeApi, makeStore } from "./dockview-panel-actions.test-utils";
import type { MockPanel } from "./dockview-panel-actions.test-utils";
import { buildPanelActions } from "./dockview-panel-actions";
import { buildExtraPanelActions } from "./dockview-extra-panel-actions";
import { DEV_SERVER_PANEL_ID } from "./layout-manager/constants";

const RIGHT_BOTTOM_GROUP = "group-right-bottom";

describe("focusOrAddBrowserPanel", () => {
  it("opens one preview browser and focuses it again on later starts", () => {
    // addBrowserPanel mints browser:<timestamp> when it has no URL, so an
    // automatic open left a new empty tab behind on every start/stop cycle.
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildPanelActions(store.set, store.get);

    actions.focusOrAddBrowserPanel();
    actions.focusOrAddBrowserPanel();

    const browsers = api.panels.filter(
      (panel) => panel.api.component === "browser",
    ) as unknown as MockPanel[];
    expect(browsers).toHaveLength(1);
    expect(browsers[0].isActive).toBe(true);
  });
});

describe("addDevServerPanel", () => {
  it("opens the dev-server output panel in the terminal group", () => {
    const api = makeApi({ extraGroupIds: [RIGHT_BOTTOM_GROUP] });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addDevServerPanel();

    const panel = api.getPanel(DEV_SERVER_PANEL_ID) as unknown as MockPanel;
    expect(panel).toBeDefined();
    expect(panel.api.component).toBe("terminal");
    expect(panel.group.id).toBe(RIGHT_BOTTOM_GROUP);
    expect(panel.title).toBe("Dev Server");
  });

  it("carries no terminalId so closing the tab never parks or destroys a user shell", () => {
    const api = makeApi({ extraGroupIds: [RIGHT_BOTTOM_GROUP] });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addDevServerPanel();

    const panel = api.getPanel(DEV_SERVER_PANEL_ID) as unknown as MockPanel;
    expect(panel.params.terminalId).toBeUndefined();
    expect(panel.params.type).toBe(DEV_SERVER_PANEL_ID);
  });

  it("reuses the existing panel instead of stacking duplicates", () => {
    const api = makeApi({ extraGroupIds: [RIGHT_BOTTOM_GROUP] });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addDevServerPanel();
    actions.addDevServerPanel();

    expect(api.panels.filter((panel) => panel.id === DEV_SERVER_PANEL_ID)).toHaveLength(1);
  });

  it("honours an explicit target group", () => {
    const api = makeApi({ extraGroupIds: ["group-other"] });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addDevServerPanel("group-other");

    const panel = api.getPanel(DEV_SERVER_PANEL_ID) as unknown as MockPanel;
    expect(panel.group.id).toBe("group-other");
  });
});
