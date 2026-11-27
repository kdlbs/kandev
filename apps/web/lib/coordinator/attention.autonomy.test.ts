import { describe, expect, it } from "vitest";
import { classify, type AttentionAutonomyInput, type AttentionTask } from "./attention";

const NOW = Date.parse("2026-09-30T10:00:00Z");

function held(over: Partial<AttentionAutonomyInput> = {}): AttentionAutonomyInput {
  return {
    coordinatorId: "c1",
    enabled: true,
    reason: "ceiling_reached",
    detail: "",
    pendingWakes: 2,
    oldestPendingAt: "2026-09-30T09:00:00Z",
    loadedAt: NOW,
    conditions: [],
    ...over,
  };
}

function errorTask(id: string, lastActivityAt: string): AttentionTask {
  return {
    id,
    title: id,
    statusSummary: {
      last_activity_at: lastActivityAt,
      primary_session: { id: "s", state: "FAILED" },
      active_error: { preview: "boom" },
    },
  };
}

describe("classify autonomy item", () => {
  it("emits one item for each of the five persistent reasons with pending wakes", () => {
    for (const reason of [
      "containment",
      "spend_unmeasured",
      "ceiling_reached",
      "no_conversation",
      "conversation_unavailable",
    ]) {
      const { needsYou } = classify([], [], [], NOW, held({ reason }));
      expect(needsYou).toHaveLength(1);
      expect(needsYou[0]).toMatchObject({ kind: "autonomy", id: "autonomy:c1", reason });
    }
  });

  it("emits nothing for transient, unknown or missing reasons", () => {
    for (const reason of ["conversation_busy", "cooldown", "brand_new_reason", undefined]) {
      expect(classify([], [], [], NOW, held({ reason })).needsYou).toEqual([]);
    }
  });

  it("emits nothing without pending wakes or when autonomy is off", () => {
    expect(classify([], [], [], NOW, held({ pendingWakes: 0 })).needsYou).toEqual([]);
    expect(classify([], [], [], NOW, held({ enabled: false })).needsYou).toEqual([]);
    expect(classify([], [], [], NOW, null).needsYou).toEqual([]);
    expect(classify([], [], [], NOW).needsYou).toEqual([]);
  });

  it("takes its reference time from oldest_pending_at, else the read time", () => {
    const [a] = classify([], [], [], NOW, held()).needsYou;
    expect(a.referenceTimeMs).toBe(Date.parse("2026-09-30T09:00:00Z"));
    expect(a.ageMs).toBe(3_600_000);
    const [b] = classify([], [], [], NOW, held({ oldestPendingAt: null })).needsYou;
    expect(b.referenceTimeMs).toBe(NOW);
  });

  it("orders after an error of the same age and before a younger error", () => {
    const same = "2026-09-30T09:00:00Z";
    const tasks = [errorTask("e-same", same), errorTask("e-young", "2026-09-30T09:30:00Z")];
    const ids = classify(tasks, [], [], NOW, held()).needsYou.map((i) => i.id);
    expect(ids).toEqual(["e-same", "autonomy:c1", "e-young"]);
  });
});
