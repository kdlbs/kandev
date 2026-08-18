import { describe, expect, it } from "vitest";
import type { DockviewApi } from "dockview-react";
import { reviewPanelId } from "./dockview-panel-actions";
import { buildExtraPanelActions } from "./dockview-extra-panel-actions";
import { CENTER_GROUP } from "./layout-manager";
import { makeApi, makeStore } from "./dockview-panel-actions.test-utils";
import type { MockPanel } from "./dockview-panel-actions.test-utils";

const PR_DETAILS_TITLE = "PR Details";

describe("addPRPanel — dedup with legacy auto-shown panel", () => {
  const PR_KEY = "testorg/testrepo/101";
  const OTHER_PR_KEY = "testorg/testrepo/202";
  const LEGACY_PR_ID = "pr-detail";
  const KEYED_PR_ID = `pr-detail|${PR_KEY}`;
  const OTHER_KEYED_PR_ID = `pr-detail|${OTHER_PR_KEY}`;

  function buildExtra(api: DockviewApi) {
    const store = makeStore(api);
    return { api, actions: buildExtraPanelActions(store.set, store.get) };
  }

  /** Canonical PR Details panels retain the key currently shown for the task. */
  function seedLegacyPanel(api: DockviewApi, prKey: string): void {
    api.addPanel({
      id: LEGACY_PR_ID,
      component: "pr-detail",
      title: "Pull Request",
      params: { prKey },
      position: { referenceGroup: CENTER_GROUP },
    });
  }

  it("reuses the legacy panel when the requested PR is what it's already showing", () => {
    const { api, actions } = buildExtra(makeApi());
    seedLegacyPanel(api, PR_KEY);

    actions.addPRPanel(PR_KEY);

    const prPanels = api.panels.filter(
      (p) => p.id === LEGACY_PR_ID || p.id.startsWith(`${LEGACY_PR_ID}|`),
    );
    expect(prPanels).toHaveLength(1);
    expect(api.getPanel(KEYED_PR_ID)).toBeUndefined();
    expect((api.getPanel(LEGACY_PR_ID) as unknown as MockPanel).isActive).toBe(true);
  });

  it("opens a distinct tab for a different PR instead of overwriting the legacy tab", () => {
    const { api, actions } = buildExtra(makeApi());
    seedLegacyPanel(api, PR_KEY);

    // Multi-repo "+" menu click for a PR other than the one the legacy tab
    // is showing must NOT repurpose that tab — it must open its own tab.
    actions.addPRPanel(OTHER_PR_KEY);

    const legacy = api.getPanel(LEGACY_PR_ID) as unknown as MockPanel;
    expect(legacy.params.prKey).toBe(PR_KEY);

    const other = api.getPanel(OTHER_KEYED_PR_ID) as unknown as MockPanel;
    expect(other).toBeDefined();
    expect(other.isActive).toBe(true);

    const prPanels = api.panels.filter(
      (p) => p.id === LEGACY_PR_ID || p.id.startsWith(`${LEGACY_PR_ID}|`),
    );
    expect(prPanels).toHaveLength(2);
  });

  it("creates a new keyed panel when no legacy panel exists", () => {
    const { api, actions } = buildExtra(makeApi());

    actions.addPRPanel(PR_KEY);

    expect(api.getPanel(KEYED_PR_ID)).toBeDefined();
    expect(api.getPanel(LEGACY_PR_ID)).toBeUndefined();
  });

  it("focuses an existing keyed panel exactly once", () => {
    const { api, actions } = buildExtra(makeApi());

    actions.addPRPanel(PR_KEY);
    actions.addPRPanel(PR_KEY);

    const prPanels = api.panels.filter(
      (p) => p.id === LEGACY_PR_ID || p.id.startsWith(`${LEGACY_PR_ID}|`),
    );
    expect(prPanels).toHaveLength(1);
    const keyed = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(keyed.isActive).toBe(true);
  });
});

