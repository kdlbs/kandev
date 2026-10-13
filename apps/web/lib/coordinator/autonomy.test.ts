import { describe, expect, it } from "vitest";
import type { AutonomyRead, TurnRead } from "@/lib/api/domains/coordinator-autonomy-api";
import {
  ceilingCents,
  formatSubcentsUsd,
  isStopFailing,
  isUsableAutonomy,
  nextDeadlineMs,
  normalizeCeiling,
  serverNowMs,
  wireMs,
} from "./autonomy";

const T0 = "2026-09-30T10:00:00Z";
const t0 = Date.parse(T0);

function turn(over: Partial<TurnRead> = {}): TurnRead {
  return {
    id: "t1",
    coordinator_id: "c1",
    conversation_task_id: "k1",
    session_id: "s1",
    started_at: T0,
    finished_at: null,
    outcome: null,
    wake_count: 1,
    denied_permissions: 0,
    cost_subcents: null,
    stop_requested_at: null,
    stop_state: null,
    ...over,
  };
}

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
      ceiling_subcents: 1_000_000,
    },
    ...over,
  };
}

describe("ceilingCents", () => {
  it("compares as integer cents so 10.5 equals 10.50", () => {
    expect(ceilingCents("10.5")).toBe(1050);
    expect(ceilingCents("10.50")).toBe(1050);
  });
  it("accepts the bounds and rejects outside them", () => {
    expect(ceilingCents("0.01")).toBe(1);
    expect(ceilingCents("10000.00")).toBe(1_000_000);
    expect(ceilingCents("0")).toBeNull();
    expect(ceilingCents("10000.01")).toBeNull();
  });
  it("rejects malformed text", () => {
    for (const bad of ["", "abc", "1.234", "-1", "1e3", ".5", "1,5"]) {
      expect(ceilingCents(bad)).toBeNull();
    }
  });
  it("normalizes to the two-place wire form", () => {
    expect(normalizeCeiling(" 10.5 ")).toBe("10.50");
    expect(normalizeCeiling("7")).toBe("7.00");
    expect(normalizeCeiling("x")).toBeNull();
  });
});

describe("formatSubcentsUsd", () => {
  it("renders subcents as two-place dollars", () => {
    expect(formatSubcentsUsd(1_040_000)).toBe("104.00");
    expect(formatSubcentsUsd(1234)).toBe("0.12");
  });
});

describe("isUsableAutonomy", () => {
  it("accepts a well-formed read", () => {
    expect(isUsableAutonomy(read())).toBe(true);
  });
  it("fails closed on malformed reads", () => {
    expect(isUsableAutonomy(null)).toBe(false);
    expect(isUsableAutonomy(read({ server_time: "nope" }))).toBe(false);
    expect(isUsableAutonomy({ ...read(), pending_wakes: -1 })).toBe(false);
    expect(isUsableAutonomy({ ...read(), spend: undefined } as unknown as AutonomyRead)).toBe(
      false,
    );
    expect(isUsableAutonomy({ ...read(), containment: {} } as unknown as AutonomyRead)).toBe(false);
  });
});

describe("serverNowMs", () => {
  it("adds the elapsed client time to server_time", () => {
    expect(serverNowMs(read(), 1000, 1000 + 45_000)).toBe(t0 + 45_000);
  });
  it("never moves backwards", () => {
    expect(serverNowMs(read(), 1000, 500)).toBe(t0);
  });
});

describe("nextDeadlineMs", () => {
  it("is stop_requested_at + 5 min + 1 s for an open turn not yet failing", () => {
    const requested = "2026-09-30T09:58:00Z";
    const v = read({ last_turn: turn({ stop_requested_at: requested }) });
    expect(nextDeadlineMs(v)).toBe(Date.parse(requested) + 301_000);
  });
  it("is null once the stop is already failing or the turn is settled", () => {
    const requested = "2026-09-30T09:50:00Z";
    expect(
      nextDeadlineMs(
        read({ last_turn: turn({ stop_requested_at: requested, stop_state: "stop_failing" }) }),
      ),
    ).toBeNull();
    expect(
      nextDeadlineMs(
        read({
          last_turn: turn({
            stop_requested_at: requested,
            finished_at: T0,
            outcome: "stopped_at_ceiling",
          }),
        }),
      ),
    ).toBeNull();
  });
  it("is until + 1 s for a cooldown", () => {
    const until = "2026-09-30T10:03:00Z";
    const v = read({ admission: { ok: false, reason: "cooldown", detail: "", until } });
    expect(nextDeadlineMs(v)).toBe(Date.parse(until) + 1000);
  });
  it("takes the earliest of both", () => {
    const until = "2026-09-30T10:03:00Z";
    const v = read({
      admission: { ok: false, reason: "cooldown", detail: "", until },
      last_turn: turn({ stop_requested_at: "2026-09-30T09:59:00Z" }),
    });
    expect(nextDeadlineMs(v)).toBe(Date.parse(until) + 1000);
  });
  it("ignores a cooldown with a malformed until", () => {
    const v = read({ admission: { ok: false, reason: "cooldown", detail: "", until: "x" } });
    expect(nextDeadlineMs(v)).toBeNull();
  });
});

describe("isStopFailing / wireMs", () => {
  it("is true only for an open turn flagged stop_failing", () => {
    expect(isStopFailing(read({ last_turn: turn({ stop_state: "stop_failing" }) }))).toBe(true);
    expect(isStopFailing(read({ last_turn: turn() }))).toBe(false);
    expect(isStopFailing(read())).toBe(false);
  });
  it("parses strictly", () => {
    expect(wireMs(T0)).toBe(t0);
    expect(wireMs("2026-02-30T00:00:00Z")).toBeNull();
    expect(wireMs(null)).toBeNull();
  });
});
