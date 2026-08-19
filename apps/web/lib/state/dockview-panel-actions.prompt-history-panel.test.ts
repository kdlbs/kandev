import { describe, expect, it } from "vitest";
import { buildExtraPanelActions } from "./dockview-extra-panel-actions";
import { makeApi, makeStore } from "./dockview-panel-actions.test-utils";
import { CENTER_GROUP, PROMPT_HISTORY_PANEL_ID } from "./layout-manager";

const PROMPT_HISTORY_TITLE = "Prompt history";

const SESSION_ID = "session-1";
const MESSAGE_ID = "message-1";

describe("addPromptHistoryPanel", () => {
  it("adds the registered panel in the invoking group", () => {
    const api = makeApi({ extraGroupIds: ["group-invoking"] });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addPromptHistoryPanel({ groupId: "group-invoking", inCenter: true });

    const panel = api.getPanel(PROMPT_HISTORY_PANEL_ID);
    expect(panel).toMatchObject({
      id: PROMPT_HISTORY_PANEL_ID,
      group: { id: "group-invoking" },
      api: { component: PROMPT_HISTORY_PANEL_ID },
    });
  });

  it("uses the localized registry title", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addPromptHistoryPanel();

    expect(api.getPanel(PROMPT_HISTORY_PANEL_ID)?.title).toBe(PROMPT_HISTORY_TITLE);
  });

  it("falls back to the center group when no groupId is given", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addPromptHistoryPanel();

    expect(api.getPanel(PROMPT_HISTORY_PANEL_ID)?.group.id).toBe(CENTER_GROUP);
  });

  it("places in the center group when inCenter is set without a groupId", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addPromptHistoryPanel({ inCenter: true });

    expect(api.getPanel(PROMPT_HISTORY_PANEL_ID)?.group.id).toBe(CENTER_GROUP);
  });

  it("adds without activating the panel when opened quietly", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addPromptHistoryPanel({ quiet: true });

    // The mock panel exposes isActive; IDockviewPanel's type omits it.
    const panel = api.getPanel(PROMPT_HISTORY_PANEL_ID) as unknown as
      | {
          isActive: boolean;
        }
      | undefined;
    expect(panel?.isActive).toBe(false);
  });
});

describe("scrollTranscriptToMessage", () => {
  it("opens a session chat target and records its exact owner", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    expect(api.getPanel(`session:${SESSION_ID}`)).toMatchObject({ api: { component: "chat" } });
    expect(store.state.scrollTarget).toMatchObject({
      sessionId: SESSION_ID,
      messageId: MESSAGE_ID,
      hostPanelId: `session:${SESSION_ID}`,
    });
  });

  it("focuses an existing session panel instead of adding a second tab", () => {
    const api = makeApi();
    api.addPanel({
      id: `session:${SESSION_ID}`,
      component: "chat",
      title: "Agent",
      position: { referenceGroup: CENTER_GROUP },
    });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    const panelCount = api.panels.length;

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    expect(api.panels).toHaveLength(panelCount);
    expect(
      (api.getPanel(`session:${SESSION_ID}`) as unknown as { isActive: boolean }).isActive,
    ).toBe(true);
    expect(store.state.scrollTarget?.hostPanelId).toBe(`session:${SESSION_ID}`);
  });

  it("focuses the canonical chat panel without adding a session tab when it is the only chat target", () => {
    const api = makeApi();
    api.addPanel({
      id: "chat",
      component: "chat",
      title: "Agent",
      position: { referenceGroup: CENTER_GROUP },
    });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    expect(api.getPanel(`session:${SESSION_ID}`)).toBeUndefined();
    expect((api.getPanel("chat") as unknown as { isActive: boolean }).isActive).toBe(true);
    expect(store.state.scrollTarget?.hostPanelId).toBe("chat");
  });

  it("activates the target panel before recording the target", () => {
    const api = makeApi();
    api.addPanel({
      id: "chat",
      component: "chat",
      title: "Agent",
      position: { referenceGroup: CENTER_GROUP },
    });
    const store = makeStore(api);
    const chatPanel = api.getPanel("chat");
    if (!chatPanel) throw new Error("chat panel did not seed");
    // A reversed implementation records the target BEFORE activating; the
    // activation-time snapshot must still be null (set happens after).
    const originalSetActive = chatPanel.api.setActive;
    chatPanel.api.setActive = () => {
      expect(store.state.scrollTarget).toBeNull();
      originalSetActive();
    };

    const actions = buildExtraPanelActions(store.set, store.get);
    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    // The mock panel exposes isActive; IDockviewPanel's type omits it.
    const typedChatPanel = chatPanel as unknown as { isActive: boolean };
    expect(typedChatPanel.isActive).toBe(true);
    expect(store.state.scrollTarget).not.toBeNull();
  });

  it("monotonically increases the token across requests", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");
    const firstToken = store.state.scrollTarget?.token ?? -1;
    actions.scrollTranscriptToMessage(SESSION_ID, "message-2", "Agent");
    const secondToken = store.state.scrollTarget?.token ?? -1;

    expect(secondToken).toBeGreaterThan(firstToken);
  });

  it("clearScrollTarget clears only on the exact token", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");
    const token = store.state.scrollTarget?.token ?? -1;

    actions.clearScrollTarget(token - 1);
    expect(store.state.scrollTarget).not.toBeNull();

    actions.clearScrollTarget(token);
    expect(store.state.scrollTarget).toBeNull();
  });

  it("clearScrollTargetForOwner clears only the matching host", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    actions.clearScrollTargetForOwner(SESSION_ID, "chat");
    expect(store.state.scrollTarget).not.toBeNull();

    actions.clearScrollTargetForOwner(SESSION_ID, `session:${SESSION_ID}`);
    expect(store.state.scrollTarget).toBeNull();
  });
});