describe("addPRPanel — layout-owned group placement", () => {
  const PR_KEY = "testorg/testrepo/202";
  const KEYED_PR_ID = `pr-detail|${PR_KEY}`;
  const SESSION_ID = "s-1";
  const SESSION_PANEL_ID = `session:${SESSION_ID}`;
  const SESSION_GROUP = "group-session-host";
  const PR_DETAILS_GROUP = "group-configured-pr-details";
  const REQUESTED_GROUP = "group-invoking-add-menu";

  function buildExtra(api: DockviewApi) {
    const store = makeStore(api);
    return { api, actions: buildExtraPanelActions(store.set, store.get) };
  }

  function seedSessionInGroup(api: DockviewApi, groupId: string): void {
    api.addPanel({
      id: SESSION_PANEL_ID,
      component: "chat",
      title: "Session",
      params: { sessionId: SESSION_ID },
      position: { referenceGroup: groupId },
    });
  }

  function seedCanonicalPRDetails(api: DockviewApi, groupId = PR_DETAILS_GROUP): void {
    api.addPanel({
      id: "pr-detail",
      component: "pr-detail",
      title: "PR Details",
      position: { referenceGroup: groupId },
    });
  }

  it("places a new PR tab in PR Details' configured group, not beside Agent", () => {
    const { api, actions } = buildExtra(makeApi());
    seedSessionInGroup(api, SESSION_GROUP);
    seedCanonicalPRDetails(api);

    actions.addPRPanel(PR_KEY);

    const pr = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(pr).toBeDefined();
    expect(pr.group.id).toBe(PR_DETAILS_GROUP);
    expect(pr.group.id).not.toBe(SESSION_GROUP);
  });

  it("falls back to centerGroupId when no PR Details panel exists", () => {
    const { api, actions } = buildExtra(makeApi());
    actions.addPRPanel(PR_KEY);

    const pr = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(pr).toBeDefined();
    expect(pr.group.id).toBe(CENTER_GROUP);
  });

  it("opens in the explicitly requested group when no PR Details panel exists", () => {
    const { api, actions } = buildExtra(makeApi({ extraGroupIds: [REQUESTED_GROUP] }));

    actions.addPRPanel(PR_KEY, { groupId: REQUESTED_GROUP });

    const pr = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(pr).toBeDefined();
    expect(pr.group.id).toBe(REQUESTED_GROUP);
  });

  it("prefers the explicitly requested group over another canonical group", () => {
    const { api, actions } = buildExtra(makeApi({ extraGroupIds: [REQUESTED_GROUP] }));
    seedCanonicalPRDetails(api);

    actions.addPRPanel(PR_KEY, { groupId: REQUESTED_GROUP });

    const pr = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(pr.group.id).toBe(REQUESTED_GROUP);
  });

  it("focuses an existing keyed PR tab without relocating it", () => {
    const { api, actions } = buildExtra(makeApi());
    seedCanonicalPRDetails(api);
    api.addPanel({
      id: KEYED_PR_ID,
      component: "pr-detail",
      title: "Pull Request",
      params: { prKey: PR_KEY },
      position: { referenceGroup: SESSION_GROUP },
    });

    actions.addPRPanel(PR_KEY);

    const pr = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(pr.group.id).toBe(SESSION_GROUP);
    expect(pr.isActive).toBe(true);
  });
});

