import { describe, expect, it } from "vitest";
import {
  orderProfilesForSelection,
  orderProfileOptionsForSelection,
} from "./agent-profile-selector-order";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

const JANUARY = "2026-01-01T00:00:00Z";
const FEBRUARY = "2026-02-01T00:00:00Z";

describe("selector creation baseline", () => {
  it("compares creation instants including submillisecond precision before breaking ties by ID", () => {
    const profiles = [
      { id: "a", createdAt: "2026-01-01T00:00:00.000100Z" },
      { id: "z", createdAt: "2026-01-01T00:00:00.000200Z" },
      { id: "b", createdAt: "2026-01-01T01:00:00.000200+01:00" },
    ];
    expect(orderProfilesForSelection(profiles).map((profile) => profile.id)).toEqual([
      "b",
      "z",
      "a",
    ]);
    expect(profiles.map((profile) => profile.id)).toEqual(["a", "z", "b"]);
  });

  it.each(["invalid", "2026-02-30T00:00:00Z", "2026-01-01", "9999"])(
    "preserves source order when creation timestamp %s is malformed",
    (createdAt) => {
      const profiles = [
        { id: "z", createdAt: JANUARY },
        { id: "a", createdAt },
        { id: "b", createdAt: FEBRUARY },
      ];
      expect(orderProfilesForSelection(profiles)).toBe(profiles);
    },
  );

  it("keeps other agent slots and unstamped orphan options without using profile updatedAt", () => {
    const options = [
      {
        id: "old",
        agent_id: "a",
        createdAt: JANUARY,
        updatedAt: "2026-03-01T00:00:00Z",
      },
      { id: "office", agent_id: "office" },
      { id: "new", agent_id: "a", createdAt: FEBRUARY },
    ] as AgentProfileOption[];
    expect(orderProfileOptionsForSelection(options).map((profile) => profile.id)).toEqual([
      "new",
      "office",
      "old",
    ]);
  });

  it("preserves an unstamped Office option in the owning agent's slots without blocking the global baseline", () => {
    const options = [
      { id: "old", agent_id: "a", createdAt: JANUARY },
      { id: "office", agent_id: "a", workspace_id: "office-workspace" },
      { id: "new", agent_id: "a", createdAt: FEBRUARY },
    ] as AgentProfileOption[];
    expect(orderProfileOptionsForSelection(options).map((profile) => profile.id)).toEqual([
      "new",
      "office",
      "old",
    ]);
  });
  it("keeps a stamped workspace-scoped option in its slot while restoring global profile order", () => {
    const options = [
      { id: "old", agent_id: "a", createdAt: JANUARY },
      {
        id: "office",
        agent_id: "a",
        workspace_id: "office-workspace",
        createdAt: "2026-03-01T00:00:00Z",
      },
      { id: "new", agent_id: "a", createdAt: FEBRUARY },
    ] as AgentProfileOption[];
    expect(orderProfileOptionsForSelection(options).map((profile) => profile.id)).toEqual([
      "new",
      "office",
      "old",
    ]);
  });
});
