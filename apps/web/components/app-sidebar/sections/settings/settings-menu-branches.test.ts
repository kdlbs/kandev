import { describe, expect, it } from "vitest";

import {
  buildAgentsBranch,
  buildBranchRoot,
  buildExecutorsBranch,
  buildWorkspacesBranch,
  findActiveNodePath,
  findNodeKeyPath,
  findSubtreeKeys,
  settingsMenuRowKey,
  type SettingsMenuNode,
} from "./settings-menu-branches";

const WORKSPACE_ID = "ws-1";
const WORKSPACES_HREF = "/settings/workspaces";
const EXECUTORS_HREF = "/settings/executors";
const WORKSPACES = [{ id: WORKSPACE_ID, name: "Main Workspace" }];
const ORDERED_WORKSPACES = [
  { id: "ws-first", name: "First" },
  { id: "ws-active", name: "Active" },
  { id: "ws-last", name: "Last" },
];
// Hoisted so the filter tests can build fixtures without re-spelling the
// display name (sonar duplicate-literal rule).
const AGENT_DISPLAY_NAME = "Claude Code";
const AGENTS = [
  {
    name: "claude-code",
    profiles: [
      { id: "profile-1", name: "Default", agentDisplayName: "Claude Code" },
      { id: "profile-2", name: "Review", agentDisplayName: "Claude Code" },
    ],
  },
];
const EXECUTORS = [
  {
    id: "exec-1",
    name: "Local Executor",
    type: "local",
    profiles: [{ id: "exec-profile-1", name: "Local" }],
  },
];

function hrefsOf(nodes: readonly SettingsMenuNode[]): Array<string | null> {
  return nodes.map((node) => node.href);
}

/** The Integrations tab among a workspace node's children. */
function integrationsTabOf(tabs: readonly SettingsMenuNode[]): SettingsMenuNode | undefined {
  return tabs.find((node) => node.href?.endsWith("/integrations"));
}

function integrationSlugsOf(tabs: readonly SettingsMenuNode[]): Array<string | undefined> {
  return (integrationsTabOf(tabs)?.children ?? []).map((node) => node.integrationSlug);
}