describe("addMRPanel — layout-owned review tabs", () => {
  const MR_KEY = "https://gitlab.example.test|platform/kandev|81";
  const OTHER_MR_KEY = "https://gitlab.example.test|platform/kandev|82";
  const LEGACY_MR_ID = "mr-detail";

  function buildExtra(api: DockviewApi) {
    const store = makeStore(api);
    return { api, actions: buildExtraPanelActions(store.set, store.get) };
  }

  function seedLegacyPanel(api: DockviewApi, mrKey: string): void {
    api.addPanel({
      id: LEGACY_MR_ID,
      component: "mr-detail",
      title: "Merge Request",
      params: { mrKey },
      position: { referenceGroup: CENTER_GROUP },
    });
  }

  it("focuses a matching legacy panel in place", () => {
    const { api, actions } = buildExtra(makeApi());
    seedLegacyPanel(api, MR_KEY);

    actions.addMRPanel(MR_KEY);

    expect(api.panels.filter((panel) => panel.id.startsWith(LEGACY_MR_ID))).toHaveLength(1);
    expect(api.getPanel(`${LEGACY_MR_ID}|${MR_KEY}`)).toBeUndefined();
    expect((api.getPanel(LEGACY_MR_ID) as unknown as MockPanel).isActive).toBe(true);
  });

  it("opens a keyed panel in the canonical PR Details group", () => {
    const { api, actions } = buildExtra(makeApi());
    seedLegacyPanel(api, MR_KEY);
    api.addPanel({
      id: "pr-detail",
      component: "pr-detail",
      title: "PR Details",
      position: { referenceGroup: "group-configured-pr-details" },
    });

    actions.addMRPanel(OTHER_MR_KEY);

    const keyed = api.getPanel(`${LEGACY_MR_ID}|${OTHER_MR_KEY}`) as unknown as MockPanel;
    expect(keyed).toBeDefined();
    expect(keyed.group.id).toBe("group-configured-pr-details");
    expect(api.panels.filter((panel) => panel.id.startsWith(LEGACY_MR_ID))).toHaveLength(2);
  });

  it("opens a new MR panel in the explicitly requested group", () => {
    const requestedGroup = "group-invoking-add-menu";
    const { api, actions } = buildExtra(makeApi({ extraGroupIds: [requestedGroup] }));

    actions.addMRPanel(MR_KEY, { groupId: requestedGroup });

    const panel = api.getPanel(`${LEGACY_MR_ID}|${MR_KEY}`) as unknown as MockPanel;
    expect(panel.group.id).toBe(requestedGroup);
  });
});

const PLUGIN_ID = "kandev-plugin-notes";
const PLUGIN_PANEL_KEY = "notes";
const PLUGIN_PANEL_TITLE = "Notes";
const PLUGIN_PANEL_ID = "plugin:kandev-plugin-notes:notes";

describe("addPluginPanel / closePluginPanels", () => {
  function buildExtra(api: DockviewApi) {
    const store = makeStore(api);
    return { api, actions: buildExtraPanelActions(store.set, store.get) };
  }

  it("opens a plugin panel beside chat by default, with the shared plugin-panel component/tab", () => {
    const { api, actions } = buildExtra(makeApi());

    actions.addPluginPanel(PLUGIN_ID, PLUGIN_PANEL_KEY, PLUGIN_PANEL_TITLE);

    const panel = api.getPanel(PLUGIN_PANEL_ID) as unknown as MockPanel;
    expect(panel).toBeDefined();
    expect(panel.title).toBe(PLUGIN_PANEL_TITLE);
    expect(panel.params).toEqual({ pluginId: PLUGIN_ID, panelKey: PLUGIN_PANEL_KEY });
  });

  it("focuses an already-open plugin panel instead of duplicating it", () => {
    const { api, actions } = buildExtra(makeApi());

    actions.addPluginPanel(PLUGIN_ID, PLUGIN_PANEL_KEY, PLUGIN_PANEL_TITLE);
    (api.getPanel(PLUGIN_PANEL_ID) as unknown as MockPanel).isActive = false;
    actions.addPluginPanel(PLUGIN_ID, PLUGIN_PANEL_KEY, PLUGIN_PANEL_TITLE);

    expect(api.panels.filter((p) => p.id === PLUGIN_PANEL_ID)).toHaveLength(1);
    expect((api.getPanel(PLUGIN_PANEL_ID) as unknown as MockPanel).isActive).toBe(true);
  });

  it("respects inCenter and quiet options", () => {
    const { api, actions } = buildExtra(makeApi());

    actions.addPluginPanel(PLUGIN_ID, PLUGIN_PANEL_KEY, PLUGIN_PANEL_TITLE, {
      inCenter: true,
      quiet: true,
    });

    const panel = api.getPanel(PLUGIN_PANEL_ID) as unknown as MockPanel;
    expect(panel.group.id).toBe(CENTER_GROUP);
    expect(panel.isActive).toBe(false);
  });

  it("closePluginPanels removes only the target plugin's panels", () => {
    const { api, actions } = buildExtra(makeApi());
    actions.addPluginPanel(PLUGIN_ID, PLUGIN_PANEL_KEY, PLUGIN_PANEL_TITLE);
    actions.addPluginPanel("kandev-plugin-other", "widget", "Widget");

    actions.closePluginPanels(PLUGIN_ID);

    expect(api.getPanel(PLUGIN_PANEL_ID)).toBeUndefined();
    expect(api.getPanel("plugin:kandev-plugin-other:widget")).toBeDefined();
  });
});

