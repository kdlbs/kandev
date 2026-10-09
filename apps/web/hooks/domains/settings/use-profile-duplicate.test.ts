import { describe, expect, it, vi } from "vitest";
import { applyProfileDuplicated } from "./use-profile-duplicate";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

vi.mock("@/components/toast-provider", () => ({ useToast: vi.fn() }));

const profile = (id: string, name: string) => ({ id, name, agentId: "a" }) as AgentProfile;
const option = (id: string): AgentProfileOption =>
  ({
    id,
    label: id,
    agent_id: "a",
    agent_name: "Agent",
    cli_passthrough: false,
  }) as AgentProfileOption;

describe("profile duplication ordering", () => {
  it("prepends the copy in Settings without applying saved order to existing selector options", () => {
    const older = { ...profile("older", "Older"), createdAt: "2026-01-01T00:00:00Z" };
    const newer = { ...profile("newer", "Newer"), createdAt: "2026-02-01T00:00:00Z" };
    const agent = { id: "a", name: "Agent", profiles: [older, newer] } as Agent;
    const state = {
      settingsAgents: { items: [agent] },
      agentProfiles: {
        items: [
          { ...option("newer"), createdAt: newer.createdAt },
          { ...option("older"), createdAt: older.createdAt },
          { ...option("office"), agent_id: "office" },
        ],
        version: 0,
        orderByAgent: {},
      },
    };
    const next = applyProfileDuplicated(state, agent, {
      ...profile("copy", "Copy"),
      createdAt: "2026-03-01T00:00:00Z",
    });
    expect(next.settingsAgents.items[0].profiles.map((item) => item.id)).toEqual([
      "copy",
      "older",
      "newer",
    ]);
    expect(next.agentProfiles.items.map((item) => item.id)).toEqual([
      "copy",
      "newer",
      "older",
      "office",
    ]);
  });

  it("prepends a duplicated profile within its agent in both mirrored lists", () => {
    const agent = {
      id: "a",
      name: "Agent",
      profiles: [profile("old-1", "One"), profile("old-2", "Two")],
    } as Agent;
    const state = {
      settingsAgents: { items: [agent] },
      agentProfiles: { items: [option("old-1"), option("old-2")], version: 0, orderByAgent: {} },
    };
    const next = applyProfileDuplicated(state, agent, profile("new", "Copy"));
    expect(next.settingsAgents.items[0].profiles.map((item) => item.id)).toEqual([
      "new",
      "old-1",
      "old-2",
    ]);
    expect(next.agentProfiles.items.map((item) => item.id)).toEqual(["new", "old-1", "old-2"]);
  });
});

it("records a duplicate in the rollback baseline while retaining pending intent", () => {
  const copyId = "copy-pending";
  const agent = {
    id: "a",
    name: "Agent",
    profiles: [profile("old-1", "One"), profile("old-2", "Two")],
  } as Agent;
  const state = {
    settingsAgents: { items: [agent] },
    agentProfiles: {
      items: [option("old-1"), option("old-2")],
      version: 0,
      orderByAgent: {
        a: { revision: 1, order: ["old-1", "old-2"], inFlight: ["old-2", "old-1"], queued: null },
      },
    },
  };
  const next = applyProfileDuplicated(state, agent, profile(copyId, "Copy"));
  expect(next.agentProfiles.orderByAgent.a.order).toEqual([copyId, "old-1", "old-2"]);
  expect(next.agentProfiles.orderByAgent.a.inFlight).toEqual(["old-2", "old-1"]);
  expect(next.settingsAgents.items[0].profiles.map((item) => item.id)).toEqual([
    copyId,
    "old-2",
    "old-1",
  ]);
});
