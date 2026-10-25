import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { AgentListResponse } from "./agent-list-resource";
import { getAgentListResourceScope } from "./agent-list-resource";

const mocks = vi.hoisted(() => ({ listAgents: vi.fn() }));
vi.mock("@/lib/api", () => ({ listAgents: mocks.listAgents }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function response(profileId: string): AgentListResponse {
  return {
    total: 1,
    agents: [
      {
        id: "agent-one",
        name: "Agent One",
        profiles: [{ id: profileId, agentDisplayName: "Agent One", name: profileId }],
      },
    ],
  } as AgentListResponse;
}

describe("agent list resource", () => {
  beforeEach(() => mocks.listAgents.mockReset());

  it("profile edit queues one fresh read and does not apply the stale response", async () => {
    const first = deferred<AgentListResponse>();
    const second = deferred<AgentListResponse>();
    mocks.listAgents.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const store = createAppStore();
    const scope = getAgentListResourceScope(store);
    const release = scope.subscribe(vi.fn());

    const read = scope.ensure();
    await vi.waitFor(() => expect(mocks.listAgents).toHaveBeenCalledTimes(1));
    store.getState().bumpAgentProfilesVersion();
    first.resolve(response("stale-profile"));
    await vi.waitFor(() => expect(mocks.listAgents).toHaveBeenCalledTimes(2));
    second.resolve(response("current-profile"));
    await read;

    expect(store.getState().agentProfiles.items.map((profile) => profile.id)).toEqual([
      "current-profile",
    ]);
    expect(scope.getSnapshot()).toMatchObject({ loaded: true, profileVersion: 1 });
    release();
  });
});
