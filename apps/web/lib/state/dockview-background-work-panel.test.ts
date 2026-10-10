import { describe, expect, it } from "vitest";
import { buildExtraPanelActions } from "./dockview-extra-panel-actions";
import { makeApi, makeStore } from "./dockview-panel-actions.test-utils";
import { CENTER_GROUP } from "./layout-manager";

const BG_WORK_PANEL_ID = "background-work";
const BG_WORK_TITLE = "Build Watcher";

describe("addBackgroundWorkPanel", () => {
  it("adds the background-work panel in the center group by default", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addBackgroundWorkPanel();

    const panel = api.getPanel(BG_WORK_PANEL_ID);
    expect(panel).toMatchObject({
      id: BG_WORK_PANEL_ID,
      group: { id: CENTER_GROUP },
      api: { component: BG_WORK_PANEL_ID },
    });
  });

  it("adds a specific workload detail panel with custom title and workId parameter", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addBackgroundWorkPanel({
      sessionId: "session-1",
      workId: "work-123",
      title: BG_WORK_TITLE,
    });

    const panel = api.getPanel("background-work:session-1:work-123");
    expect(panel).toMatchObject({
      id: "background-work:session-1:work-123",
      title: BG_WORK_TITLE,
      group: { id: CENTER_GROUP },
      api: { component: BG_WORK_PANEL_ID },
      params: { sessionId: "session-1", workId: "work-123" },
    });
  });

  it("focuses an existing workload panel instead of adding a duplicate", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addBackgroundWorkPanel({ workId: "work-123", title: BG_WORK_TITLE });
    const countAfterFirst = api.panels.length;
    actions.addBackgroundWorkPanel({ workId: "work-123", title: BG_WORK_TITLE });

    expect(api.panels.length).toBe(countAfterFirst);
  });

  it("adds without activating the panel when opened quietly", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addBackgroundWorkPanel({ quiet: true });

    const panel = api.getPanel(BG_WORK_PANEL_ID) as unknown as { isActive: boolean } | undefined;
    expect(panel?.isActive).toBe(false);
  });
});
