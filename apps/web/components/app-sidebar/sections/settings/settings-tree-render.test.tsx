import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SettingsMenuMode } from "@/lib/settings/settings-menu-mode";

const MAIN_WORKSPACE_ID = "ws-1";
const MAIN_WORKSPACE_NAME = "Main Workspace";
// The active workspace carries a badge, so its row's accessible name is the
// name plus that badge — match on the name rather than the whole string.
const WORKSPACE_ROW = new RegExp(MAIN_WORKSPACE_NAME);
const SEARCH_LABEL = "Search settings";
// A section that lives inside Task Behavior rather than owning a nav row —
// the same shape Voice Mode had before it moved to a plugin. Used as the probe
// for "sections never render as page rows".
const NESTED_SECTION_LABEL = "Message Queue";
const WORKSPACES_ROW_KEY = "row:/settings/workspaces";
const AGENTS_ROW_KEY = "row:/settings/agents";
const WORKSPACE_KEY = `workspace:${MAIN_WORKSPACE_ID}`;
const AGENT_KEY = "agent:claude-code";
const AGENT_LABEL = "Claude Code";
// Anchored: the caret beside the row is labelled "Expand/Collapse Claude Code",
// and a badge can extend the row's own name past an exact match.
const AGENT_ROW = /^Claude Code/;
// The hide-disabled setting's storage key, used to flip the real hook in the
// tree tests. Hoisted to a constant so the sonar duplicate-literal rule stays
// quiet.
const HIDE_DISABLED_AGENT_KEY = "kandev:agents:hideDisabledInNav:v1";
// Which integrations the probe reports as connected, per test.
const integrationsEnabled = new Set<string>();

const state = {
  workspaces: {
    activeId: MAIN_WORKSPACE_ID,
    items: [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }],
  },
  settingsAgents: {
    items: [
      {
        name: "claude-code",
        profiles: [
          { id: "profile-1", name: "Default", agentDisplayName: AGENT_LABEL },
          { id: "profile-2", name: "Retired", agentDisplayName: AGENT_LABEL, enabled: false },
        ],
      },
    ],
  },
  executors: {
    items: [
      {
        id: "exec-1",
        name: "Local Executor",
        type: "local",
        profiles: [{ id: "exec-profile-1", name: "Local" }],
      },
    ],
  },
  // The branch orders agents the way the Agents page groups them, so it reads
  // the CLI scan. Empty here: one agent, so there is no order to disturb.
  agentDiscovery: {
    items: [] as Array<{ name: string; available: boolean }>,
    loaded: false,
  },
  settingsData: {
    executorsLoaded: true,
    agentsLoaded: true,
  },
  userSettings: {
    loaded: true,
    savedLayouts: [] as Array<{ id: string }>,
  },
  auth: {
    mode: "disabled" as string,
    authenticated: true,
    user: null as { role: string } | null,
  },
  settingsMenu: {
    mode: "flat" as SettingsMenuMode,
    savedMode: "flat" as SettingsMenuMode,
    expandedKeys: [] as string[],
  },
  setSettingsMenuExpandedKeys: (keys: string[]) => {
    state.settingsMenu.expandedKeys = keys;
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

// Per-test feature flags, so a row that only exists behind one can be driven
// both ways rather than being permanently invisible to this suite.
const featuresOn = new Set<string>();

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (name: string) => featuresOn.has(name),
}));

vi.mock("@/hooks/domains/settings/use-secrets", () => ({
  useSecrets: () => ({
    items: [{ id: "secret-1" }, { id: "secret-2" }],
    loaded: true,
    loading: false,
  }),
}));

vi.mock("@/hooks/domains/integrations/use-enabled-integrations", () => ({
  useEnabledIntegrations: () => integrationsEnabled,
}));

vi.mock("@/hooks/domains/plugins/use-plugins", () => ({
  usePlugins: () => ({ items: [{ id: "plugin-1" }], loaded: true, loading: false, error: null }),
}));

