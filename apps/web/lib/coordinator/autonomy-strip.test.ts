import { describe, expect, it } from "vitest";
import type { AutonomyRead } from "@/lib/api/domains/coordinator-autonomy-api";
import { lastWokeAgeMs, spendView, stripState, stripVisible } from "./autonomy-strip";

const T0 = "2026-09-30T10:00:00Z";
const t0 = Date.parse(T0);

function read(over: Partial<AutonomyRead> = {}): AutonomyRead {
  return {
    server_time: T0,
    autonomy_enabled: true,
    admission: { ok: true, detail: "" },
    pending_wakes: 0,
    oldest_pending_at: null,
    last_woke_at: null,
    last_turn: null,
    containment: { conditions: [] },
    spend: {
      measurable: true,
      degraded: false,
      window_subcents: 0,
      mean_daily_subcents_7d: 0,
      mean_known: true,
      ceiling_subcents: 1000,
    },
    ...over,
  };
}

describe("stripState", () => {
  it("is off when autonomy is off, whatever admission says", () => {
    expect(stripState(read({ autonomy_enabled: false, admission: undefined })).kind).toBe("off");
  });
  it("is active when admission is ok", () => {
    expect(stripState(read()).kind).toBe("active");
  });
  it("maps the transient reasons to active variants", () => {
    const busy = read({ admission: { ok: false, reason: "conversation_busy", detail: "" } });
    expect(stripState(busy).kind).toBe("busy");
    const until = "2026-09-30T10:03:00Z";
    const cool = read({ admission: { ok: false, reason: "cooldown", detail: "", until } });
    expect(stripState(cool)).toEqual({ kind: "cooldown", untilMs: Date.parse(until) });
  });
  it("maps each persistent reason to held with its detail", () => {
    const v = read({ admission: { ok: false, reason: "containment", detail: "auth_enabled" } });
    expect(stripState(v)).toEqual({ kind: "held", reason: "containment", detail: "auth_enabled" });
  });
  it("fails closed: an unknown reason, a missing admission or a cooldown without until is unknown", () => {
    expect(
      stripState(read({ admission: { ok: false, reason: "brand_new", detail: "" } })).kind,
    ).toBe("unknown");
    expect(stripState(read({ admission: undefined })).kind).toBe("unknown");
    expect(
      stripState(read({ admission: { ok: false, reason: "cooldown", detail: "" } })).kind,
    ).toBe("unknown");
    expect(
      stripState(read({ admission: { ok: false, reason: "autonomy_off", detail: "" } })).kind,
    ).toBe("unknown");
  });
});

describe("stripVisible", () => {
  const failing = {
    id: "t",
    coordinator_id: "c",
    conversation_task_id: "k",
    session_id: "s",
    started_at: T0,
    finished_at: null,
    outcome: null,
    wake_count: 1,
    denied_permissions: 0,
    cost_subcents: null,
    stop_requested_at: T0,
    stop_state: "stop_failing" as const,
  };
  it("shows when on, or when off with a failing stop; hides when off", () => {
    expect(stripVisible(read())).toBe(true);
    expect(stripVisible(read({ autonomy_enabled: false }))).toBe(false);
    expect(stripVisible(read({ autonomy_enabled: false, last_turn: failing }))).toBe(true);
  });
});

describe("lastWokeAgeMs", () => {
  it("adds elapsed client time to the server-side age", () => {
    const v = read({ last_woke_at: "2026-09-30T08:00:00Z" });
    expect(lastWokeAgeMs(v, 1000, 1000 + 60_000)).toBe(2 * 3_600_000 + 60_000);
  });
  it("is null when never woken or malformed", () => {
    expect(lastWokeAgeMs(read(), 0, t0)).toBeNull();
    expect(lastWokeAgeMs(read({ last_woke_at: "x" }), 0, t0)).toBeNull();
  });
});

describe("spendView", () => {
  const base = read().spend;
  it("is Over at exactly the ceiling and Inside below it", () => {
    expect(spendView({ ...base, window_subcents: 1000 })).toMatchObject({ over: true });
    expect(spendView({ ...base, window_subcents: 999 })).toMatchObject({ over: false });
  });
  it("has no ceiling form when the ceiling is null", () => {
    expect(spendView({ ...base, ceiling_subcents: null, window_subcents: 5 })).toEqual({
      kind: "no-ceiling",
      windowSubcents: 5,
    });
  });
  it("is unmeasurable when not measurable, distinguishing degraded", () => {
    expect(
      spendView({ ...base, measurable: false, degraded: true, window_subcents: null }),
    ).toEqual({ kind: "unmeasurable", degraded: true });
    expect(spendView({ ...base, measurable: false, window_subcents: null })).toEqual({
      kind: "unmeasurable",
      degraded: false,
    });
  });
  it("treats a measurable read without a window as unmeasurable", () => {
    expect(spendView({ ...base, window_subcents: null })).toMatchObject({ kind: "unmeasurable" });
  });
});
