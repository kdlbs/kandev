import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Component, IntegrationSettingsActionProps } from "@kandev/plugin-sdk";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { IntegrationsIndexPage } from "@/components/integrations/integrations-index-page";
import { pluginRegistry } from "@/lib/plugins/registry";

const { pushNavigationStateSpy } = vi.hoisted(() => ({
  pushNavigationStateSpy: vi.fn(),
}));

vi.mock("@/lib/routing/navigation-guard", () => ({
  pushNavigationState: (...args: unknown[]) => pushNavigationStateSpy(...args),
  setNavigationBlocker: () => () => {},
}));

// The mocks record the workspace they were called with: the page's only job
// here is to hand each slider the workspace whose toggles it is editing.
const { makeEnabledMock, enabledCalls } = vi.hoisted(() => {
  const enabledCalls: Array<string | null | undefined> = [];
  return {
    enabledCalls,
    makeEnabledMock: (enabled: boolean) => (workspaceId?: string | null) => {
      enabledCalls.push(workspaceId);
      return { enabled, setEnabled: vi.fn(), loaded: true };
    },
  };
});

vi.mock("@/hooks/domains/azure-devops/use-azure-devops-enabled", () => ({
  useAzureDevOpsEnabled: makeEnabledMock(true),
}));
vi.mock("@/hooks/domains/github/use-github-enabled", () => ({
  useGitHubEnabled: makeEnabledMock(false),
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-enabled", () => ({
  useGitLabEnabled: makeEnabledMock(true),
}));
vi.mock("@/hooks/domains/jira/use-jira-enabled", () => ({
  useJiraEnabled: makeEnabledMock(true),
}));
vi.mock("@/hooks/domains/linear/use-linear-enabled", () => ({
  useLinearEnabled: makeEnabledMock(true),
}));
vi.mock("@/hooks/domains/sentry/use-sentry-enabled", () => ({
  useSentryEnabled: makeEnabledMock(true),
}));

const PLUGIN_ID = "plugin-source-control";
const SOURCE_CONTROL_ID = "source-control";
const SOURCE_CONTROL_DESCRIPTION = "Connect a source-control provider.";

function registerIntegration() {
  pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
    id: SOURCE_CONTROL_ID,
    label: "Source Control",
    description: SOURCE_CONTROL_DESCRIPTION,
    icon: "cloud",
    Component: () => null,
  });
}

function registerIntegrationWithAction(
  pluginId = PLUGIN_ID,
  id = SOURCE_CONTROL_ID,
  action?: Component<IntegrationSettingsActionProps>,
) {
  const labels: Record<string, string> = {
    "source-control": "Source Control",
    "throwing-provider": "Throwing Provider",
  };
  pluginRegistry.forPlugin(pluginId).registerIntegrationSettings({
    id,
    label: labels[id] ?? "Healthy Provider",
    description: SOURCE_CONTROL_DESCRIPTION,
    icon: "cloud",
    Component: () => null,
    action,
  });
}

beforeEach(() => {
  pushNavigationStateSpy.mockClear();
  enabledCalls.length = 0;
  window.localStorage.removeItem("kandev:integrations:hideDisabledInNav:v1");
});

afterEach(() => {
  act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));
  cleanup();
});

function renderPage(workspaceId?: string) {
  return render(
    <SettingsSaveProvider>
      <IntegrationsIndexPage workspaceId={workspaceId} />
    </SettingsSaveProvider>,
  );
}

const ARIA_CHECKED_TRUE = "true";
const ARIA_CHECKED_FALSE = "false";

function ariaChecked(element: Element | null) {
  return element?.getAttribute("aria-checked");
}