describe("buildWorkspacesBranch", () => {
  it("places the active workspace first without disturbing the remaining order", () => {
    const branch = buildWorkspacesBranch(ORDERED_WORKSPACES, "ws-active");

    expect(branch.map((workspace) => workspace.key)).toEqual([
      "workspace:ws-active",
      "workspace:ws-first",
      "workspace:ws-last",
    ]);
    expect(branch[0].badge).toBe("active");
  });

  it("hangs each workspace's tabs under it, minus its own overview page", () => {
    const [workspace] = buildWorkspacesBranch(WORKSPACES);

    expect(workspace.href).toBe(`/settings/workspaces/${WORKSPACE_ID}`);
    expect(hrefsOf(workspace.children ?? [])).toEqual([
      `/settings/workspaces/${WORKSPACE_ID}/repositories`,
      `/settings/workspaces/${WORKSPACE_ID}/workflows`,
      `/settings/workspaces/${WORKSPACE_ID}/integrations`,
      `/settings/workspaces/${WORKSPACE_ID}/automations`,
      `/settings/workspaces/${WORKSPACE_ID}/secrets`,
    ]);
  });

  it("goes one level deeper for integrations, the menu's deepest branch", () => {
    const [workspace] = buildWorkspacesBranch(WORKSPACES);
    const integrations = integrationsTabOf(workspace.children ?? []);

    expect(hrefsOf(integrations?.children ?? [])).toContain(
      `/settings/workspaces/${WORKSPACE_ID}/integrations/github`,
    );
  });

  it("appends registered provider integrations using the native workspace route", () => {
    const IntegrationIcon = () => null;
    const [workspace] = buildWorkspacesBranch(WORKSPACES, null, undefined, [
      { id: "bitbucket", pluginId: "plugin-bitbucket", label: "Bitbucket", icon: IntegrationIcon },
    ]);
    const integration = integrationsTabOf(workspace.children ?? [])?.children?.find(
      (node) => node.key === `workspace:${WORKSPACE_ID}:integrations:bitbucket`,
    );

    expect(integration).toMatchObject({
      href: `/settings/workspaces/${WORKSPACE_ID}/integrations/bitbucket`,
      label: { text: "Bitbucket" },
      icon: IntegrationIcon,
    });
  });

  it("filters plugin integrations per workspace and keeps unknown state visible", () => {
    const IntegrationIcon = () => null;
    const workspaces = [
      { id: "workspace-disabled", name: "Disabled" },
      { id: "workspace-enabled", name: "Enabled" },
      { id: "workspace-unknown", name: "Unknown" },
    ];
    const enabledByWorkspace: Record<string, boolean | undefined> = {
      "workspace-disabled": false,
      "workspace-enabled": true,
    };
    const [disabled, enabled, unknown] = buildWorkspacesBranch(
      workspaces,
      null,
      undefined,
      [
        {
          id: "bitbucket",
          pluginId: "plugin-bitbucket",
          label: "Bitbucket",
          icon: IntegrationIcon,
        },
      ],
      (_integrationId, workspaceId) => enabledByWorkspace[workspaceId],
    );

    expect(integrationSlugsOf(disabled.children ?? [])).not.toContain("bitbucket");
    expect(integrationSlugsOf(enabled.children ?? [])).toContain("bitbucket");
    expect(integrationSlugsOf(unknown.children ?? [])).toContain("bitbucket");
  });

  it("gives every tab and every integration its own mark", () => {
    const [workspace] = buildWorkspacesBranch(WORKSPACES);
    const tabs = workspace.children ?? [];
    const integrations = integrationsTabOf(tabs)?.children ?? [];

    // A row with no icon sits text-first and breaks the column the rest form.
    expect(tabs.every((node) => node.icon !== undefined)).toBe(true);
    expect(integrations.every((node) => node.icon !== undefined)).toBe(true);
    // Marks identify — two sections sharing one would defeat the point.
    expect(new Set(tabs.map((node) => node.icon)).size).toBe(tabs.length);
    expect(new Set(integrations.map((node) => node.icon)).size).toBe(integrations.length);
  });

  it("percent-encodes an id that would otherwise break the path", () => {
    const [workspace] = buildWorkspacesBranch([{ id: "a/b", name: "Slashy" }]);

    expect(workspace.href).toBe("/settings/workspaces/a%2Fb");
  });
});

describe("buildWorkspacesBranch integration visibility", () => {
  function integrationSlugsOf(nodes: readonly SettingsMenuNode[]): Array<string | undefined> {
    return (integrationsTabOf(nodes)?.children ?? []).map((node) => node.integrationSlug);
  }

  it("lists every integration when no visible set is given — the default", () => {
    const [workspace] = buildWorkspacesBranch(WORKSPACES);

    expect(integrationSlugsOf(workspace.children ?? [])).toEqual([
      "azure-devops",
      "github",
      "gitlab",
      "jira",
      "linear",
      "sentry",
    ]);
  });

  it("drops the integrations missing from the visible set, keeping catalog order", () => {
    const [workspace] = buildWorkspacesBranch(
      WORKSPACES,
      null,
      () => new Set(["azure-devops", "linear"] as const),
    );

    expect(integrationSlugsOf(workspace.children ?? [])).toEqual(["azure-devops", "linear"]);
  });

  it("resolves the visible set per workspace, since the toggles are per workspace", () => {
    // Disabling GitHub in one workspace must not strip it from another's
    // branch, which is exactly what a single shared set used to do.
    const branch = buildWorkspacesBranch(
      [
        { id: "ws-1", name: "First" },
        { id: "ws-2", name: "Second" },
      ],
      null,
      (workspaceId) =>
        workspaceId === "ws-1" ? new Set(["jira"] as const) : new Set(["github"] as const),
    );

    expect(integrationSlugsOf(branch[0].children ?? [])).toEqual(["jira"]);
    expect(integrationSlugsOf(branch[1].children ?? [])).toEqual(["github"]);
  });

  it("never consults whether an integration is configured — the badge owns that", () => {
    // Nothing about credentials reaches this builder: an integration the user
    // has never connected is listed exactly like a connected one, so the
    // visible set is the only thing that can remove a row.
    const [workspace] = buildWorkspacesBranch(WORKSPACES, null, () => new Set(["github"] as const));

    expect(integrationSlugsOf(workspace.children ?? [])).toEqual(["github"]);
  });

  it("leaves the Integrations row navigable when the set hides all of them", () => {
    // An empty branch would otherwise render a chevron opening onto nothing.
    const [workspace] = buildWorkspacesBranch(WORKSPACES, null, () => new Set());
    const integrations = integrationsTabOf(workspace.children ?? []);

    expect(integrations?.children).toEqual([]);
    expect(integrations?.href).toBe(`/settings/workspaces/${WORKSPACE_ID}/integrations`);
  });
});

