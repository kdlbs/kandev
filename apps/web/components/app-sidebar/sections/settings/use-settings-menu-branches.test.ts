import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";

import type { SettingsMenuNode } from "./settings-menu-branches";

const WORKSPACE_ID = "ws-1";

const state = {
  workspaces: {
    activeId: WORKSPACE_ID,
    items: [{ id: WORKSPACE_ID, name: "Main Workspace" }],
  },
  settingsAgents: { items: [] as unknown[] },
  executors: { items: [] as unknown[] },
  agentDiscovery: { items: [] as unknown[], loaded: false },
};

// Per-integration enable/disable toggles plus the install-wide "hide disabled
// integrations from left panel navigation" setting, held as hoisted state so a
// test can flip either without re-mocking.
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
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-enabled", () => ({
  useAzureDevOpsEnabled: () => ({ enabled: integrationEnabled["azure-devops"] }),
}));
vi.mock("@/hooks/domains/github/use-github-enabled", () => ({
  useGitHubEnabled: () => ({ enabled: integrationEnabled.github }),
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-enabled", () => ({
  useGitLabEnabled: () => ({ enabled: integrationEnabled.gitlab }),
}));
vi.mock("@/hooks/domains/jira/use-jira-enabled", () => ({
  useJiraEnabled: () => ({ enabled: integrationEnabled.jira }),
}));
vi.mock("@/hooks/domains/linear/use-linear-enabled", () => ({
  useLinearEnabled: () => ({ enabled: integrationEnabled.linear }),
}));
vi.mock("@/hooks/domains/sentry/use-sentry-enabled", () => ({
  useSentryEnabled: () => ({ enabled: integrationEnabled.sentry }),
}));
vi.mock("@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav", () => ({
  useHideDisabledIntegrationsInNav: () => ({ hideDisabled: hideDisabled.value }),
}));

import { useSettingsMenuBranches } from "./use-settings-menu-branches";

const WORKSPACES_HREF = "/settings/workspaces";

/** The integration slugs the Workspaces branch currently lists. */
function listedIntegrations(): Array<string | undefined> {
  const { result } = renderHook(() => useSettingsMenuBranches("accordion"));
  const workspace = result.current[WORKSPACES_HREF]?.children?.[0];
  const integrations = (workspace?.children ?? ([] as SettingsMenuNode[])).find((node) =>
    node.href?.endsWith("/integrations"),
  );
  return (integrations?.children ?? []).map((node) => node.integrationSlug);
}

describe("useSettingsMenuBranches integration visibility", () => {
  beforeEach(() => {
    hideDisabled.value = false;
    for (const slug of Object.keys(integrationEnabled) as Array<keyof typeof integrationEnabled>) {
      integrationEnabled[slug] = true;
    }
  });

  afterEach(() => {
    pluginRegistry.unregisterPlugin("plugin-bitbucket");
    // Restore the file-scoped mock state so no test inherits the previous one's
    // toggles.
    hideDisabled.value = false;
    for (const slug of Object.keys(integrationEnabled) as Array<keyof typeof integrationEnabled>) {
      integrationEnabled[slug] = true;
    }
  });

  it("lists every integration while the hide-disabled setting is off, disabled ones included", () => {
    integrationEnabled.github = false;
    integrationEnabled.sentry = false;

    expect(listedIntegrations()).toEqual([
      "azure-devops",
      "github",
      "gitlab",
      "jira",
      "linear",
      "sentry",
    ]);
  });

  it("hides the disabled integrations once the setting is on", () => {
    integrationEnabled.github = false;
    integrationEnabled.sentry = false;
    hideDisabled.value = true;

    expect(listedIntegrations()).toEqual(["azure-devops", "gitlab", "jira", "linear"]);
  });

  it("keeps every integration when the setting is on but none are disabled", () => {
    hideDisabled.value = true;

    expect(listedIntegrations()).toEqual([
      "azure-devops",
      "github",
      "gitlab",
      "jira",
      "linear",
      "sentry",
    ]);
  });

  it("keeps an enabled integration listed even though nothing here knows it is connected", () => {
    // The toggle is the only gate: this hook never probes credentials, so an
    // enabled-but-unconfigured integration stays reachable and only its badge
    // (resolved at render) reflects the connection.
    hideDisabled.value = true;
    integrationEnabled.github = true;

    expect(listedIntegrations()).toContain("github");
  });

  it("adds and revokes registered provider integrations reactively", () => {
    pluginRegistry.forPlugin("plugin-bitbucket").registerIntegrationSettings({
      id: "bitbucket",
      label: "Bitbucket",
      description: "Configure Bitbucket.",
      icon: () => null,
      Component: () => null,
    });
    const { result } = renderHook(() => useSettingsMenuBranches("accordion"));
    const integrations = result.current[WORKSPACES_HREF]?.children?.[0].children?.find((node) =>
      node.href?.endsWith("/integrations"),
    );

    expect(integrations?.children?.some((node) => node.href?.endsWith("/bitbucket"))).toBe(true);

    act(() => pluginRegistry.unregisterPlugin("plugin-bitbucket"));

    const refreshed = result.current[WORKSPACES_HREF]?.children?.[0].children?.find((node) =>
      node.href?.endsWith("/integrations"),
    );
    expect(refreshed?.children?.some((node) => node.href?.endsWith("/bitbucket"))).toBe(false);
  });

  it("leaves the flat menu without branches at all, whatever the toggles say", () => {
    hideDisabled.value = true;
    integrationEnabled.github = false;

    const { result } = renderHook(() => useSettingsMenuBranches("flat"));

    expect(result.current).toEqual({});
  });
});