describe("addReviewPanel", () => {
  const providerId = "bitbucket";
  const reviewKey = "project/repository/42";
  const repositoryId = "repository-uuid";
  const connectionScope = "https://bitbucket.example.test";
  const changeRequestNumber = 42;
  const title = "Bitbucket Pull Request";
  const review = {
    providerId,
    reviewKey,
    connectionScope,
    repositoryId,
    changeRequestNumber,
    title,
  };
  const panelId = reviewPanelId(review);

  it("opens a provider-neutral panel with normalized params in the canonical review group", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    api.addPanel({
      id: "pr-detail",
      component: "pr-detail",
      title: PR_DETAILS_TITLE,
      position: { referenceGroup: "provider-review-group" },
    });

    actions.addReviewPanel(review);

    const panel = api.getPanel(panelId) as unknown as MockPanel;
    expect(panel.title).toBe(title);
    expect(panel.params).toEqual({
      providerId,
      reviewKey,
      connectionScope,
      repositoryId,
      changeRequestNumber,
    });
    expect(panel.group.id).toBe("provider-review-group");
  });

  it("focuses the canonical review panel when it already renders the requested review", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    api.addPanel({
      id: "pr-detail",
      component: "pr-detail",
      title: PR_DETAILS_TITLE,
      params: { providerId, reviewKey, connectionScope, repositoryId, changeRequestNumber },
    });

    actions.addReviewPanel(review);

    expect((api.getPanel("pr-detail") as unknown as MockPanel).isActive).toBe(true);
    expect(api.getPanel(panelId)).toBeUndefined();
  });

  it("opens a new provider-neutral review in the explicitly requested group", () => {
    const requestedGroup = "group-invoking-add-menu";
    const api = makeApi({ extraGroupIds: [requestedGroup] });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addReviewPanel(review, { groupId: requestedGroup });

    const panel = api.getPanel(panelId) as unknown as MockPanel;
    expect(panel.group.id).toBe(requestedGroup);
  });

  it("does not collide when provider IDs or review keys contain panel delimiters", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    const first = {
      providerId: "a|b",
      reviewKey: "mutable",
      connectionScope: "https://one.example.test",
      repositoryId: "c",
      changeRequestNumber: 1,
      title: "First",
    };
    const second = {
      providerId: "a",
      reviewKey: "mutable",
      connectionScope: "https://two.example.test",
      repositoryId: "b|c",
      changeRequestNumber: 1,
      title: "Second",
    };
    actions.addReviewPanel(first);
    actions.addReviewPanel(second);

    expect(api.getPanel(reviewPanelId(first))).toBeDefined();
    expect(api.getPanel(reviewPanelId(second))).toBeDefined();
  });
});
