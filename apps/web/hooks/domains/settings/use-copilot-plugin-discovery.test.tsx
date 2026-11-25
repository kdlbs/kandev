import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CopilotPluginDiscoveryResponse } from "@/lib/types/copilot-plugin-discovery";

const { getDiscovery } = vi.hoisted(() => ({ getDiscovery: vi.fn() }));
vi.mock("@/app/actions/agents", () => ({ getCopilotPluginDiscoveryAction: getDiscovery }));

import { useCopilotPluginDiscovery } from "./use-copilot-plugin-discovery";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

const discovery: CopilotPluginDiscoveryResponse = {
  agent_id: "agent-a",
  profile_id: "profile-a",
  provider_id: "copilot-acp",
  status: "ready",
  runtime_version: "1.0.88",
  mount_permitted: true,
  plugins: [],
};

beforeEach(() => getDiscovery.mockReset());

describe("useCopilotPluginDiscovery requests", () => {
  it("requests a saved profile by identity", async () => {
    getDiscovery.mockResolvedValue(discovery);
    const view = renderHook(() =>
      useCopilotPluginDiscovery({ agentId: "agent-a", profileId: "profile-a" }),
    );
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    expect(getDiscovery).toHaveBeenCalledWith(
      "agent-a",
      {
        profile_id: "profile-a",
        launch_settings: { env_vars: [], cli_flags: [], command_prefix: "" },
      },
      expect.any(AbortSignal),
    );
  });

  it("requests a draft with its complete launch snapshot", async () => {
    getDiscovery.mockResolvedValue({
      ...discovery,
      profile_id: "",
    });
    const launchSettings = {
      env_vars: [{ key: "HOME", value: "/tmp/home" }],
      cli_flags: [],
      command_prefix: "",
    };
    const view = renderHook(() =>
      useCopilotPluginDiscovery({ agentId: "agent-a", profileId: "", launchSettings }),
    );
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    expect(getDiscovery).toHaveBeenCalledWith(
      "agent-a",
      { launch_settings: launchSettings },
      expect.any(AbortSignal),
    );
  });
});

describe("useCopilotPluginDiscovery response fencing", () => {
  it("ignores a late response for a previous profile", async () => {
    const oldResponse = deferred<CopilotPluginDiscoveryResponse>();
    const newResponse = deferred<CopilotPluginDiscoveryResponse>();
    getDiscovery.mockReturnValueOnce(oldResponse.promise).mockReturnValueOnce(newResponse.promise);
    const view = renderHook(
      ({ profileId }) => useCopilotPluginDiscovery({ agentId: "agent-a", profileId }),
      { initialProps: { profileId: "profile-a" } },
    );
    view.rerender({ profileId: "profile-b" });

    await act(async () => {
      oldResponse.resolve({
        ...discovery,
        plugins: [
          {
            id: "installed:m/old",
            name: "Old",
            native_enabled: true,
            availability: "available",
          },
        ],
      });
      await oldResponse.promise;
    });
    expect(view.result.current.status).toBe("loading");

    await act(async () => {
      newResponse.resolve({ ...discovery, profile_id: "profile-b" });
      await newResponse.promise;
    });
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    expect(view.result.current.response?.profile_id).toBe("profile-b");
  });

  it("preserves the last same-context rows when refresh fails", async () => {
    getDiscovery.mockResolvedValueOnce({
      ...discovery,
      plugins: [
        {
          id: "installed:m/p",
          name: "p",
          native_enabled: true,
          availability: "available",
        },
      ],
    });
    const view = renderHook(() =>
      useCopilotPluginDiscovery({ agentId: "agent-a", profileId: "profile-a" }),
    );
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    getDiscovery.mockRejectedValueOnce(new Error("offline"));
    await act(async () => {
      await view.result.current.refresh();
    });
    expect(view.result.current.status).toBe("unavailable");
    expect(view.result.current.response?.plugins).toHaveLength(1);
  });

  it("requests an explicit cache-busting refresh", async () => {
    getDiscovery.mockResolvedValue(discovery);
    const view = renderHook(() =>
      useCopilotPluginDiscovery({ agentId: "agent-a", profileId: "profile-a" }),
    );
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    getDiscovery.mockClear();
    await act(async () => {
      await view.result.current.refresh();
    });
    expect(getDiscovery).toHaveBeenCalledWith(
      "agent-a",
      {
        profile_id: "profile-a",
        refresh: true,
        launch_settings: { env_vars: [], cli_flags: [], command_prefix: "" },
      },
      expect.any(AbortSignal),
    );
  });
});

