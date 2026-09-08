import { describe, expect, it } from "vitest";

import {
  matchSettingsCrumbRoute,
  resolveSettingsBreadcrumbs,
  type CrumbValues,
} from "./settings-breadcrumbs";

// `t` echoes the key so an assertion names the catalog entry the crumb resolved
// to. A real label would prove the copy, not the wiring.
const t = (key: string) => key;

const SETTINGS = "common:settings";
const WORKSPACES = "common:workspaces";
const WORKSPACE = "common:workspace";
const EXECUTORS = "common:executors";
const AGENTS = "common:agents";
const PLUGINS = "common:plugins";
const INTEGRATIONS = "common:integrations";
const AUTOMATIONS = "common:automations";

const SETTINGS_HREF = "/settings";
const WORKSPACES_HREF = "/settings/workspaces";
const EXECUTORS_HREF = "/settings/executors";
const WS_HREF = `${WORKSPACES_HREF}/ws-1`;
const EXECUTOR_HREF = "/settings/executor/exec-1";

const WS_NAME = "Kanban1";
const EXECUTOR_NAME = "local-docker";
const PROFILE_NAME = "Alpha";
const PLUGIN_NAME = "Time Tracker";

const NAMES: Record<string, string> = {
  "ws-1": WS_NAME,
  "auto-1": "Nightly sync",
  "exec-1": EXECUTOR_NAME,
  "exec-profile-1": PROFILE_NAME,
  "agent-profile-1": "My Profile101",
  claude: "Claude Code",
  "plugin-1": PLUGIN_NAME,
};

function values(overrides: Partial<CrumbValues> = {}): CrumbValues {
  return {
    workspaceName: ({ workspaceId }) => (workspaceId && NAMES[workspaceId]) || null,
    agentDisplayName: ({ agentName }) => (agentName && NAMES[agentName]) || null,
    agentProfileName: ({ agentProfileId }) => (agentProfileId && NAMES[agentProfileId]) || null,
    automationName: ({ automationId }) => (automationId && NAMES[automationId]) || null,
    executorName: ({ executorId }) => (executorId && NAMES[executorId]) || null,
    executorProfileName: ({ executorProfileId }) =>
      (executorProfileId && NAMES[executorProfileId]) || null,
    executorTypeTitle: ({ executorType }) => (executorType ? `New ${executorType} Profile` : null),
    integrationTitle: ({ integrationSlug }) => (integrationSlug === "github" ? "GitHub" : null),
    pluginName: ({ pluginId }) => (pluginId && NAMES[pluginId]) || null,
    ...overrides,
  };
}

/** The crumb chain as `label` strings, so a test reads like the topbar. */
function chain(pathname: string, overrides?: Partial<CrumbValues>): string[] {
  const crumbs = resolveSettingsBreadcrumbs(pathname, t, values(overrides));
  return [...crumbs.parents.map((parent) => parent.label), crumbs.title];
}

function hrefs(pathname: string): Array<string | undefined> {
  return resolveSettingsBreadcrumbs(pathname, t, values()).parents.map((parent) => parent.href);
}

describe("settings breadcrumb chain", () => {
  it("gives the settings index no parents", () => {
    const crumbs = resolveSettingsBreadcrumbs(SETTINGS_HREF, t, values());
    expect(crumbs.parents).toEqual([]);
    expect(crumbs.title).toBe("settings:settings");
  });

  it("keeps the Settings crumb a phone-only link", () => {
    const [settings] = resolveSettingsBreadcrumbs(
      "/settings/preferences/appearance",
      t,
      values(),
    ).parents;
    expect(settings).toEqual({ label: SETTINGS, href: SETTINGS_HREF, phoneOnlyLink: true });
  });

  it("names the owning menu page but never a flat page's own row", () => {
    // Appearance IS the menu row, so it is not its own parent.
    expect(chain("/settings/preferences/appearance")).toEqual([SETTINGS, "settings:appearance"]);
    // Browse is a sub-page of the Agents row, so the row becomes a crumb.
    expect(chain("/settings/agents/browse")).toEqual([
      SETTINGS,
      AGENTS,
      "agents:browseAvailableAgents",
    ]);
  });
});

