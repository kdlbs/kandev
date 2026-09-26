import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  INTEGRATION_ENABLED_KEYS,
  type IntegrationSlug,
} from "@/lib/integrations/integration-enabled-keys";
import { makeLocalStorageMock } from "@/hooks/local-storage-mock.test-helpers";

import type { SettingsMenuNode } from "./settings-menu-branches";

const WORKSPACE_ID = "ws-1";
const BITBUCKET_PLUGIN_ID = "plugin-bitbucket";
const BITBUCKET_INTEGRATION_ID = "bitbucket";
const BITBUCKET_REGISTRATION = {
  id: BITBUCKET_INTEGRATION_ID,
  label: "Bitbucket",
  description: "Configure Bitbucket.",
  icon: () => null,
  Component: () => null,
};
const OTHER_WORKSPACE_ID = "ws-2";

const state = {
  workspaces: {
    activeId: WORKSPACE_ID,
    items: [
      { id: WORKSPACE_ID, name: "Main Workspace" },
      { id: OTHER_WORKSPACE_ID, name: "Second Workspace" },
    ],
  },
  settingsAgents: { items: [] as unknown[] },
  executors: { items: [] as unknown[] },
  agentDiscovery: { items: [] as unknown[], loaded: false },
};

// The install-wide "hide disabled integrations from left panel navigation"
// setting, held as hoisted state so a test can flip it without re-mocking. The
// per-integration toggles are NOT mocked: they are per-workspace localStorage
// values, and reading them for real is the only way to prove the branch
// resolves each workspace's own.
const hideDisabled = vi.hoisted(() => ({ value: false }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));
vi.mock("@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav", () => ({
  useHideDisabledIntegrationsInNav: () => ({ hideDisabled: hideDisabled.value }),
}));

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  configurable: true,
});

/** Turn an integration off for one workspace, the way its slider does. */
function disableIn(workspaceId: string, slug: IntegrationSlug): void {
  window.localStorage.setItem(
    `${INTEGRATION_ENABLED_KEYS[slug].storageKey}:${workspaceId}`,
    "false",
  );
}

import { useSettingsMenuBranches } from "./use-settings-menu-branches";

const WORKSPACES_HREF = "/settings/workspaces";

/** The integration slugs the Workspaces branch lists for one workspace. */
function listedIntegrations(workspaceIndex = 0): Array<string | undefined> {
  const { result } = renderHook(() => useSettingsMenuBranches("accordion"));
  const workspace = result.current[WORKSPACES_HREF]?.children?.[workspaceIndex];
  const integrations = (workspace?.children ?? ([] as SettingsMenuNode[])).find((node) =>
    node.href?.endsWith("/integrations"),
  );
  return (integrations?.children ?? []).map((node) => node.integrationSlug);
}

describe("useSettingsMenuBranches integration visibility", () => {
  beforeEach(() => {
    hideDisabled.value = false;
    localStorageMock.clear();
  });

  afterEach(() => {
    pluginRegistry.unregisterPlugin(BITBUCKET_PLUGIN_ID);
    // Restore the file-scoped mock state so no test inherits the previous one's
    // toggles.
    hideDisabled.value = false;
    localStorageMock.clear();
  });

  it("lists every integration while the hide-disabled setting is off, disabled ones included", () => {
    disableIn(WORKSPACE_ID, "github");
    disableIn(WORKSPACE_ID, "sentry");

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
    disableIn(WORKSPACE_ID, "github");
    disableIn(WORKSPACE_ID, "sentry");
    hideDisabled.value = true;

    expect(listedIntegrations()).toEqual(["azure-devops", "gitlab", "jira", "linear"]);
  });

  it("hides an integration only in the workspace that disabled it", () => {
    // The toggle is per workspace: turning GitHub off in ws-1 must leave ws-2's
    // branch untouched.
    disableIn(WORKSPACE_ID, "github");
    hideDisabled.value = true;

    expect(listedIntegrations(0)).not.toContain("github");
    expect(listedIntegrations(1)).toContain("github");
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

    expect(listedIntegrations()).toContain("github");
  });

  it("adds and revokes registered provider integrations reactively", () => {
    pluginRegistry
      .forPlugin(BITBUCKET_PLUGIN_ID)
      .registerIntegrationSettings(BITBUCKET_REGISTRATION);
    const { result } = renderHook(() => useSettingsMenuBranches("accordion"));
    const integrations = result.current[WORKSPACES_HREF]?.children?.[0].children?.find((node) =>
      node.href?.endsWith("/integrations"),
    );

    expect(
      integrations?.children?.some((node) => node.href?.endsWith(`/${BITBUCKET_INTEGRATION_ID}`)),
    ).toBe(true);

    act(() => pluginRegistry.unregisterPlugin(BITBUCKET_PLUGIN_ID));

    const refreshed = result.current[WORKSPACES_HREF]?.children?.[0].children?.find((node) =>
      node.href?.endsWith("/integrations"),
    );
    expect(
      refreshed?.children?.some((node) => node.href?.endsWith(`/${BITBUCKET_INTEGRATION_ID}`)),
    ).toBe(false);
  });

  it("hides an explicitly disabled plugin integration while keeping unknown state visible", () => {
    hideDisabled.value = true;
    pluginRegistry
      .forPlugin(BITBUCKET_PLUGIN_ID)
      .registerIntegrationSettings(BITBUCKET_REGISTRATION);

    const setIntegrationEnabled = pluginRegistry.setIntegrationEnabled.bind(
      pluginRegistry,
    ) as unknown as (
      pluginId: string,
      integrationId: string,
      workspaceId: string,
      enabled: boolean,
    ) => void;
    setIntegrationEnabled(BITBUCKET_PLUGIN_ID, BITBUCKET_INTEGRATION_ID, WORKSPACE_ID, false);

    expect(listedIntegrations()).not.toContain(BITBUCKET_INTEGRATION_ID);

    pluginRegistry.unregisterPlugin(BITBUCKET_PLUGIN_ID);
    pluginRegistry
      .forPlugin(BITBUCKET_PLUGIN_ID)
      .registerIntegrationSettings(BITBUCKET_REGISTRATION);

    expect(listedIntegrations()).toContain(BITBUCKET_INTEGRATION_ID);
  });

  it("leaves the flat menu without branches at all, whatever the toggles say", () => {
    hideDisabled.value = true;
    disableIn(WORKSPACE_ID, "github");

    const { result } = renderHook(() => useSettingsMenuBranches("flat"));

    expect(result.current).toEqual({});
  });
});