describe("IntegrationsIndexPage", () => {
  it("renders one enable/disable slider per native integration", () => {
    renderPage();

    const switches = screen.getAllByRole("switch");
    expect(switches).toHaveLength(7);

    expect(ariaChecked(document.getElementById("azure-devops-enabled"))).toBe(ARIA_CHECKED_TRUE);
    expect(ariaChecked(document.getElementById("github-enabled"))).toBe(ARIA_CHECKED_FALSE);
  });

  it("gives every slider the workspace whose toggles it edits", () => {
    // The toggles are per workspace, so a slider that never learns its
    // workspace would silently edit (and read) the wrong one.
    renderPage("ws-42");

    expect(enabledCalls).toHaveLength(6);
    expect(new Set(enabledCalls)).toEqual(new Set(["ws-42"]));
  });

  it("renders the hide-disabled-in-nav setting off by default and drafts a toggle without persisting until save", () => {
    renderPage();

    const hideDisabledSwitch = document.getElementById("hide-disabled-integrations-in-nav");
    expect(hideDisabledSwitch).not.toBeNull();
    expect(ariaChecked(hideDisabledSwitch)).toBe(ARIA_CHECKED_FALSE);

    fireEvent.click(hideDisabledSwitch as HTMLElement);

    expect(ariaChecked(hideDisabledSwitch)).toBe(ARIA_CHECKED_TRUE);
    expect(window.localStorage.getItem("kandev:integrations:hideDisabledInNav:v1")).toBeNull();
  });

  it("exposes one full-card navigation target for a native integration", () => {
    renderPage();

    const card = screen.getByTestId("integration-card-azure-devops");
    const links = within(card).getAllByRole("link");

    expect(links).toHaveLength(1);
    expect(links[0].getAttribute("aria-label")).toBe("Azure DevOps");
    expect(links[0].getAttribute("href")).toBe("/settings/integrations/azure-devops");

    fireEvent.click(links[0]);

    expect(pushNavigationStateSpy).toHaveBeenCalledWith(
      {},
      "",
      "/settings/integrations/azure-devops",
      expect.any(Function),
    );
  });

  it("keeps workspace-scoped integration links on the workspace route", () => {
    renderPage("ws-1");

    // Plural: the restructure serves workspace settings from
    // /settings/workspaces/<id>, with the singular path kept as a redirect.
    expect(screen.getByRole("link", { name: "Azure DevOps" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/integrations/azure-devops",
    );
  });

  it("does not navigate when a slider is toggled", () => {
    renderPage();

    const githubSwitch = document.getElementById("github-enabled");
    expect(githubSwitch).not.toBeNull();
    fireEvent.click(githubSwitch as HTMLElement);

    expect(pushNavigationStateSpy).not.toHaveBeenCalled();
    expect(ariaChecked(githubSwitch)).toBe(ARIA_CHECKED_TRUE);
  });
});

describe("IntegrationsIndexPage plugin contributions", () => {
  it("renders a plugin contribution beside native integrations", () => {
    registerIntegration();

    renderPage();

    const card = screen.getByTestId(`integration-card-${PLUGIN_ID}-${SOURCE_CONTROL_ID}`);
    const links = within(card).getAllByRole("link");

    expect(links).toHaveLength(1);
    const link = links[0];
    expect(link.getAttribute("aria-label")).toBe("Source Control");
    expect(link.getAttribute("href")).toBe(`/settings/integrations/${SOURCE_CONTROL_ID}`);
    expect(screen.getByText(SOURCE_CONTROL_DESCRIPTION)).not.toBeNull();
  });

  it("renders the plugin action with the index surface and routed workspace", () => {
    const actionCalls: Array<{ workspaceId?: string; surface: "detail" | "index" }> = [];
    const Action = ({ workspaceId, surface }: IntegrationSettingsActionProps) => {
      actionCalls.push({ workspaceId, surface });
      return <span data-testid="plugin-index-action">Action</span>;
    };
    registerIntegrationWithAction(PLUGIN_ID, SOURCE_CONTROL_ID, Action);

    renderPage("workspace-42");

    expect(screen.getByTestId("plugin-index-action")).not.toBeNull();
    expect(actionCalls).toEqual([{ workspaceId: "workspace-42", surface: "index" }]);
  });

  it("passes an undefined workspace to a plugin action on the global index route", () => {
    const actionCalls: Array<{ workspaceId?: string; surface: "detail" | "index" }> = [];
    const Action = ({ workspaceId, surface }: IntegrationSettingsActionProps) => {
      actionCalls.push({ workspaceId, surface });
      return null;
    };
    registerIntegrationWithAction(PLUGIN_ID, SOURCE_CONTROL_ID, Action);

    renderPage();

    expect(actionCalls).toEqual([{ workspaceId: undefined, surface: "index" }]);
  });

  it("contains a throwing plugin action without removing other integration cards", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const ThrowingAction = () => {
      throw new Error("plugin action failed");
    };
    const HealthyAction = () => <span data-testid="healthy-plugin-action">Healthy</span>;
    registerIntegrationWithAction(PLUGIN_ID, "throwing-provider", ThrowingAction);
    registerIntegrationWithAction("plugin-healthy", "healthy-provider", HealthyAction);

    renderPage();

    expect(screen.getByRole("link", { name: /throwing provider/i })).not.toBeNull();
    expect(screen.getByRole("link", { name: /healthy provider/i })).not.toBeNull();
    expect(screen.getByTestId("healthy-plugin-action")).not.toBeNull();
  });

  it("uses the workspace-scoped plugin integration path", () => {
    registerIntegration();

    renderPage("workspace one");

    expect(screen.getByRole("link", { name: /source control/i }).getAttribute("href")).toBe(
      `/settings/workspaces/workspace%20one/integrations/${SOURCE_CONTROL_ID}`,
    );
  });

  it("removes the contribution reactively when its plugin unloads", () => {
    registerIntegration();
    renderPage();
    expect(screen.getByRole("link", { name: /source control/i })).not.toBeNull();

    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

    expect(screen.queryByRole("link", { name: /source control/i })).toBeNull();
  });
});