describe("settings breadcrumb: workspaces", () => {
  it("titles the workspace page with its name and links the list", () => {
    expect(chain(WS_HREF)).toEqual([SETTINGS, WORKSPACES, WS_NAME]);
    expect(hrefs(WS_HREF)).toEqual([SETTINGS_HREF, WORKSPACES_HREF]);
  });

  it("adds the workspace name crumb on its tabs", () => {
    expect(chain(`${WS_HREF}/repositories`)).toEqual([
      SETTINGS,
      WORKSPACES,
      WS_NAME,
      "sidebar:repositories",
    ]);
    expect(hrefs(`${WS_HREF}/repositories`)).toEqual([SETTINGS_HREF, WORKSPACES_HREF, WS_HREF]);
  });

  it("falls back to the Workspace label until the name loads", () => {
    expect(chain(`${WORKSPACES_HREF}/ws-unknown/repositories`)).toEqual([
      SETTINGS,
      WORKSPACES,
      WORKSPACE,
      "sidebar:repositories",
    ]);
  });
});

describe("settings breadcrumb: integrations", () => {
  // Integrations is a real page, so an integration detail hangs off it rather
  // than jumping straight from the workspace to "GitHub".
  it("keeps the Integrations crumb on a service page", () => {
    expect(chain(`${WS_HREF}/integrations/github`)).toEqual([
      SETTINGS,
      WORKSPACES,
      WS_NAME,
      INTEGRATIONS,
      "GitHub",
    ]);
    expect(hrefs(`${WS_HREF}/integrations/github`)).toEqual([
      SETTINGS_HREF,
      WORKSPACES_HREF,
      WS_HREF,
      `${WS_HREF}/integrations`,
    ]);
  });

  it("titles the Integrations tab itself without repeating the crumb", () => {
    expect(chain(`${WS_HREF}/integrations`)).toEqual([SETTINGS, WORKSPACES, WS_NAME, INTEGRATIONS]);
  });
});

describe("settings breadcrumb: automations", () => {
  // The id was skipped as unreadable and the title landed back on the section
  // segment, so an automation used to read "Automations › Automations".
  it("titles an automation with its name", () => {
    expect(chain(`${WS_HREF}/automations/auto-1`)).toEqual([
      SETTINGS,
      WORKSPACES,
      WS_NAME,
      AUTOMATIONS,
      "Nightly sync",
    ]);
  });

  it("names the create page rather than the section", () => {
    expect(chain(`${WS_HREF}/automations/new`)).toEqual([
      SETTINGS,
      WORKSPACES,
      WS_NAME,
      AUTOMATIONS,
      "automations:newAutomation",
    ]);
  });

  it("falls back to the Automation label, never to the section label", () => {
    expect(chain(`${WS_HREF}/automations/auto-gone`)).toEqual([
      SETTINGS,
      WORKSPACES,
      WS_NAME,
      AUTOMATIONS,
      "common:automation",
    ]);
  });
});

describe("settings breadcrumb: executors", () => {
  // Both spellings are live: `/executors` is the page listing every profile,
  // `/executor/<id>` an executor's own page. Neither had any crumb chain.
  it("puts every executor route under the Executors page", () => {
    expect(chain(`${EXECUTORS_HREF}/exec-profile-1`)).toEqual([SETTINGS, EXECUTORS, PROFILE_NAME]);
    expect(chain(`${EXECUTORS_HREF}/ssh/exec-1`)).toEqual([SETTINGS, EXECUTORS, EXECUTOR_NAME]);
    expect(chain(`${EXECUTORS_HREF}/k8s/exec-1`)).toEqual([SETTINGS, EXECUTORS, EXECUTOR_NAME]);
    expect(chain(`${EXECUTORS_HREF}/new/local_docker`)).toEqual([
      SETTINGS,
      EXECUTORS,
      "New local_docker Profile",
    ]);
    expect(chain(EXECUTOR_HREF)).toEqual([SETTINGS, EXECUTORS, EXECUTOR_NAME]);
    expect(chain("/settings/executor/new")).toEqual([SETTINGS, EXECUTORS, "settings:new"]);
  });

  it("names the executor a profile belongs to", () => {
    expect(chain(`${EXECUTOR_HREF}/profile/exec-profile-1`)).toEqual([
      SETTINGS,
      EXECUTORS,
      EXECUTOR_NAME,
      PROFILE_NAME,
    ]);
    expect(hrefs(`${EXECUTOR_HREF}/profile/exec-profile-1`)).toEqual([
      SETTINGS_HREF,
      EXECUTORS_HREF,
      EXECUTOR_HREF,
    ]);
  });

  it("does not title an unknown executor type off the URL", () => {
    // "local_docker" title-cased is "Local_docker" — the old behaviour.
    expect(
      resolveSettingsBreadcrumbs(
        `${EXECUTORS_HREF}/new/local_docker`,
        t,
        values({ executorTypeTitle: () => null }),
      ),
    ).toMatchObject({ title: "executors:newProfile", titleFromUrlSegment: false });
  });
});