describe("useCopilotPluginDiscovery launch context", () => {
  it("keeps a saved profile's launch snapshot in the request and stale key", async () => {
    getDiscovery.mockResolvedValue({ ...discovery, profile_id: "profile-a" });
    const view = renderHook(
      ({ home }) =>
        useCopilotPluginDiscovery({
          agentId: "agent-a",
          profileId: "profile-a",
          launchSettings: {
            env_vars: [{ key: "HOME", value: home }],
            cli_flags: [],
            command_prefix: "",
          },
        }),
      { initialProps: { home: "/home/before" } },
    );
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    expect(getDiscovery.mock.calls[0][1]).toMatchObject({
      profile_id: "profile-a",
      launch_settings: { env_vars: [{ key: "HOME", value: "/home/before" }] },
    });

    view.rerender({ home: "/home/after" });
    expect(view.result.current.stale).toBe(true);
    getDiscovery.mockClear();
    await act(async () => {
      await view.result.current.refresh();
    });
    const request = getDiscovery.mock.calls[0][1] as Record<string, unknown>;
    expect(request).toMatchObject({
      profile_id: "profile-a",
      refresh: true,
      launch_settings: { env_vars: [{ key: "HOME", value: "/home/after" }] },
    });
  });

  it("keeps a late old-context response stale for a saved profile", async () => {
    const oldResponse = deferred<CopilotPluginDiscoveryResponse>();
    getDiscovery.mockReturnValueOnce(oldResponse.promise).mockResolvedValueOnce({
      ...discovery,
      profile_id: "profile-a",
    });
    const view = renderHook(
      ({ home }) =>
        useCopilotPluginDiscovery({
          agentId: "agent-a",
          profileId: "profile-a",
          launchSettings: {
            env_vars: [{ key: "HOME", value: home }],
            cli_flags: [],
            command_prefix: "",
          },
        }),
      { initialProps: { home: "/home/before" } },
    );
    await waitFor(() => expect(getDiscovery).toHaveBeenCalledTimes(1));
    view.rerender({ home: "/home/after" });
    await act(async () => {
      oldResponse.resolve({
        ...discovery,
        profile_id: "profile-a",
        plugins: [
          {
            id: "installed:m/old",
            name: "Old",
            native_enabled: true,
            availability: "available",
          },
        ],
      });
      await oldResponse.promise;
    });
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    expect(view.result.current.stale).toBe(true);
    expect(view.result.current.response?.plugins[0]?.id).toBe("installed:m/old");

    getDiscovery.mockClear();
    await act(async () => {
      await view.result.current.refresh();
    });
    await waitFor(() => expect(view.result.current.stale).toBe(false));
    expect((getDiscovery.mock.calls[0][1] as Record<string, unknown>).refresh).toBe(true);
  });
});

describe("useCopilotPluginDiscovery stale launch context", () => {
  it("marks inventory stale when launch settings change without a refresh", async () => {
    getDiscovery.mockResolvedValue({ ...discovery, profile_id: "" });
    const view = renderHook(
      ({ commandPrefix }) =>
        useCopilotPluginDiscovery({
          agentId: "agent-a",
          profileId: "",
          launchSettings: { env_vars: [], cli_flags: [], command_prefix: commandPrefix },
        }),
      { initialProps: { commandPrefix: "" } },
    );
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    view.rerender({ commandPrefix: "sandbox --" });
    expect(view.result.current.stale).toBe(true);
    expect(getDiscovery).toHaveBeenCalledTimes(1);
  });
});