import { SettingsTree } from "./settings-tree";

function setMenuMode(mode: SettingsMenuMode, expandedKeys: string[] = []) {
  state.settingsMenu.mode = mode;
  state.settingsMenu.savedMode = mode;
  state.settingsMenu.expandedKeys = expandedKeys;
}

const FONT_NORMAL = "font-normal";
const FONT_MEDIUM = "font-medium";
/** The glyph box every row opens, whether or not a glyph fills it. */
const GLYPH_BOX = "h-3.5 w-3.5";
/** The record mark — shared with the dot the Agents page renders per profile. */
const RECORD_DOT = "h-1.5 w-1.5";

/**
 * `data-record` for a row, whether it is a leaf (which carries the attribute
 * itself) or a branch (whose wrapper carries it, so an href-less agent row gets
 * it too). Deliberately not `closest`: every row inside an open record branch
 * has a marked ancestor, which would make the negative assertions vacuous.
 */
function recordFlag(row: HTMLElement): string | null {
  return row.getAttribute("data-record") ?? row.parentElement?.getAttribute("data-record") ?? null;
}

/** Every row claiming to be the page you are on. Must always be exactly one. */
function activeRowNames(): string[] {
  return Array.from(document.querySelectorAll("[data-active='true']")).map(
    (element) => element.textContent ?? "",
  );
}

