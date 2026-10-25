import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";

describe("agent profile recent-use state", () => {
  it("hydrates and applies only newer revisions per context", () => {
    const store = createAppStore({
      agentProfileRecentUse: {
        loaded: true,
        records: {
          task_create: {
            profileIds: ["profile-a"],
            revision: 4,
            updatedAt: "later",
          },
        },
      },
    });

    store.getState().applyAgentProfileRecentUse("task_create", {
      profileIds: ["profile-old"],
      revision: 3,
      updatedAt: "older",
    });
    store.getState().applyAgentProfileRecentUse("quick_chat", {
      profileIds: ["profile-chat"],
      revision: 1,
      updatedAt: "new",
    });

    expect(store.getState().agentProfileRecentUse.records.task_create?.profileIds).toEqual([
      "profile-a",
    ]);
    expect(store.getState().agentProfileRecentUse.records.quick_chat?.profileIds).toEqual([
      "profile-chat",
    ]);
  });
});
