import { describe, expect, it, vi } from "vitest";

const { fetchRecentUseMock } = vi.hoisted(() => ({
  fetchRecentUseMock: vi.fn(),
}));

vi.mock("@/lib/api/domains/agent-profile-recent-use-api", () => ({
  fetchAgentProfileRecentUse: fetchRecentUseMock,
  recordAgentProfileRecentUse: vi.fn(),
}));
import {
  ensureAgentProfileRecentUseLoaded,
  mergeAgentProfileRecentUseState,
  orderAgentProfilesByRecentUse,
} from "./agent-profile-recent-use";

const profiles = [{ id: "profile-a" }, { id: "profile-b" }, { id: "profile-c" }];

describe("orderAgentProfilesByRecentUse", () => {
  it("moves remembered profiles ahead of unseen profiles while preserving both orders", () => {
    const ordered = orderAgentProfilesByRecentUse(profiles, ["profile-c", "missing", "profile-a"]);

    expect(ordered.map((profile) => profile.id)).toEqual(["profile-c", "profile-a", "profile-b"]);
  });

  it("uses at most ten ranked ids and does not duplicate profiles", () => {
    const source = Array.from({ length: 12 }, (_, index) => ({ id: `profile-${index}` }));
    const rankedIds = [...source].reverse().map((profile) => profile.id);

    const ordered = orderAgentProfilesByRecentUse(source, rankedIds);

    expect(ordered.slice(0, 10).map((profile) => profile.id)).toEqual(rankedIds.slice(0, 10));
    expect(new Set(ordered.map((profile) => profile.id)).size).toBe(source.length);
  });

  it("keeps the current selection ahead of remembered profiles", () => {
    const ordered = orderAgentProfilesByRecentUse(profiles, ["profile-c"], "profile-b");

    expect(ordered.map((profile) => profile.id)).toEqual(["profile-b", "profile-c", "profile-a"]);
  });
});

describe("ensureAgentProfileRecentUseLoaded", () => {
  it("deduplicates concurrent recovery reads and merges their response into the store", async () => {
    const setAgentProfileRecentUse = vi.fn();
    const store = {
      getState: () => ({
        agentProfileRecentUse: { records: {}, loaded: false },
        setAgentProfileRecentUse,
      }),
    };
    fetchRecentUseMock.mockResolvedValueOnce([
      {
        context: "task_create",
        profile_ids: ["profile-a"],
        revision: 2,
        updated_at: "2026-08-27T12:00:00Z",
      },
    ]);

    const first = ensureAgentProfileRecentUseLoaded(store);
    const second = ensureAgentProfileRecentUseLoaded(store);

    expect(second).toBe(first);
    await first;
    expect(fetchRecentUseMock).toHaveBeenCalledOnce();
    expect(setAgentProfileRecentUse).toHaveBeenCalledWith({
      records: {
        task_create: {
          profileIds: ["profile-a"],
          revision: 2,
          updatedAt: "2026-08-27T12:00:00Z",
        },
      },
      loaded: true,
    });
  });
});

describe("mergeAgentProfileRecentUseState", () => {
  it("keeps newer context revisions and merges independent contexts", () => {
    const current = {
      loaded: true,
      records: {
        task_create: { profileIds: ["profile-a"], revision: 4, updatedAt: "later" },
      },
    };
    const merged = mergeAgentProfileRecentUseState(current, {
      loaded: true,
      records: {
        task_create: { profileIds: ["profile-b"], revision: 3, updatedAt: "older" },
        quick_chat: { profileIds: ["profile-c"], revision: 1, updatedAt: "new" },
      },
    });

    expect(merged.records.task_create?.profileIds).toEqual(["profile-a"]);
    expect(merged.records.quick_chat?.profileIds).toEqual(["profile-c"]);
  });
});
