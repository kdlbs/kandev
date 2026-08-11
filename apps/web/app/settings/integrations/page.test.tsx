import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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

const { makeEnabledMock } = vi.hoisted(() => ({
  makeEnabledMock: (enabled: boolean) => () => ({ enabled, setEnabled: vi.fn(), loaded: true }),
}));

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

function registerIntegration() {
  pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
    id: "source-control",
    label: "Source Control",
    description: "Connect a source-control provider.",
    icon: "cloud",
    Component: () => null,
  });
}

beforeEach(() => {
  pushNavigationStateSpy.mockClear();
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

  it("renders the hide-disabled-in-nav setting off by default and drafts a toggle without persisting until save", () => {
    renderPage();

    const hideDisabledSwitch = document.getElementById("hide-disabled-integrations-in-nav");
    expect(hideDisabledSwitch).not.toBeNull();
    expect(ariaChecked(hideDisabledSwitch)).toBe(ARIA_CHECKED_FALSE);

    fireEvent.click(hideDisabledSwitch as HTMLElement);

    expect(ariaChecked(hideDisabledSwitch)).toBe(ARIA_CHECKED_TRUE);
    expect(window.localStorage.getItem("kandev:integrations:hideDisabledInNav:v1")).toBeNull();
  });

  it("navigates when the integration label is clicked", () => {
    renderPage();

    fireEvent.click(screen.getByRole("link", { name: "Azure DevOps" }));

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

    const link = screen.getByRole("link", { name: /source control/i });
    expect(link.getAttribute("href")).toBe("/settings/integrations/source-control");
    expect(screen.getByText("Connect a source-control provider.")).not.toBeNull();
  });

  it("uses the workspace-scoped plugin integration path", () => {
    registerIntegration();

    renderPage("workspace one");

    expect(screen.getByRole("link", { name: /source control/i }).getAttribute("href")).toBe(
      "/settings/workspaces/workspace%20one/integrations/source-control",
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