describe("isUserRecord", () => {
  it("marks the nodes the user created: workspaces and profiles", () => {
    const [workspace] = buildWorkspacesBranch(WORKSPACES);
    const [agent] = buildAgentsBranch(AGENTS);
    const [executor] = buildExecutorsBranch(EXECUTORS);

    expect(workspace.isUserRecord).toBe(true);
    expect(agent.children?.every((node) => node.isUserRecord)).toBe(true);
    expect(executor.children?.every((node) => node.isUserRecord)).toBe(true);
  });

  it("leaves the agent and the executor themselves unmarked", () => {
    const [agent] = buildAgentsBranch(AGENTS);
    const [executor] = buildExecutorsBranch(EXECUTORS);

    // `Claude` and `Local` ship with kandev and read the same in every install.
    // The user authors the profiles under them, not the agent or executor.
    expect(agent.isUserRecord).toBeFalsy();
    expect(executor.isUserRecord).toBeFalsy();
  });

  it("leaves fixed structure unmarked, including at a record's own depth", () => {
    const [workspace] = buildWorkspacesBranch(WORKSPACES);
    const tabs = workspace.children ?? [];
    const integrations = integrationsTabOf(tabs)?.children ?? [];

    // These are the assertions that hold the line: a workspace's tabs are a
    // fixed six, and the integrations a fixed seven, in every install — even
    // though `GitHub` sits at the same indent as a workspace name.
    expect(tabs.every((node) => !node.isUserRecord)).toBe(true);
    expect(integrations.length).toBeGreaterThan(0);
    expect(integrations.every((node) => !node.isUserRecord)).toBe(true);
  });

  it("does not mark a branch root, which is a menu row", () => {
    const root = buildBranchRoot(
      { href: WORKSPACES_HREF, labelKey: "common:workspaces" },
      buildWorkspacesBranch(WORKSPACES),
    );

    expect(root.isUserRecord).toBeFalsy();
  });
});

