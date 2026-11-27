import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { ApiError } from "@/lib/api/client";
import { ProfileOrderQueue } from "./profile-order-queue";
import { listAgents } from "@/lib/api";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { Agent } from "@/lib/types/http";
import type * as AgentsApi from "@/lib/api";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof AgentsApi>()),
  listAgents: vi.fn(),
}));

const store = createAppStore();
const MEMBERSHIP_EVENT_TIME = "2026-03-01T00:00:00Z";

beforeEach(() => {
  store.getState().setSettingsAgents([
    {
      id: "a",
      name: "Agent",
      profiles: [
        { id: "x", name: "X" },
        { id: "y", name: "Y" },
      ],
    } as never,
  ]);
  store.setState((state) => {
    state.agentProfiles.orderByAgent.a = {
      revision: 1,
      order: ["x", "y"],
      inFlight: null,
      queued: null,
    };
  });
});
describe("ProfileOrderQueue", () => {
  it("submits the latest intent after an earlier save rejects", async () => {
    let rejectFirst!: (error: Error) => void;
    const first = new Promise<{ profile_ids: string[]; revision: number }>((_, reject) => {
      rejectFirst = reject;
    });
    const save = vi
      .fn()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({ profile_ids: ["x", "y"], revision: 2 });
    const queue = new ProfileOrderQueue(store, { save });
    queue.requestProfileOrder("a", ["y", "x"]);
    queue.requestProfileOrder("a", ["x", "y"]);
    rejectFirst(new Error("failed"));
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(store.getState().agentProfiles.orderByAgent.a.queued).toBeNull());
    expect(save.mock.calls.map((call) => call[1])).toEqual([
      ["y", "x"],
      ["x", "y"],
    ]);
    expect(store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      "x",
      "y",
    ]);
  });

  it("clears the optimistic order after the latest save fails", async () => {
    const save = vi.fn().mockRejectedValue(new Error("failed"));
    const onError = vi.fn();
    const queue = new ProfileOrderQueue(store, { save, onError });
    queue.requestProfileOrder("a", ["y", "x"]);
    await vi.waitFor(() => expect(onError).toHaveBeenCalledOnce());
    expect(store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      "x",
      "y",
    ]);
    expect(store.getState().agentProfiles.orderByAgent.a.inFlight).toBeNull();
  });
});

describe("ProfileOrderQueue creation rollback", () => {
  it("restores the saved order of newly created profiles after a failed drag", async () => {
    const baseline = {
      id: "a",
      name: "Agent",
      profile_order_revision: 1,
      profiles: [
        { id: "new2", name: "New 2" },
        { id: "new1", name: "New 1" },
        { id: "x", name: "X" },
        { id: "y", name: "Y" },
      ],
    } as Agent;
    store.getState().applyAgentListSnapshot([baseline], store.getState().agentProfiles.version);
    const onError = vi.fn();
    const queue = new ProfileOrderQueue(store, {
      save: vi.fn().mockRejectedValue(new Error("failed")),
      onError,
    });
    queue.requestProfileOrder("a", ["new1", "new2", "y", "x"]);
    await vi.waitFor(() => expect(onError).toHaveBeenCalledOnce());
    expect(store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      "new2",
      "new1",
      "x",
      "y",
    ]);
  });

  it("restores live creation order and keeps deletions after a failed drag", async () => {
    const liveAgentId = "live-events";
    const firstCreationId = "live-new1";
    const secondCreationId = "live-new2";
    const firstOldId = "live-old1";
    const secondOldId = "live-old2";
    const local = createAppStore();
    local.getState().setSettingsAgents([
      {
        id: liveAgentId,
        name: "Live events",
        profile_order_revision: 3,
        profiles: [
          { id: firstOldId, name: "Old 1" },
          { id: secondOldId, name: "Old 2" },
        ],
      } as Agent,
    ]);
    const handlers = registerAgentsHandlers(local);
    for (const id of [firstCreationId, secondCreationId]) {
      handlers["agent.profile.created"]!({
        timestamp: MEMBERSHIP_EVENT_TIME,
        payload: {
          profile: { id, agent_id: liveAgentId, name: id, created_at: MEMBERSHIP_EVENT_TIME },
        },
      } as BackendMessageMap["agent.profile.created"]);
    }
    let rejectSave!: (error: Error) => void;
    const pending = new Promise<{ profile_ids: string[]; revision: number }>((_, reject) => {
      rejectSave = reject;
    });
    const onError = vi.fn();
    const queue = new ProfileOrderQueue(local, { save: vi.fn().mockReturnValue(pending), onError });
    queue.requestProfileOrder(liveAgentId, [
      firstCreationId,
      secondCreationId,
      secondOldId,
      firstOldId,
    ]);
    handlers["agent.profile.deleted"]!({
      timestamp: MEMBERSHIP_EVENT_TIME,
      payload: { profile: { id: secondOldId, agent_id: liveAgentId } },
    } as BackendMessageMap["agent.profile.deleted"]);
    rejectSave(new Error("failed"));
    await vi.waitFor(() => expect(onError).toHaveBeenCalledOnce());
    expect(local.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      secondCreationId,
      firstCreationId,
      firstOldId,
    ]);
    expect(local.getState().agentProfiles.orderByAgent[liveAgentId].order).toEqual([
      secondCreationId,
      firstCreationId,
      firstOldId,
    ]);
  });
});