describe("SettingsTree static menu", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
    setMenuMode("flat");
  });

  afterEach(() => {
    cleanup();
    featuresOn.clear();
    state.auth.mode = "disabled";
    state.auth.user = null;
  });

  it("renders section headers as static text, not links or buttons", () => {
    render(<SettingsTree pathname="/settings" />);

    for (const header of ["Preferences", "Workspaces & Access", "System"]) {
      expect(screen.getByText(header)).toBeTruthy();
      expect(screen.queryByRole("link", { name: header })).toBeNull();
      expect(screen.queryByRole("button", { name: header })).toBeNull();
    }
  });

  it("holds no rows for user-created data, so menu length is constant", () => {
    render(<SettingsTree pathname="/settings" />);

    // Store has a workspace, an agent profile and an executor profile — none
    // of them may appear as menu rows.
    expect(screen.queryByRole("link", { name: WORKSPACE_ROW })).toBeNull();
    expect(screen.queryByRole("link", { name: new RegExp(AGENT_LABEL) })).toBeNull();
    expect(screen.queryByRole("link", { name: "Local" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Repositories" })).toBeNull();

    // One row per page.
    expect(screen.getByRole("link", { name: /^Workspaces/ }).getAttribute("href")).toBe(
      "/settings/workspaces",
    );
    expect(screen.getByRole("link", { name: /^Agents/ }).getAttribute("href")).toBe(
      "/settings/agents",
    );
    expect(screen.getByRole("link", { name: /^Executors/ }).getAttribute("href")).toBe(
      "/settings/executors",
    );
  });

  it("marks the owning page row active for detail routes", () => {
    render(<SettingsTree pathname="/settings/agents/claude-code/profiles/profile-1" />);

    expect(screen.getByRole("link", { name: /^Agents/ }).getAttribute("data-active")).toBe("true");
    expect(screen.getByRole("link", { name: /^Executors/ }).getAttribute("data-active")).toBeNull();
  });

  it("hides auth-gated rows and the Access Control section when auth is disabled", () => {
    render(<SettingsTree pathname="/settings" />);

    expect(screen.queryByText("Access Control")).toBeNull();
    expect(screen.queryByRole("link", { name: "Users" })).toBeNull();
    expect(screen.queryByRole("link", { name: "API Tokens" })).toBeNull();
  });

  // An organization is a boundary above users, so it belongs with the other
  // who-can-reach-what settings. It used to sit under System, which is for
  // operating the instance.
  it("puts Organizations in Access Control, not System", () => {
    featuresOn.add("auth");
    featuresOn.add("multiTenancy");
    state.auth.mode = "enabled";
    state.auth.user = { role: "admin" };

    render(<SettingsTree pathname="/settings" />);

    const access = screen.getByTestId("settings-section-access");
    expect(within(access).getByRole("link", { name: "Organizations" })).toBeTruthy();
    const system = screen.queryByTestId("settings-section-system");
    if (system) {
      expect(within(system).queryByRole("link", { name: "Organizations" })).toBeNull();
    }
  });

  it("hides Organizations when the feature is off, rather than linking a page that refuses", () => {
    featuresOn.add("auth");
    state.auth.mode = "enabled";
    state.auth.user = { role: "admin" };

    render(<SettingsTree pathname="/settings" />);

    expect(screen.queryByRole("link", { name: "Organizations" })).toBeNull();
  });

  it("keeps a section inside Task Behavior instead of rendering it as a page row", () => {
    render(<SettingsTree pathname="/settings/preferences/task-behavior" />);

    expect(screen.queryByRole("link", { name: NESTED_SECTION_LABEL })).toBeNull();
    expect(screen.getByRole("link", { name: "Task Behavior" }).getAttribute("data-active")).toBe(
      "true",
    );
  });

  it("puts the count straight after the title, not out at the row's edge", () => {
    render(<SettingsTree pathname="/settings" />);

    const row = screen.getByRole("link", { name: "Workspaces 1" });
    const title = within(row).getByText("Workspaces");
    const count = title.nextElementSibling;

    // Immediately after the title and sharing its parent — a right-aligned
    // badge would sit outside the title's wrapper instead.
    expect(count?.textContent).toBe("1");
    expect(count?.parentElement).toBe(title.parentElement);
  });

  it("shows item counts on rows whose page owns a list", () => {
    render(<SettingsTree pathname="/settings" />);

    // One workspace, two agent profiles (one disabled — the badge does not
    // change the count), one executor profile, two secrets, one plugin.
    expect(screen.getByRole("link", { name: "Workspaces 1" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Agents 2" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Executors 1" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Global Secrets 2" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Plugins 1" })).toBeTruthy();
    // Rows without a list never carry a badge.
    expect(screen.getByRole("link", { name: "Appearance" }).textContent).toBe("Appearance");
  });
});

describe("SettingsTree tree modes", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
    state.agentDiscovery.items = [];
    state.agentDiscovery.loaded = false;
    integrationsEnabled.clear();
  });

  afterEach(() => {
    setMenuMode("flat");
    cleanup();
  });

  it("grows the branched rows into their records and opens the route's path", () => {
    setMenuMode("accordion");
    render(
      <SettingsTree pathname={`/settings/workspaces/${MAIN_WORKSPACE_ID}/integrations/github`} />,
    );

    // Workspaces › Main Workspace › Integrations › GitHub — the same chain the
    // breadcrumb renders on that page.
    expect(screen.getByRole("link", { name: /^Workspaces/ })).toBeTruthy();
    expect(screen.getByRole("link", { name: WORKSPACE_ROW })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Integrations" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "GitHub" }).getAttribute("href")).toBe(
      `/settings/workspaces/${MAIN_WORKSPACE_ID}/integrations/github`,
    );
  });

  it("marks only the deepest row active, never its ancestors", () => {
    setMenuMode("accordion");
    render(
      <SettingsTree pathname={`/settings/workspaces/${MAIN_WORKSPACE_ID}/integrations/github`} />,
    );

    expect(activeRowNames()).toEqual(["GitHub"]);
  });

  it("falls back to the owning row when no branch node claims the route", () => {
    setMenuMode("accordion");
    // `/settings/agents/browse` is the install catalogue, not an agent, so no
    // node holds it — the Agents row has to keep the active mark itself.
    render(<SettingsTree pathname="/settings/agents/browse" />);

    expect(activeRowNames()).toEqual(["Agents2"]);
  });

  it("collapses the other branches when one is opened in accordion mode", () => {
    setMenuMode("accordion");
    render(<SettingsTree pathname={`/settings/workspaces/${MAIN_WORKSPACE_ID}`} />);

    expect(screen.getByRole("link", { name: WORKSPACE_ROW })).toBeTruthy();
    expect(screen.queryByRole("button", { name: AGENT_LABEL })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Expand Agents" }));

    expect(screen.getByRole("button", { name: AGENT_LABEL })).toBeTruthy();
    expect(screen.queryByRole("link", { name: WORKSPACE_ROW })).toBeNull();
  });

  it("keeps several branches open at once in persistent mode", () => {
    setMenuMode("persistent", [
      WORKSPACES_ROW_KEY,
      `workspace:${MAIN_WORKSPACE_ID}`,
      AGENTS_ROW_KEY,
      AGENT_KEY,
      "row:/settings/executors",
      "executor:exec-1",
    ]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    expect(screen.getByRole("link", { name: WORKSPACE_ROW })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Repositories" })).toBeTruthy();
    expect(screen.getByRole("button", { name: AGENT_LABEL })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Default" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Local" })).toBeTruthy();
    // A page outside every branch still leaves exactly one active row.
    expect(activeRowNames()).toEqual(["Appearance"]);
  });

  it("links an agent's profile under the agent, which only discloses", () => {
    setMenuMode("persistent", [AGENTS_ROW_KEY, AGENT_KEY]);
    render(<SettingsTree pathname="/settings/agents/claude-code/profiles/profile-1" />);

    // `/settings/agents/<name>` redirects back to the index for a saved agent,
    // so the agent row must not be a link.
    expect(screen.queryByRole("link", { name: new RegExp(AGENT_LABEL) })).toBeNull();
    expect(screen.getByRole("link", { name: "Default" }).getAttribute("href")).toBe(
      "/settings/agents/claude-code/profiles/profile-1",
    );
    expect(activeRowNames()).toEqual(["Default"]);
  });
});