describe("buildAgentsBranch", () => {
  it("gives the agent no page of its own, only its profiles", () => {
    const [agent] = buildAgentsBranch(AGENTS);

    // `/settings/agents/<name>` redirects back to the index for a saved agent.
    expect(agent.href).toBeNull();
    expect(agent.ownsPrefixes).toEqual(["/settings/agents/claude-code/"]);
    expect(hrefsOf(agent.children ?? [])).toEqual([
      "/settings/agents/claude-code/profiles/profile-1",
      "/settings/agents/claude-code/profiles/profile-2",
    ]);
  });

  it("drops an agent with no profiles rather than rendering a dead row", () => {
    expect(buildAgentsBranch([{ name: "empty", profiles: [] }])).toEqual([]);
  });

  it("omits disabled profiles from the branch only when the hide setting is on", () => {
    const mixedAgent = {
      name: "claude-code",
      profiles: [
        { id: "profile-1", name: "Default", agentDisplayName: AGENT_DISPLAY_NAME },
        { id: "profile-2", name: "Retired", agentDisplayName: AGENT_DISPLAY_NAME, enabled: false },
        { id: "profile-3", name: "Legacy", agentDisplayName: AGENT_DISPLAY_NAME },
      ],
    };

    const [agent] = buildAgentsBranch([mixedAgent], undefined, true);

    // Enabled and legacy (no `enabled` field) profiles stay; only `enabled ===
    // false` is omitted.
    expect(hrefsOf(agent.children ?? [])).toEqual([
      "/settings/agents/claude-code/profiles/profile-1",
      "/settings/agents/claude-code/profiles/profile-3",
    ]);
  });

  it("keeps disabled profiles listed when the hide setting is off or absent", () => {
    const mixedAgent = {
      name: "claude-code",
      profiles: [
        { id: "profile-1", name: "Default", agentDisplayName: AGENT_DISPLAY_NAME },
        { id: "profile-2", name: "Retired", agentDisplayName: AGENT_DISPLAY_NAME, enabled: false },
      ],
    };

    const [agent] = buildAgentsBranch([mixedAgent]);
    const [agentWithOff] = buildAgentsBranch([mixedAgent], undefined, false);

    expect(hrefsOf(agent.children ?? [])).toHaveLength(2);
    expect(hrefsOf(agentWithOff.children ?? [])).toHaveLength(2);
  });
});

describe("buildExecutorsBranch", () => {
  it("uses the executor-scoped profile route, the one with an executor crumb", () => {
    const [executor] = buildExecutorsBranch(EXECUTORS);

    expect(executor.href).toBe("/settings/executor/exec-1");
    expect(hrefsOf(executor.children ?? [])).toEqual([
      "/settings/executor/exec-1/profile/exec-profile-1",
    ]);
  });

  it("makes a configured Kubernetes executor disclosure-only and links its profile root", () => {
    const [executor] = buildExecutorsBranch([
      {
        id: "cluster/primary",
        name: "Kubernetes",
        type: "k8s",
        profiles: [{ id: "profile/primary", name: "Primary" }],
      },
    ]);

    expect(executor.href).toBeNull();
    expect(hrefsOf(executor.children ?? [])).toEqual(["/settings/executors/profile%2Fprimary"]);
  });

  it("keeps the Kubernetes connection route reachable for an executor without profiles", () => {
    const [executor] = buildExecutorsBranch([
      {
        id: "cluster/primary",
        name: "Kubernetes",
        type: "k8s",
        profiles: [],
      },
    ]);

    expect(executor.href).toBe("/settings/executors/k8s/cluster%2Fprimary");
    expect(executor.children).toEqual([]);
  });
});

