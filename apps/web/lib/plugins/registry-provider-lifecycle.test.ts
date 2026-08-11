import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "./registry";
import type { RepositoryProviderRegistration } from "./types";

const PRIMARY_PLUGIN_ID = "plugin-a";
const SOURCE_CONTROL_PROVIDER_ID = "source-control";
const WORKSPACE_ID = "workspace-a";

function repositoryProvider(
  overrides: Partial<RepositoryProviderRegistration> = {},
): RepositoryProviderRegistration {
  return {
    id: SOURCE_CONTROL_PROVIDER_ID,
    label: SOURCE_CONTROL_PROVIDER_ID,
    listRepositories: async () => [],
    matchesURL: () => false,
    listBranches: async () => [],
    inspectURL: async () => null,
    ...overrides,
  };
}

afterEach(() => {
  pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
});

describe("pluginRegistry — repository provider result lifecycle", () => {
  it("binds repository inspection identity to the registered provider", async () => {
    const spoofed = {
      providerId: "spoofed-provider",
      providerHost: "code.example.test",
      ownerOrProject: "team",
      repositoryId: "team/app",
      repositoryName: "app",
      cloneUrl: "https://code.example.test/team/app.git",
    };
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(
      repositoryProvider({
        listRepositories: async () => [spoofed],
        inspectURL: async () => spoofed,
      }),
    );
    const provider = pluginRegistry.getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)!;
    const signal = new AbortController().signal;

    await expect(provider.listRepositories({ workspaceId: WORKSPACE_ID, signal })).resolves.toEqual(
      [{ ...spoofed, providerId: SOURCE_CONTROL_PROVIDER_ID }],
    );
    await expect(
      provider.inspectURL({ workspaceId: WORKSPACE_ID, url: spoofed.cloneUrl, signal }),
    ).resolves.toEqual({ ...spoofed, providerId: SOURCE_CONTROL_PROVIDER_ID });
  });

  it("aborts in-flight repository work when its owner unloads", async () => {
    const aborted = vi.fn();
    let markStarted: () => void;
    const started = new Promise<void>((resolve) => {
      markStarted = resolve;
    });
    const provider = repositoryProvider({
      listRepositories: ({ signal }) =>
        new Promise((_, reject) => {
          markStarted();
          signal.addEventListener(
            "abort",
            () => {
              aborted();
              reject(new Error("provider request aborted"));
            },
            { once: true },
          );
        }),
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(provider);

    const request = pluginRegistry
      .getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)
      ?.listRepositories({ workspaceId: WORKSPACE_ID, signal: new AbortController().signal });
    await started;
    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);

    await expect(request).rejects.toThrow("provider request aborted");
    expect(aborted).toHaveBeenCalledOnce();
  });
});

describe("pluginRegistry — repository provider reload lifecycle", () => {
  it("keeps re-enabled provider work tracked after an older request settles", async () => {
    let rejectFirst!: (error: Error) => void;
    let markFirstStarted!: () => void;
    const firstStarted = new Promise<void>((resolve) => {
      markFirstStarted = resolve;
    });
    const first = repositoryProvider({
      listRepositories: () =>
        new Promise((_, reject) => {
          rejectFirst = reject;
          markFirstStarted();
        }),
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(first);
    const firstRequest = pluginRegistry
      .getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)!
      .listRepositories({ workspaceId: WORKSPACE_ID, signal: new AbortController().signal });
    await firstStarted;

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
    const secondAborted = vi.fn();
    let markSecondStarted!: () => void;
    const secondStarted = new Promise<void>((resolve) => {
      markSecondStarted = resolve;
    });
    const second = repositoryProvider({
      listRepositories: ({ signal }) =>
        new Promise((_, reject) => {
          markSecondStarted();
          signal.addEventListener(
            "abort",
            () => {
              secondAborted();
              reject(new Error("second provider request aborted"));
            },
            { once: true },
          );
        }),
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(second);
    const secondRequest = pluginRegistry
      .getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)!
      .listRepositories({ workspaceId: WORKSPACE_ID, signal: new AbortController().signal });
    await secondStarted;
    rejectFirst(new Error("first provider request stopped"));
    await expect(firstRequest).rejects.toThrow();

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);

    await expect(secondRequest).rejects.toThrow("second provider request aborted");
    expect(secondAborted).toHaveBeenCalledOnce();
  });
});

describe("pluginRegistry — repository change-request lifecycle", () => {
  it("aborts provider change-request creation when its owner unloads", async () => {
    let markStarted!: () => void;
    const started = new Promise<void>((resolve) => (markStarted = resolve));
    const aborted = vi.fn();
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(
      repositoryProvider({
        createChangeRequest: ({ signal }) =>
          new Promise((_, reject) => {
            markStarted();
            signal.addEventListener(
              "abort",
              () => {
                aborted();
                reject(new Error("create aborted"));
              },
              { once: true },
            );
          }),
      }),
    );
    const provider = pluginRegistry.getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)!;
    const request = provider.createChangeRequest!({
      workspaceId: WORKSPACE_ID,
      taskId: "task-a",
      sessionId: "session-a",
      repositoryId: "repo-a",
      repository: {
        id: "repo-a",
        workspace_id: WORKSPACE_ID,
        name: "api",
        provider: SOURCE_CONTROL_PROVIDER_ID,
      },
      title: "Title",
      body: "",
      draft: false,
      signal: new AbortController().signal,
    });
    await started;

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);

    await expect(request).rejects.toThrow("create aborted");
    expect(aborted).toHaveBeenCalledOnce();
  });
});