describe("ProfileOrderQueue conflict replay", () => {
  it("refetches on conflict and replays only surviving queued profiles after new IDs", async () => {
    let rejectFirst!: (error: Error) => void;
    const first = new Promise<{ profile_ids: string[]; revision: number }>((_, reject) => {
      rejectFirst = reject;
    });
    const save = vi
      .fn()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({ profile_ids: ["new", "x", "y"], revision: 4 });
    const read = vi.fn().mockResolvedValue({
      agents: [
        {
          id: "a",
          name: "Agent",
          profile_order_revision: 3,
          profiles: [
            { id: "new", name: "New" },
            { id: "x", name: "X" },
            { id: "y", name: "Y" },
          ],
        },
      ],
    });
    const queue = new ProfileOrderQueue(store, { save, read });
    queue.requestProfileOrder("a", ["y", "x"]);
    queue.requestProfileOrder("a", ["y", "deleted", "x"]);
    rejectFirst(new ApiError("stale", 409, {}));
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));
    expect(read).toHaveBeenCalledOnce();
    expect(save.mock.calls[1][1]).toEqual(["new", "y", "x"]);
  });
});

describe("ProfileOrderQueue membership and revision races", () => {
  it("discards a deferred conflict refetch after membership events and replays against a fresh resource snapshot", async () => {
    const local = createAppStore();
    const oldAgent = {
      id: "queue-agent",
      name: "Agent",
      profile_order_revision: 1,
      profiles: [
        { id: "queue-x", name: "X", createdAt: "2026-01-01T00:00:00Z" },
        { id: "queue-y", name: "Y", createdAt: "2026-02-01T00:00:00Z" },
      ],
    } as Agent;
    local.getState().applyAgentListSnapshot([oldAgent], 0);
    let resolveStale!: (response: { agents: Agent[] }) => void;
    let resolveFresh!: (response: { agents: Agent[]; total: number }) => void;
    const stale = new Promise<{ agents: Agent[] }>((resolve) => {
      resolveStale = resolve;
    });
    const fresh = new Promise<{ agents: Agent[]; total: number }>((resolve) => {
      resolveFresh = resolve;
    });
    vi.mocked(listAgents).mockReturnValueOnce(fresh);
    const read = vi.fn().mockReturnValueOnce(stale);
    const save = vi
      .fn()
      .mockRejectedValueOnce(new ApiError("stale", 409, {}))
      .mockResolvedValueOnce({ profile_ids: ["queue-new", "queue-y"], revision: 2 });
    const queue = new ProfileOrderQueue(local, { save, read });
    queue.requestProfileOrder(oldAgent.id, ["queue-y", "queue-x"]);
    queue.requestProfileOrder(oldAgent.id, ["queue-y", "queue-x"]);
    await vi.waitFor(() => expect(read).toHaveBeenCalledOnce());
    const handlers = registerAgentsHandlers(local);
    handlers["agent.profile.created"]!({
      timestamp: MEMBERSHIP_EVENT_TIME,
      payload: {
        profile: {
          id: "queue-new",
          agent_id: oldAgent.id,
          name: "New",
          created_at: MEMBERSHIP_EVENT_TIME,
        },
      },
    } as BackendMessageMap["agent.profile.created"]);
    handlers["agent.profile.deleted"]!({
      timestamp: MEMBERSHIP_EVENT_TIME,
      payload: { profile: { id: "queue-x", agent_id: oldAgent.id } },
    } as BackendMessageMap["agent.profile.deleted"]);
    resolveStale({ agents: [oldAgent] });
    await vi.waitFor(() => expect(listAgents).toHaveBeenCalledOnce());
    expect(local.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      "queue-new",
      "queue-y",
    ]);
    expect(local.getState().agentProfiles.items.map((profile) => profile.id)).toEqual([
      "queue-new",
      "queue-y",
    ]);
    resolveFresh({
      total: 1,
      agents: [
        {
          ...oldAgent,
          profiles: [
            { id: "queue-new", name: "New", createdAt: MEMBERSHIP_EVENT_TIME },
            oldAgent.profiles[1],
          ],
        } as Agent,
      ],
    });
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));
    expect(save.mock.calls[1][1]).toEqual(["queue-new", "queue-y"]);
    await vi.waitFor(() =>
      expect(local.getState().agentProfiles.orderByAgent[oldAgent.id].inFlight).toBeNull(),
    );
  });

  it("does not let a late successful response replace a higher-revision foreign order", async () => {
    let resolveSave!: (response: { profile_ids: string[]; revision: number }) => void;
    const pending = new Promise<{ profile_ids: string[]; revision: number }>((resolve) => {
      resolveSave = resolve;
    });
    const queue = new ProfileOrderQueue(store, { save: vi.fn().mockReturnValueOnce(pending) });
    queue.requestProfileOrder("a", ["y", "x"]);
    store.getState().acceptAgentProfileOrder("a", ["x", "y"], 4);
    resolveSave({ profile_ids: ["y", "x"], revision: 3 });
    await vi.waitFor(() =>
      expect(store.getState().agentProfiles.orderByAgent.a.inFlight).toBeNull(),
    );
    expect(store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      "x",
      "y",
    ]);
    expect(store.getState().agentProfiles.orderByAgent.a.revision).toBe(4);
  });
});