/**
 * The visual split between rows kandev ships and rows the user made. Its own
 * block: the tree-mode suite is about what the menu contains, this one is about
 * how a record is drawn, and together they overran the per-function line limit.
 */
describe("SettingsTree record treatment", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
    state.agentDiscovery.items = [];
    state.agentDiscovery.loaded = false;
    integrationsEnabled.clear();
  });

  afterEach(() => {
    setMenuMode("flat");
    cleanup();
  });

  it("lightens rows from the user's data, leaving navigation alone", () => {
    setMenuMode("accordion");
    render(
      <SettingsTree pathname={`/settings/workspaces/${MAIN_WORKSPACE_ID}/integrations/github`} />,
    );

    const workspace = screen.getByRole("link", { name: WORKSPACE_ROW });
    // `GitHub` sits at the same indent as the workspace name but ships with the
    // app, so the split cannot be read off depth.
    const integration = screen.getByRole("link", { name: "GitHub" });

    expect(recordFlag(workspace)).toBe("true");
    expect(recordFlag(integration)).toBeNull();
    expect(workspace.className).toContain(FONT_NORMAL);
    expect(integration.className).toContain(FONT_MEDIUM);
  });

  it("treats a profile as the record, not the agent it belongs to", () => {
    setMenuMode("persistent", [AGENTS_ROW_KEY, AGENT_KEY]);
    render(<SettingsTree pathname="/settings/agents/claude-code/profiles/profile-1" />);

    const agent = screen.getByRole("button", { name: AGENT_LABEL });
    const profile = screen.getByRole("link", { name: "Default" });

    // The agent ships with kandev: full weight.
    expect(recordFlag(agent)).toBeNull();
    expect(agent.className).toContain(FONT_MEDIUM);
    // Its profile is the user's.
    expect(recordFlag(profile)).toBe("true");
    expect(profile.className).toContain(FONT_NORMAL);
  });

  it("marks a record with the dot its own page uses, in a full glyph box", () => {
    setMenuMode("persistent", [AGENTS_ROW_KEY, AGENT_KEY, WORKSPACES_ROW_KEY]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    for (const name of [WORKSPACE_ROW, /^Default$/]) {
      const box = screen.getByRole("link", { name }).firstElementChild;
      // Same footprint as a tabler glyph, so the label lands in the same column.
      expect(box?.getAttribute("class")).toContain(GLYPH_BOX);
      // The dot the Agents page puts in front of a profile.
      expect(box?.firstElementChild?.getAttribute("class")).toContain(RECORD_DOT);
    }
  });

  it("leaves a row that has a glyph of its own alone", () => {
    setMenuMode("persistent", [AGENTS_ROW_KEY, AGENT_KEY, WORKSPACES_ROW_KEY, WORKSPACE_KEY]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    // An agent keeps its logo, and a workspace tab its icon — neither is a
    // record, and neither gains a dot.
    for (const row of [
      screen.getByRole("button", { name: AGENT_LABEL }),
      screen.getByRole("link", { name: "Repositories" }),
    ]) {
      expect(row.querySelector("svg, img")).not.toBeNull();
      expect(row.innerHTML).not.toContain(RECORD_DOT);
    }
  });
});