describe("findActiveNodePath", () => {
  const forest = [
    buildBranchRoot(
      {
        href: WORKSPACES_HREF,
        labelKey: "common:workspaces",
        activePrefixes: ["/settings/workspaces/", "/settings/workspace/"],
      },
      buildWorkspacesBranch(WORKSPACES),
    ),
    buildBranchRoot(
      { href: "/settings/agents", labelKey: "common:agents" },
      buildAgentsBranch(AGENTS),
    ),
    buildBranchRoot(
      {
        href: "/settings/executors",
        labelKey: "common:executors",
        activePrefixes: ["/settings/executors/", "/settings/executor/"],
      },
      buildExecutorsBranch(EXECUTORS),
    ),
  ];

  it("descends to the deepest node owning the route", () => {
    expect(
      findActiveNodePath(forest, `/settings/workspaces/${WORKSPACE_ID}/integrations/github`),
    ).toEqual([
      settingsMenuRowKey(WORKSPACES_HREF),
      `workspace:${WORKSPACE_ID}`,
      `workspace:${WORKSPACE_ID}:integrations`,
      `workspace:${WORKSPACE_ID}:integrations:github`,
    ]);
  });

  it("stops at the workspace when the route is its own page", () => {
    expect(findActiveNodePath(forest, `/settings/workspaces/${WORKSPACE_ID}`)).toEqual([
      settingsMenuRowKey(WORKSPACES_HREF),
      `workspace:${WORKSPACE_ID}`,
    ]);
  });

  it("reaches a profile through an agent that has no page", () => {
    expect(findActiveNodePath(forest, "/settings/agents/claude-code/profiles/profile-2")).toEqual([
      settingsMenuRowKey("/settings/agents"),
      "agent:claude-code",
      "agent:claude-code:profile:profile-2",
    ]);
  });

  it("reaches a Kubernetes profile through its disclosure-only executor", () => {
    const kubernetesForest = [
      buildBranchRoot(
        {
          href: EXECUTORS_HREF,
          labelKey: "common:executors",
          activePrefixes: ["/settings/executors/", "/settings/executor/"],
        },
        buildExecutorsBranch([
          {
            id: "cluster-primary",
            name: "Kubernetes",
            type: "k8s",
            profiles: [{ id: "profile-primary", name: "Primary" }],
          },
        ]),
      ),
    ];

    expect(findActiveNodePath(kubernetesForest, `${EXECUTORS_HREF}/profile-primary`)).toEqual([
      settingsMenuRowKey(EXECUTORS_HREF),
      "executor:cluster-primary",
      "executor:cluster-primary:profile:profile-primary",
    ]);
  });

  it("stops at the row when the route has no node — the row stays active", () => {
    // The install catalogue is not an agent, and the flat profile spelling has
    // no executor to nest under. Both are the row's own pages.
    expect(findActiveNodePath(forest, "/settings/agents/browse")).toEqual([
      settingsMenuRowKey("/settings/agents"),
    ]);
    expect(findActiveNodePath(forest, "/settings/executors/exec-profile-1")).toEqual([
      settingsMenuRowKey(EXECUTORS_HREF),
    ]);
  });

  it("claims the row's alternate route spellings through activePrefixes", () => {
    // `/settings/executor/<id>` is not under `/settings/executors`, but the
    // Executors row owns it — and so must its branch.
    expect(findActiveNodePath(forest, "/settings/executor/exec-1")).toEqual([
      settingsMenuRowKey(EXECUTORS_HREF),
      "executor:exec-1",
    ]);
  });

  it("returns nothing for a route outside every branch", () => {
    expect(findActiveNodePath(forest, "/settings/preferences/appearance")).toEqual([]);
  });

  it("does not let one workspace claim another's routes by prefix", () => {
    const twoWorkspaces = buildWorkspacesBranch([
      { id: "ws-1", name: "One" },
      { id: "ws-10", name: "Ten" },
    ]);

    expect(findActiveNodePath(twoWorkspaces, "/settings/workspaces/ws-10/secrets")).toEqual([
      "workspace:ws-10",
      "workspace:ws-10:secrets",
    ]);
  });
});

describe("key path helpers", () => {
  const workspaces = buildWorkspacesBranch(WORKSPACES);

  it("names every ancestor needed to reveal a node", () => {
    expect(findNodeKeyPath(workspaces, `workspace:${WORKSPACE_ID}:integrations:jira`)).toEqual([
      `workspace:${WORKSPACE_ID}`,
      `workspace:${WORKSPACE_ID}:integrations`,
      `workspace:${WORKSPACE_ID}:integrations:jira`,
    ]);
  });

  it("names the node and everything under it when collapsing", () => {
    const keys = findSubtreeKeys(workspaces, `workspace:${WORKSPACE_ID}:integrations`);

    expect(keys[0]).toBe(`workspace:${WORKSPACE_ID}:integrations`);
    expect(keys).toContain(`workspace:${WORKSPACE_ID}:integrations:github`);
    expect(keys).not.toContain(`workspace:${WORKSPACE_ID}:secrets`);
  });

  it("returns nothing for a key no node carries — a stale stored branch", () => {
    expect(findNodeKeyPath(workspaces, "workspace:deleted")).toEqual([]);
    expect(findSubtreeKeys(workspaces, "workspace:deleted")).toEqual([]);
  });
});
