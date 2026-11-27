import { describe, expect, it } from "vitest";
import { SharedResourceReads } from "./shared-resource-reads";

describe("resource invalidation with retained consumers", () => {
  it("rejects old snapshots while keeping unrelated resource reads alive", async () => {
    const reads = new SharedResourceReads<{ id: string }>();
    const release = reads.retain("task");
    const releaseOther = reads.retain("other-task");
    const old = Promise.withResolvers<{ id: string }>();
    const other = Promise.withResolvers<{ id: string }>();
    const retired = Promise.allSettled([reads.read("task", () => old.promise)]);
    const retained = reads.read("other-task", () => other.promise);
    reads.invalidate("task");
    const fresh = reads.read("task", async () => ({ id: "fresh" }));
    old.resolve({ id: "obsolete" });
    other.resolve({ id: "retained" });
    const [oldResult] = await retired;
    expect(oldResult).toMatchObject({ status: "rejected", reason: { name: "AbortError" } });
    expect((await fresh).id).toBe("fresh");
    expect((await retained).id).toBe("retained");
    release();
    releaseOther();
  });

  it("does not let an old queued refresh consume a newer generation's refresh", async () => {
    const reads = new SharedResourceReads<{ id: string }>();
    const release = reads.retain("task");
    const old = Promise.withResolvers<{ id: string }>();
    const fresh = Promise.withResolvers<{ id: string }>();
    const retired = Promise.allSettled([
      reads.read("task", () => old.promise),
      reads.read("task", async () => ({ id: "obsolete-refresh" }), { refresh: true }),
    ]);
    reads.invalidate("task");
    const admitted = Promise.all([
      reads.read("task", () => fresh.promise),
      reads.read("task", async () => ({ id: "newest" }), { refresh: true }),
    ]);
    old.resolve({ id: "obsolete" });
    for (const result of await retired) {
      expect(result).toMatchObject({ status: "rejected", reason: { name: "AbortError" } });
    }
    fresh.resolve({ id: "fresh" });
    expect((await admitted).map((value) => value.id)).toEqual(["fresh", "newest"]);
    release();
  });
});
