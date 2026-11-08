import { describe, expect, it } from "vitest";
import { retainControlRows } from "./use-control-resource";
import { retainCapacity, retainControlCollection } from "./control-refresh-snapshot";
import type { SessionCapacityObservation } from "@/lib/types/system";
import type { ControlCollection } from "./control-conversation-lists";
import type { ControlReadStatus } from "./use-control-resource";
type Row = { id: string; status: ControlReadStatus; data?: number[] };

describe("authorized snapshot continuity", () => {
  it("keeps a failed workspace's old timestamp while publishing its successful sibling", () => {
    const first = retainControlRows<Row>(undefined, [
      { id: "one", status: "ready", data: [1] },
      { id: "two", status: "ready", data: [2] },
    ]);
    const next = retainControlRows(first, [
      { id: "one", status: "error", data: undefined },
      { id: "two", status: "ready", data: [3] },
    ]);
    expect(next[0]).toMatchObject({
      status: "error",
      stale: true,
      data: [1],
      receivedAt: first[0].receivedAt,
    });
    expect(next[1].data).toEqual([3]);
    expect(next[1].stale).toBeUndefined();
  });
  it.each(["denied", "unsupported"] as const)(
    "discards %s rows and removed workspaces",
    (status) => {
      const old = retainControlRows<Row>(undefined, [
        { id: "one", status: "ready", data: [1] },
        { id: "removed", status: "ready", data: [2] },
      ]);
      const next = retainControlRows(old, [{ id: "one", status, data: undefined }]);
      expect(next).toHaveLength(1);
      expect(next[0].data).toBeUndefined();
      expect(next[0].receivedAt).toBeUndefined();
    },
  );
  it("keeps navigation rows through transport errors and clears them on lost permission", () => {
    const previous: ControlCollection<string> = {
      items: ["conversation"],
      error: false,
      denied: false,
      receivedAt: "2026-10-19T10:00:00Z",
    };
    const failed = retainControlCollection(previous, {
      items: [],
      error: true,
      denied: false,
      transient: true,
    });
    expect(failed.items).toEqual(previous.items);
    expect(failed.receivedAt).toBe(previous.receivedAt);
    expect(failed.stale).toBe(true);
    const denied = retainControlCollection(failed, { items: [], error: true, denied: true });
    expect(denied.items).toEqual([]);
    expect(denied.receivedAt).toBeUndefined();
  });
});

describe("latest unknown and historical capacity", () => {
  const observation: SessionCapacityObservation = {
    enabled: true,
    policy: "all",
    known: true,
    in_use: 0,
    limit: 14,
    waiting: { known: false },
    observed_at: new Date().toISOString(),
  };
  it("retains a dated response only on transport failure, while unknown remains the latest value", () => {
    const first = retainCapacity(undefined, observation, "ready");
    const failed = retainCapacity(first, undefined, "error");
    expect(failed.capacity).toBe(observation);
    expect(failed.capacityReceivedAt).toBe(first.capacityReceivedAt);
    expect(failed.capacityStatus).toBe("error");
    const unknown = { ...observation, known: false, in_use: null };
    const latest = retainCapacity(failed, unknown, "ready");
    expect(latest.capacity).toBe(unknown);
    expect(latest.lastConfirmedCapacity).toBe(observation);
  });
  it("clears both the current response and historical value on lost capacity permission", () => {
    const first = retainCapacity(undefined, observation, "ready");
    const denied = retainCapacity(first, undefined, "denied");
    expect(denied.capacity).toBeUndefined();
    expect(denied.lastConfirmedCapacity).toBeUndefined();
  });
});
