import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { mergeTaskSession } from "./session-merge";

function session(metadata: Record<string, unknown>): TaskSession {
  return {
    id: "session",
    task_id: "task",
    state: "RUNNING",
    started_at: "2026-11-06T00:00:00Z",
    updated_at: "2026-11-06T00:00:00Z",
    metadata,
  } as TaskSession;
}

describe("compaction metadata freshness", () => {
  it("keeps a newer lifetime count when a deferred snapshot arrives", () => {
    const merged = mergeTaskSession(
      session({ context_compaction_count: 5 }),
      session({ context_compaction_count: 2 }),
    );
    expect(merged.metadata?.context_compaction_count).toBe(5);
  });

  it("keeps newer reporting and sticky partial coverage", () => {
    const merged = mergeTaskSession(
      session({
        context_compaction_count: 5,
        context_compaction_reporting: {
          source: "copilot_native",
          availability: "available",
          coverage: "partial",
          revision: 4,
        },
      }),
      session({
        context_compaction_count: 3,
        context_compaction_reporting: {
          source: "copilot_native",
          availability: "available",
          coverage: "observed",
          revision: 3,
        },
      }),
    );
    expect(merged.metadata?.context_compaction_count).toBe(5);
    expect(merged.metadata?.context_compaction_reporting).toEqual(
      expect.objectContaining({ revision: 4, coverage: "partial" }),
    );
  });
});

describe("compaction metadata clear", () => {
  it("keeps the retained count when an older snapshot omits the gauge", () => {
    const merged = mergeTaskSession(
      session({
        context_compaction_count: 4,
        context_compaction_reporting: {
          source: "copilot_native",
          availability: "available",
          coverage: "partial",
          revision: 5,
        },
      }),
      session({ context_compaction_count: 1 }),
    );
    expect(merged.metadata?.context_compaction_count).toBe(4);
    expect(merged.metadata?.context_compaction_reporting).toEqual(
      expect.objectContaining({ revision: 5 }),
    );
  });
});
