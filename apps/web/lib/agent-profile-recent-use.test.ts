import { describe, expect, it } from "vitest";
import {
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
