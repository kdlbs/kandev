import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentMcpDiscoveryResponse } from "@/lib/types/agent-mcp-discovery";

const { getDiscovery } = vi.hoisted(() => ({ getDiscovery: vi.fn() }));
vi.mock("@/app/actions/agents", () => ({ getAgentMcpDiscoveryAction: getDiscovery }));

import { useAgentMcpDiscovery } from "./use-agent-mcp-discovery";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

const discovery: AgentMcpDiscoveryResponse = {
  agent_id: "agent-a",
  provider_id: "cursor",
  status: "ready",
  servers: [],
};

describe("useAgentMcpDiscovery", () => {
  beforeEach(() => getDiscovery.mockReset());

  it("ignores a response for the previous agent profile", async () => {
    const oldResponse = deferred<AgentMcpDiscoveryResponse>();
    const newResponse = deferred<AgentMcpDiscoveryResponse>();
    getDiscovery.mockReturnValueOnce(oldResponse.promise).mockReturnValueOnce(newResponse.promise);
    const view = renderHook(({ agentId, profileId }) => useAgentMcpDiscovery(agentId, profileId), {
      initialProps: { agentId: "agent-a", profileId: "profile-a" },
    });
    view.rerender({ agentId: "agent-b", profileId: "profile-b" });

    await act(async () => {
      oldResponse.resolve({
        ...discovery,
        servers: [
          {
            id: "old",
            name: "Old",
            source_kind: "plugin",
            credentials_available: false,
          },
        ],
      });
      await oldResponse.promise;
    });
    expect(view.result.current.status).toBe("loading");
    expect(view.result.current.profileKey).toBe("agent-b:profile-b");

    await act(async () => {
      newResponse.resolve({ ...discovery, agent_id: "agent-b" });
      await newResponse.promise;
    });
    await waitFor(() => expect(view.result.current.status).toBe("ready"));
    expect(view.result.current.response?.servers).toEqual([]);
  });

  it("retains the last good profile discovery when refresh fails", async () => {
    const goodDiscovery = {
      ...discovery,
      servers: [
        {
          id: "plugin-atlassian-jira",
          name: "Jira tools",
          source_kind: "plugin" as const,
          credentials_available: true,
        },
      ],
    };
    getDiscovery.mockResolvedValueOnce(goodDiscovery);
    const view = renderHook(() => useAgentMcpDiscovery("agent-a", "profile-a"));
    await waitFor(() => expect(view.result.current.status).toBe("ready"));

    getDiscovery.mockRejectedValueOnce(new Error("private discovery error"));
    await act(async () => view.result.current.refresh());
    expect(view.result.current.status).toBe("unavailable");
    expect(view.result.current.response).toEqual(goodDiscovery);
  });

  it("does not accept discovery returned for a different agent identity", async () => {
    getDiscovery.mockResolvedValueOnce({ ...discovery, agent_id: "another-agent" });
    const view = renderHook(() => useAgentMcpDiscovery("agent-a", "profile-a"));
    await waitFor(() => expect(view.result.current.status).toBe("unavailable"));
    expect(view.result.current.response).toBeUndefined();
  });
});
