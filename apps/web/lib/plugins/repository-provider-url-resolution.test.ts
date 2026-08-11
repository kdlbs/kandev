import { afterEach, describe, expect, it } from "vitest";
import { pluginRegistry } from "./registry";
import type { RepositoryInspection, RepositoryProviderRegistration } from "./types";
import { inspectRegisteredRepositoryURL } from "./repository-provider-url-resolution";

const PLUGINS = ["url-owner-a", "url-owner-b"];
const URL = "https://code.example.test/projects/TEAM/repos/app";
const WORKSPACE_ID = "workspace-1";
const PROVIDER_B_ID = "provider-b";
const PROVIDER_A_ID = "provider-a";

function provider(
  id: string,
  inspectURL: RepositoryProviderRegistration["inspectURL"],
): RepositoryProviderRegistration {
  return {
    id,
    label: id,
    listRepositories: async () => [],
    matchesURL: () => true,
    listBranches: async () => [],
    inspectURL,
  };
}

function inspection(providerId: string): RepositoryInspection {
  return {
    providerId,
    providerHost: "code.example.test",
    ownerOrProject: "TEAM",
    repositoryId: "app",
    repositoryName: "app",
    cloneUrl: "https://code.example.test/scm/TEAM/app.git",
  };
}

afterEach(() => {
  for (const pluginId of PLUGINS) pluginRegistry.unregisterPlugin(pluginId);
});

describe("inspectRegisteredRepositoryURL", () => {
  it("rejects ambiguous structured ownership instead of using registration order", async () => {
    pluginRegistry
      .forPlugin(PLUGINS[0]!)
      .registerRepositoryProvider(provider(PROVIDER_A_ID, async () => inspection(PROVIDER_A_ID)));
    pluginRegistry
      .forPlugin(PLUGINS[1]!)
      .registerRepositoryProvider(provider(PROVIDER_B_ID, async () => inspection(PROVIDER_B_ID)));

    await expect(
      inspectRegisteredRepositoryURL({
        workspaceId: WORKSPACE_ID,
        url: URL,
        signal: new AbortController().signal,
      }),
    ).rejects.toThrow("More than one repository provider recognized this URL");
  });

  it("keeps one exact match when another coarse candidate fails inspection", async () => {
    pluginRegistry.forPlugin(PLUGINS[0]!).registerRepositoryProvider(
      provider(PROVIDER_A_ID, async () => {
        throw new Error("candidate unavailable");
      }),
    );
    pluginRegistry
      .forPlugin(PLUGINS[1]!)
      .registerRepositoryProvider(provider(PROVIDER_B_ID, async () => inspection(PROVIDER_B_ID)));

    await expect(
      inspectRegisteredRepositoryURL({
        workspaceId: WORKSPACE_ID,
        url: URL,
        signal: new AbortController().signal,
      }),
    ).resolves.toMatchObject({ provider: { id: PROVIDER_B_ID } });
  });

  it("surfaces inspection failure when no provider can establish ownership", async () => {
    pluginRegistry.forPlugin(PLUGINS[0]!).registerRepositoryProvider(
      provider(PROVIDER_A_ID, async () => {
        throw new Error("provider inspection unavailable");
      }),
    );
    pluginRegistry
      .forPlugin(PLUGINS[1]!)
      .registerRepositoryProvider(provider(PROVIDER_B_ID, async () => null));

    await expect(
      inspectRegisteredRepositoryURL({
        workspaceId: WORKSPACE_ID,
        url: URL,
        signal: new AbortController().signal,
      }),
    ).rejects.toThrow("provider inspection unavailable");
  });
});
