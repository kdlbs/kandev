import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";

const MAIN_WORKSPACE_ID = "ws-1";
const ARCHIVE_WORKSPACE_ID = "ws-10";
const MAIN_WORKSPACE_NAME = "Main Workspace";
const ARCHIVE_WORKSPACE_NAME = "Archive Workspace";
const VOICE_MODE_LABEL = "Voice Mode";

const state = {
  workspaces: {
    activeId: MAIN_WORKSPACE_ID,
    items: [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }],
  },
  setActiveWorkspace: vi.fn(),
  settingsAgents: {
    items: [],
  },
  executors: {
    items: [],
  },
  features: {
    office: false,
    auth: false,
  },
  auth: {
    mode: "disabled" as const,
    authenticated: true,
    user: null,
  },
};

const integrationAvailability = vi.hoisted(() => ({
  azureDevOps: true,
  github: false,
  gitlab: false,
  jira: false,
  linear: false,
  sentry: false,
}));

// Per-integration enable/disable state plus the "hide disabled integrations
// from left panel navigation" setting, shared by the mocks below so each test
// can flip them independently (same shape the settings-tree-render suite uses
// for `integrationAvailability`).
const integrationEnabled = vi.hoisted(() => ({
  "azure-devops": true,
  github: true,
  gitlab: true,
  jira: true,
  linear: true,
  sentry: true,
}));
const hideDisabled = vi.hoisted(() => ({ value: false }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => undefined,
}));

vi.mock("@/hooks/domains/azure-devops/use-azure-devops-availability", () => ({
  useAzureDevOpsAvailable: () => integrationAvailability.azureDevOps,
}));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({
    status: integrationAvailability.github ? { authenticated: true } : null,
    loading: false,
  }),
}));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: () => integrationAvailability.gitlab,
}));
vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAuthed: () => integrationAvailability.jira,
}));
vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAuthed: () => integrationAvailability.linear,
}));
vi.mock("@/hooks/domains/sentry/use-sentry-availability", () => ({
  useSentryAvailable: () => integrationAvailability.sentry,
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-enabled", () => ({
  useAzureDevOpsEnabled: () => ({
    enabled: integrationEnabled["azure-devops"],
    setEnabled: vi.fn(),
  }),
}));
vi.mock("@/hooks/domains/github/use-github-enabled", () => ({
  useGitHubEnabled: () => ({ enabled: integrationEnabled.github, setEnabled: vi.fn() }),
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-enabled", () => ({
  useGitLabEnabled: () => ({ enabled: integrationEnabled.gitlab, setEnabled: vi.fn() }),
}));
vi.mock("@/hooks/domains/jira/use-jira-enabled", () => ({
  useJiraEnabled: () => ({ enabled: integrationEnabled.jira, setEnabled: vi.fn() }),
}));
vi.mock("@/hooks/domains/linear/use-linear-enabled", () => ({
  useLinearEnabled: () => ({ enabled: integrationEnabled.linear, setEnabled: vi.fn() }),
}));
vi.mock("@/hooks/domains/sentry/use-sentry-enabled", () => ({
  useSentryEnabled: () => ({ enabled: integrationEnabled.sentry, setEnabled: vi.fn() }),
}));
vi.mock("@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav", () => ({
  useHideDisabledIntegrationsInNav: () => ({
    hideDisabled: hideDisabled.value,
    setHideDisabled: vi.fn(),
  }),
}));
vi.mock("@kandev/ui/collapsible", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  const CollapsibleContext = React.createContext(false);
  return {
    Collapsible: ({ open, children }: { open?: boolean; children: ReactNode }) =>
      React.createElement(CollapsibleContext.Provider, { value: Boolean(open) }, children),
    CollapsibleContent: ({ children, className }: { children: ReactNode; className?: string }) => {
      const open = React.useContext(CollapsibleContext);
      return open ? React.createElement("div", { className }, children) : null;
    },
  };
});

import { SettingsTree } from "./settings-tree";
import { AgentsGroup } from "./agents-group";
import { GeneralGroup } from "./general-group";
import { SystemGroup } from "./system-group";
import { WorkspacesGroup } from "./workspaces-group";

