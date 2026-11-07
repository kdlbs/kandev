import { describe, expect, it } from "vitest";
import { buildExtraPanelActions } from "./dockview-extra-panel-actions";
import { makeApi, makeStore } from "./dockview-panel-actions.test-utils";
import { CENTER_GROUP, KNOWN_PANEL_IDS, PANEL_REGISTRY } from "./layout-manager";
import { RENDERABLE_COMPONENTS } from "./layout-manager/renderable-components";

const GRAPH = "task-graph";

describe("addTaskGraphPanel", () => {
  it("opens the task graph as a tab in the center group", () => {
    const api = makeApi();
    const store = makeStore(api);
    buildExtraPanelActions(store.set, store.get).addTaskGraphPanel();

    expect(api.getPanel(GRAPH)).toMatchObject({
      id: GRAPH,
      title: "Task graph",
      group: { id: CENTER_GROUP },
      api: { component: GRAPH },
    });
  });

  it("reuses an existing tab", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    actions.addTaskGraphPanel();
    const count = api.panels.length;
    actions.addTaskGraphPanel();
    expect(api.panels.length).toBe(count);
  });

  it("is a known, renderable panel so saved layouts keep it", () => {
    expect(KNOWN_PANEL_IDS.has(GRAPH)).toBe(true);
    expect(RENDERABLE_COMPONENTS.has(GRAPH)).toBe(true);
    expect(PANEL_REGISTRY[GRAPH]).toMatchObject({ titleKey: "task:panelTaskGraph" });
  });
});