describe("settings breadcrumb: agents", () => {
  it("titles an agent with its display name, directly under Agents", () => {
    expect(chain("/settings/agents/claude")).toEqual([SETTINGS, AGENTS, "Claude Code"]);
  });

  it("titles an agent profile with the profile name", () => {
    expect(chain("/settings/agents/claude/profiles/agent-profile-1")).toEqual([
      SETTINGS,
      AGENTS,
      "My Profile101",
    ]);
  });
});

describe("settings breadcrumb: plugins", () => {
  it("titles a plugin page with its display name", () => {
    expect(chain("/settings/plugins/plugin-1")).toEqual([SETTINGS, PLUGINS, PLUGIN_NAME]);
  });

  it("names the plugin as the parent of its own sub-pages", () => {
    expect(chain("/settings/plugins/plugin-1/reports")).toEqual([
      SETTINGS,
      PLUGINS,
      PLUGIN_NAME,
      "Reports",
    ]);
  });

  it("falls back to the plugin id, which no catalog can name", () => {
    const crumbs = resolveSettingsBreadcrumbs("/settings/plugins/time-tracker", t, values());
    expect(crumbs.title).toBe(PLUGIN_NAME);
    expect(crumbs.titleFromUrlSegment).toBe(true);
  });

  it("falls back to the plugin id, not the sub-page it is the parent of", () => {
    // A crumb that falls back to the *page's* deepest segment would label the
    // plugin "Reports" — its own child's name.
    expect(chain("/settings/plugins/time-tracker/reports")).toEqual([
      SETTINGS,
      PLUGINS,
      PLUGIN_NAME,
      "Reports",
    ]);
  });
});

describe("settings breadcrumb: routes with no crumb row", () => {
  it("still names the owning page for a legacy workspace path", () => {
    // Redirect frames get no crumb row of their own, but the menu knows the
    // legacy prefix, so the chain is still oriented.
    expect(chain("/settings/workspace/ws-1/integrations/github")).toEqual([
      SETTINGS,
      WORKSPACES,
      "GitHub",
    ]);
  });

  it("reports a title it had to take off the URL", () => {
    expect(resolveSettingsBreadcrumbs("/settings/not-a-real-page", t, values())).toMatchObject({
      title: "Not A Real Page",
      titleFromUrlSegment: true,
    });
  });
});

describe("settings breadcrumb: malformed ids", () => {
  const BAD = `${WORKSPACES_HREF}/%E0%A4%A/repositories`;

  it("labels the crumb from the catalog instead of the broken segment", () => {
    expect(chain(BAD)).toEqual([SETTINGS, WORKSPACES, WORKSPACE, "sidebar:repositories"]);
  });

  it("renders the crumb as static text rather than linking somewhere broken", () => {
    expect(hrefs(BAD)).toEqual([SETTINGS_HREF, WORKSPACES_HREF, undefined]);
  });
});

describe("matchSettingsCrumbRoute", () => {
  it("decodes captured ids so lookups and labels see the real value", () => {
    const matched = matchSettingsCrumbRoute(`${WORKSPACES_HREF}/ws%20one/automations/a%2Fb`);
    expect(matched?.params).toEqual({ workspaceId: "ws one", automationId: "a/b" });
  });

  it("re-encodes ids when filling a crumb href", () => {
    expect(hrefs(`${WORKSPACES_HREF}/ws%20one/repositories`)).toEqual([
      SETTINGS_HREF,
      WORKSPACES_HREF,
      `${WORKSPACES_HREF}/ws%20one`,
    ]);
  });

  it("prefers the reserved route over the record route it would shadow", () => {
    expect(matchSettingsCrumbRoute("/settings/executor/new")?.params).toEqual({});
    expect(matchSettingsCrumbRoute("/settings/agents/browse")?.params).toEqual({});
    expect(matchSettingsCrumbRoute(`${WS_HREF}/automations/new`)?.params).toEqual({
      workspaceId: "ws-1",
    });
  });
});