describe("SettingsTree badges", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
    state.agentDiscovery.items = [];
    state.agentDiscovery.loaded = false;
    integrationsEnabled.clear();
    window.localStorage.removeItem(HIDE_DISABLED_AGENT_KEY);
  });

  afterEach(() => {
    setMenuMode("flat");
    window.localStorage.removeItem(HIDE_DISABLED_AGENT_KEY);
    cleanup();
  });

  it("badges a disabled profile, which stays listed so it can be re-enabled", () => {
    setMenuMode("persistent", [AGENTS_ROW_KEY, AGENT_KEY]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    // Ported from #2339, which added this to the accordion tree the static menu
    // replaced. A disabled profile is hidden from every task and session picker
    // but stays here, so the menu has to say why it looks inert.
    expect(screen.getByRole("link", { name: /Retired/ }).textContent).toContain("Disabled");
    expect(screen.getByRole("link", { name: "Default" }).textContent).not.toContain("Disabled");
  });

  it("hides a disabled profile from the tree when the hide setting is on", () => {
    window.localStorage.setItem(HIDE_DISABLED_AGENT_KEY, "true");
    setMenuMode("persistent", [AGENTS_ROW_KEY, AGENT_KEY]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    expect(screen.queryByRole("link", { name: /Retired/ })).toBeNull();
    expect(screen.getByRole("link", { name: "Default" })).toBeTruthy();
  });

  it("reveals a disabled profile again once the hide setting is turned back off", () => {
    window.localStorage.setItem(HIDE_DISABLED_AGENT_KEY, "true");
    setMenuMode("persistent", [AGENTS_ROW_KEY, AGENT_KEY]);
    const { rerender } = render(<SettingsTree pathname="/settings/preferences/appearance" />);
    expect(screen.queryByRole("link", { name: /Retired/ })).toBeNull();

    window.localStorage.removeItem(HIDE_DISABLED_AGENT_KEY);
    window.dispatchEvent(new Event("kandev:agents:hide-disabled-in-nav-changed"));
    rerender(<SettingsTree pathname="/settings/preferences/appearance" />);

    expect(screen.getByRole("link", { name: /Retired/ }).textContent).toContain("Disabled");
  });

  it("badges the active workspace, the way its own list does", () => {
    setMenuMode("persistent", [WORKSPACES_ROW_KEY]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    // `MAIN_WORKSPACE_ID` is the active one in the mocked store.
    expect(
      screen.getByRole("link", { name: new RegExp(MAIN_WORKSPACE_NAME) }).textContent,
    ).toContain("Active");
  });

  it("badges only the active workspace", () => {
    state.workspaces.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: "ws-2", name: "Second Workspace" },
    ];
    setMenuMode("persistent", [WORKSPACES_ROW_KEY]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    expect(screen.getByRole("link", { name: /Second Workspace/ }).textContent).not.toContain(
      "Active",
    );
  });

  it("badges an agent whose CLI the scan cannot find", () => {
    state.agentDiscovery.items = [{ name: "claude-code", available: false }];
    state.agentDiscovery.loaded = true;
    setMenuMode("persistent", [AGENTS_ROW_KEY]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    // Its profiles stay listed below, so the agent row says why none can run.
    expect(screen.getByRole("button", { name: AGENT_ROW }).textContent).toContain("Not installed");
  });

  it("says nothing about installation before the scan has reported", () => {
    state.agentDiscovery.items = [];
    state.agentDiscovery.loaded = false;
    setMenuMode("persistent", [AGENTS_ROW_KEY]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    // "Not looked yet" is not "not installed" — badging here would flash a
    // wrong claim on every cold load.
    expect(screen.getByRole("button", { name: AGENT_ROW }).textContent).not.toContain(
      "Not installed",
    );
  });

  it("badges an integration the workspace has connected", () => {
    integrationsEnabled.add("github");
    setMenuMode("persistent", [WORKSPACES_ROW_KEY, WORKSPACE_KEY, `${WORKSPACE_KEY}:integrations`]);
    render(<SettingsTree pathname="/settings/preferences/appearance" />);

    expect(screen.getByRole("link", { name: /GitHub/ }).textContent).toContain("Enabled");
    expect(screen.getByRole("link", { name: /GitLab/ }).textContent).not.toContain("Enabled");
  });
});

describe("SettingsTree search", () => {
  afterEach(cleanup);

  it("preserves the normal menu until a query filters it to grouped hits", () => {
    render(<SettingsTree pathname="/settings" />);

    const search = screen.getByRole("searchbox", { name: SEARCH_LABEL });
    expect(screen.queryByRole("link", { name: NESTED_SECTION_LABEL })).toBeNull();

    fireEvent.change(search, { target: { value: "font size" } });

    const result = screen.getByRole("link", { name: /Terminal Font Size/ });
    expect(result.getAttribute("href")).toBe(
      "/settings/preferences/terminal-editors#setting-terminal-font-size",
    );
    expect(result.textContent).toContain("Terminal & Editors");
    expect(screen.queryByRole("link", { name: NESTED_SECTION_LABEL })).toBeNull();
  });

  it("prefixes per-workspace results with the workspace name", () => {
    render(<SettingsTree pathname="/settings" />);

    fireEvent.change(screen.getByRole("searchbox", { name: SEARCH_LABEL }), {
      target: { value: "Jira" },
    });

    const result = screen.getByRole("link", { name: /Jira/ });
    expect(result.getAttribute("href")).toContain(
      `/settings/workspaces/${MAIN_WORKSPACE_ID}/integrations/jira`,
    );
    expect(result.textContent).toContain(`${MAIN_WORKSPACE_NAME} › Integrations`);
  });

  it("clears a query with Escape and restores the normal menu", () => {
    render(<SettingsTree pathname="/settings" />);
    const search = screen.getByRole("searchbox", { name: SEARCH_LABEL });

    fireEvent.change(search, { target: { value: "font size" } });
    fireEvent.keyDown(search, { key: "Escape" });

    expect((search as HTMLInputElement).value).toBe("");
    expect(screen.queryByRole("link", { name: NESTED_SECTION_LABEL })).toBeNull();
  });

  it("announces an empty result without rendering the normal menu", () => {
    render(<SettingsTree pathname="/settings" />);

    fireEvent.change(screen.getByRole("searchbox", { name: SEARCH_LABEL }), {
      target: { value: "definitely missing" },
    });

    expect(screen.getByText("No matching settings")).toBeTruthy();
    expect(screen.queryByRole("link", { name: NESTED_SECTION_LABEL })).toBeNull();
  });
});