describe("SettingsTree rendering", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
    state.setActiveWorkspace.mockClear();
    state.settingsAgents.items = [];
    state.executors.items = [];
    integrationAvailability.azureDevOps = true;
    integrationAvailability.github = false;
    integrationAvailability.gitlab = false;
    integrationAvailability.jira = false;
    integrationAvailability.linear = false;
    integrationAvailability.sentry = false;
  });

  afterEach(() => cleanup());

  it("renders workspace repository and workflow links when Workspaces is open", () => {
    render(<WorkspacesGroup pathname="/settings/workspace" expanded />);

    expect(screen.getByRole("link", { name: "Repositories" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/repositories",
    );
    expect(screen.getByRole("link", { name: "Workflows" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/workflows",
    );
    expect(screen.getByRole("link", { name: "Automations" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/automations",
    );
    expect(screen.getByRole("link", { name: "Integrations" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/integrations",
    );
  });

  it("opens the active workspace by default when the settings tree opens", () => {
    state.workspaces.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    render(<SettingsTree pathname="/settings" />);

    expect(screen.getByRole("link", { name: `${MAIN_WORKSPACE_NAME} Active` })).toBeTruthy();
    expect(screen.getByRole("link", { name: ARCHIVE_WORKSPACE_NAME })).toBeTruthy();
    expect(screen.queryByText("[active]")).toBeNull();
    expect(screen.getByRole("link", { name: "Repositories" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/repositories",
    );
    expect(screen.getByRole("link", { name: "Automations" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/automations",
    );
  });

  it("uses an accordion for workspace subsections", () => {
    state.workspaces.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    render(<WorkspacesGroup pathname="/settings" expanded />);

    expect(screen.getByRole("link", { name: "Repositories" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/repositories",
    );

    fireEvent.click(screen.getByRole("button", { name: "Expand Archive Workspace" }));

    expect(screen.getByRole("button", { name: "Expand Main Workspace" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Repositories" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-10/repositories",
    );
  });

  it("only opens the routed workspace subsection on workspace detail routes", () => {
    state.workspaces.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    const { rerender } = render(<WorkspacesGroup pathname="/settings/workspace" expanded />);

    expect(screen.getAllByRole("link", { name: "Repositories" })).toHaveLength(1);

    rerender(<WorkspacesGroup pathname="/settings/workspace/ws-10/repositories" expanded />);

    expect(
      screen.getByRole("link", { name: `${MAIN_WORKSPACE_NAME} Active` }).getAttribute("href"),
    ).toBe("/settings/workspace/ws-1");
    const repositoryLinks = screen.getAllByRole("link", { name: "Repositories" });
    const workflowLinks = screen.getAllByRole("link", { name: "Workflows" });

    expect(repositoryLinks).toHaveLength(1);
    expect(workflowLinks).toHaveLength(1);
    expect(repositoryLinks[0].getAttribute("href")).toBe("/settings/workspace/ws-10/repositories");
    expect(workflowLinks[0].getAttribute("href")).toBe("/settings/workspace/ws-10/workflows");
    expect(screen.getByRole("link", { name: ARCHIVE_WORKSPACE_NAME }).getAttribute("href")).toBe(
      "/settings/workspace/ws-10",
    );
  });

  it("opens workspace integrations when a workspace integration route is active", () => {
    state.workspaces.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    render(<WorkspacesGroup pathname="/settings/workspace/ws-10/integrations/github" expanded />);

    expect(screen.getByRole("link", { name: "GitHub" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-10/integrations/github",
    );
    expect(screen.getByRole("button", { name: "Expand Main Workspace" })).toBeTruthy();
  });
});

describe("Workspace settings order", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
  });

  afterEach(() => cleanup());

  it("places workspace secrets below automations", () => {
    render(<WorkspacesGroup pathname="/settings/workspace" expanded />);

    const hrefs = screen.getAllByRole("link").map((link) => link.getAttribute("href"));
    const automationsIndex = hrefs.indexOf("/settings/workspace/ws-1/automations");
    const secretsIndex = hrefs.indexOf("/settings/workspace/ws-1/secrets");

    expect(automationsIndex).toBeGreaterThanOrEqual(0);
    expect(secretsIndex).toBeGreaterThan(automationsIndex);
    expect(screen.getByRole("link", { name: "Secrets" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Workspace Secrets" })).toBeNull();
  });
});

describe("SettingsTree integration status", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
    integrationAvailability.azureDevOps = true;
    integrationAvailability.github = false;
  });

  afterEach(() => {
    pluginRegistry.unregisterPlugin("plugin-bitbucket");
    cleanup();
  });

  it("labels configured integrations as enabled", () => {
    render(
      <WorkspacesGroup pathname="/settings/workspace/ws-1/integrations/azure-devops" expanded />,
    );

    expect(screen.getByRole("link", { name: "Azure DevOps Enabled" })).toBeTruthy();
    expect(screen.getByTestId("azure-devops-icon")).toBeTruthy();
    expect(screen.getByRole("link", { name: "GitHub" })).toBeTruthy();
  });

  it("renders plugin integration settings in the native workspace navigation", () => {
    pluginRegistry.forPlugin("plugin-bitbucket").registerIntegrationSettings({
      id: "bitbucket",
      label: "Bitbucket",
      description: "Bitbucket Cloud and Data Center.",
      icon: "bitbucket",
      Component: () => null,
    });

    render(<WorkspacesGroup pathname="/settings/workspace/ws-1/integrations/bitbucket" expanded />);

    expect(screen.getByRole("link", { name: "Bitbucket" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/integrations/bitbucket",
    );
  });
});

describe("SettingsTree hide disabled integrations", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
    integrationAvailability.azureDevOps = true;
    integrationAvailability.github = false;
    integrationAvailability.gitlab = false;
    integrationAvailability.jira = false;
    integrationAvailability.linear = false;
    integrationAvailability.sentry = false;
    integrationEnabled["azure-devops"] = true;
    integrationEnabled.github = true;
    integrationEnabled.gitlab = true;
    integrationEnabled.jira = true;
    integrationEnabled.linear = true;
    integrationEnabled.sentry = true;
    hideDisabled.value = false;
  });

  afterEach(() => {
    cleanup();
    // Restore the file-scoped mock state so later suites in this file never
    // inherit the previous test's hideDisabled/enabled flags.
    hideDisabled.value = false;
    integrationEnabled["azure-devops"] = true;
    integrationEnabled.github = true;
    integrationEnabled.gitlab = true;
    integrationEnabled.jira = true;
    integrationEnabled.linear = true;
    integrationEnabled.sentry = true;
  });

  it("keeps every integration listed when the hide-disabled setting is off (default), even disabled ones", () => {
    integrationEnabled.github = false;

    render(
      <WorkspacesGroup pathname="/settings/workspace/ws-1/integrations/azure-devops" expanded />,
    );

    for (const name of [/(Azure DevOps)/, /GitHub/, /GitLab/, /Jira/, /Linear/, /Sentry/]) {
      expect(screen.getByRole("link", { name })).toBeTruthy();
    }
  });

  it("hides a disabled integration from the workspace integrations list when the setting is on", () => {
    integrationEnabled.github = false;
    hideDisabled.value = true;

    render(
      <WorkspacesGroup pathname="/settings/workspace/ws-1/integrations/azure-devops" expanded />,
    );

    expect(screen.queryByRole("link", { name: "GitHub" })).toBeNull();
    expect(screen.getByRole("link", { name: /Azure DevOps/ })).toBeTruthy();
  });

  it("keeps every integration listed when the setting is on but none are disabled", () => {
    hideDisabled.value = true;

    render(
      <WorkspacesGroup pathname="/settings/workspace/ws-1/integrations/azure-devops" expanded />,
    );

    for (const name of [/(Azure DevOps)/, /GitHub/, /GitLab/, /Jira/, /Linear/, /Sentry/]) {
      expect(screen.getByRole("link", { name })).toBeTruthy();
    }
  });

  it("keeps an enabled-but-unconfigured integration listed when the setting is on", () => {
    // GitHub is enabled (toggle on) but has no credentials — only *disabled*
    // integrations are hidden by the setting, so it must stay reachable.
    hideDisabled.value = true;

    render(
      <WorkspacesGroup pathname="/settings/workspace/ws-1/integrations/azure-devops" expanded />,
    );

    expect(screen.getByRole("link", { name: "GitHub" })).toBeTruthy();
  });
});

describe("SettingsTree agents group", () => {
  beforeEach(() => {
    state.settingsAgents.items = [
      {
        id: "agent-1",
        name: "mock-agent",
        profiles: [
          { id: "p-on", name: "default", agentDisplayName: "Mock", enabled: true },
          { id: "p-off", name: "alt", agentDisplayName: "Mock", enabled: false },
          { id: "p-legacy", name: "legacy", agentDisplayName: "Mock" },
        ],
      },
    ] as unknown as typeof state.settingsAgents.items;
  });

  afterEach(() => {
    cleanup();
    state.settingsAgents.items = [];
  });

  it("labels disabled profiles with a badge and leaves others unlabeled", () => {
    render(<AgentsGroup pathname="/settings/agents" expanded />);

    expect(screen.getByRole("link", { name: "Mock • default" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Mock • alt Disabled" })).toBeTruthy();
    // Legacy payloads without the flag are treated as enabled — no badge.
    expect(screen.getByRole("link", { name: "Mock • legacy" })).toBeTruthy();
  });
});

describe("SettingsTree standalone leaves", () => {
  afterEach(cleanup);

  it("keeps Voice Mode in the settings tree as a standalone active leaf", () => {
    render(<SettingsTree pathname="/settings" />);

    expect(screen.getByRole("link", { name: VOICE_MODE_LABEL }).getAttribute("href")).toBe(
      "/settings/voice-mode",
    );

    cleanup();

    render(<SettingsTree pathname="/settings/voice-mode" />);

    expect(screen.getByRole("link", { name: VOICE_MODE_LABEL }).className).toContain(
      "before:bg-primary",
    );
    expect(screen.queryByRole("link", { name: "Appearance" })).toBeNull();
  });

  it("puts Plugins immediately before System", () => {
    render(<SettingsTree pathname="/settings" />);

    expect(
      screen
        .getAllByRole("link")
        .slice(-2)
        .map((link) => link.textContent),
    ).toEqual(["Plugins", "System"]);
  });
});

describe("SettingsTree search", () => {
  afterEach(cleanup);

  it("preserves the normal tree until a query filters it to grouped hits", () => {
    render(<SettingsTree pathname="/settings" />);

    const search = screen.getByRole("searchbox", { name: "Search settings" });
    expect(screen.getByRole("link", { name: VOICE_MODE_LABEL })).toBeTruthy();

    fireEvent.change(search, { target: { value: "font size" } });

    const result = screen.getByRole("link", { name: /Terminal Font Size/ });
    expect(result.getAttribute("href")).toBe(
      "/settings/general/terminal#setting-terminal-font-size",
    );
    expect(result.textContent).toContain("General");
    expect(result.textContent).toContain("Terminal");
    expect(screen.queryByRole("link", { name: VOICE_MODE_LABEL })).toBeNull();
  });

  it("clears a query with Escape and restores the normal tree", () => {
    render(<SettingsTree pathname="/settings" />);
    const search = screen.getByRole("searchbox", { name: "Search settings" });

    fireEvent.change(search, { target: { value: "font size" } });
    fireEvent.keyDown(search, { key: "Escape" });

    expect((search as HTMLInputElement).value).toBe("");
    expect(screen.getByRole("link", { name: VOICE_MODE_LABEL })).toBeTruthy();
  });

  it("announces an empty result without rendering the normal tree", () => {
    render(<SettingsTree pathname="/settings" />);

    fireEvent.change(screen.getByRole("searchbox", { name: "Search settings" }), {
      target: { value: "definitely missing" },
    });

    expect(screen.getByText("No matching settings")).toBeTruthy();
    expect(screen.queryByRole("link", { name: VOICE_MODE_LABEL })).toBeNull();
  });
});

describe("Message Queue settings navigation", () => {
  afterEach(cleanup);

  it("exposes Message Queue under General in the shared desktop and mobile settings tree", () => {
    render(<GeneralGroup pathname="/settings/general/message-queue" expanded />);

    const link = screen.getByRole("link", { name: "Message Queue" });
    expect(link.getAttribute("href")).toBe("/settings/general/message-queue");
    expect(link.className).toContain("before:bg-primary");
  });

  it("does not expose Message Queue under System", () => {
    render(<SystemGroup pathname="/settings/system/status" expanded />);

    expect(screen.queryByRole("link", { name: "Message Queue" })).toBeNull();
  });
});

describe("WorkspacesGroup active workspace presentation", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
    ];
    state.setActiveWorkspace.mockClear();
  });

  afterEach(() => cleanup());

  it("keeps the active workspace first even when the API returns it later", () => {
    render(<WorkspacesGroup pathname="/settings" expanded />);

    const workspaceLinks = getWorkspaceRootLinks();

    expect(workspaceLinks.map((link) => link.textContent)).toEqual([
      `${MAIN_WORKSPACE_NAME}Active`,
      ARCHIVE_WORKSPACE_NAME,
    ]);
  });

  it("expands another workspace without changing the active workspace", () => {
    render(<WorkspacesGroup pathname="/settings" expanded />);

    fireEvent.click(screen.getByRole("button", { name: "Expand Archive Workspace" }));

    expect(state.setActiveWorkspace).not.toHaveBeenCalled();
    expect(getWorkspaceRootLinks()[0].textContent).toBe(`${MAIN_WORKSPACE_NAME}Active`);
    expect(screen.getByRole("link", { name: `${MAIN_WORKSPACE_NAME} Active` })).toBeTruthy();
  });
});

describe("WorkspacesGroup integration route sync", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];
  });

  afterEach(() => cleanup());

  it("opens workspace integrations after navigating into an integration route", async () => {
    const { rerender } = render(
      <WorkspacesGroup pathname="/settings/workspace/ws-10/repositories" expanded />,
    );

    expect(screen.queryByRole("link", { name: "GitHub" })).toBeNull();

    rerender(<WorkspacesGroup pathname="/settings/workspace/ws-10/integrations/github" expanded />);

    expect((await screen.findByRole("link", { name: "GitHub" })).getAttribute("href")).toBe(
      "/settings/workspace/ws-10/integrations/github",
    );
  });
});

function getWorkspaceRootLinks(): HTMLAnchorElement[] {
  return screen.getAllByRole("link").filter((link): link is HTMLAnchorElement => {
    const href = link.getAttribute("href");
    return Boolean(href?.match(/^\/settings\/workspace\/[^/]+$/));
  });
}
