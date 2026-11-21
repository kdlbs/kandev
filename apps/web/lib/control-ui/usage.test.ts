import { describe, expect, it } from "vitest";
import { groupExecutionUsage, observedLimitState, latestModelUsage } from "./usage";
import type { ProfileExecutionUsage } from "@/lib/api/domains/provider-usage-api";

const row = (id: string, account = "a", at = "2026-10-18T12:00:00Z"): ProfileExecutionUsage => ({
  execution_id: id,
  session_id: `session-${id}`,
  report: {
    source_known: true,
    reason: "",
    model: "sol",
    limited: false,
    current: { provider: "openai", account_ref: account, fetched_at: at, windows: [] },
  },
});
describe("provider account observation grouping", () => {
  it("deduplicates executions and groups one account across resolved profiles", () => {
    const a = row("one");
    expect(groupExecutionUsage([a, a, row("two")])).toMatchObject([
      { provider: "openai", account: "a", entries: [a, row("two")] },
    ]);
  });
  it("keeps provider, account and organization boundaries", () => {
    const a = row("one");
    const b = row("two", "b");
    const c = row("three");
    c.report.current!.organization_ref = "other";
    expect(groupExecutionUsage([a, b, c])).toHaveLength(3);
  });
  it("does not combine unproven account bindings", () => {
    const a = row("one");
    const b = row("two");
    a.report.source_known = b.report.source_known = false;
    expect(groupExecutionUsage([a, b])).toHaveLength(2);
    expect(groupExecutionUsage([a])[0].sample).toBeUndefined();
  });
  it("chooses a whole newest sample including measured zero", () => {
    const a = row("one");
    const b = row("two", "a", "2026-10-18T13:00:00Z");
    b.report.current!.windows = [
      {
        label: "5h",
        utilization_pct: 0,
        utilization_observed: true,
        reset_at: "",
        reset_at_observed: false,
      },
    ];
    expect(groupExecutionUsage([a, b])[0].sample).toEqual(b.report.current);
  });
});
describe("saved limit time boundaries", () => {
  const now = Date.parse("2026-10-18T14:30:00Z");
  it("keeps unknown source unknown even when limited is false", () => {
    expect(observedLimitState({ ...row("one").report, source_known: false }, now)).toBe("unknown");
  });
  it("does not call an expired saved block current", () => {
    const report = { ...row("one").report, limited: true, reset_at: "2026-10-18T14:30:00Z" };
    expect(observedLimitState(report, now)).toBe("expired");
    expect(observedLimitState({ ...report, reset_at: "2026-10-18T14:30:01Z" }, now)).toBe(
      "limited",
    );
  });
  it("keeps a block without a reset visible and does not invent a deadline", () => {
    expect(observedLimitState({ ...row("one").report, limited: true }, now)).toBe("limited");
  });
});

it("shows one latest saved report per model instead of stale block history", () => {
  const old = row("old");
  old.report.limited = true;
  const current = row("new", "a", "2026-10-18T13:00:00Z");
  expect(latestModelUsage([old, current])).toEqual([current]);
});
