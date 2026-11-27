import { describe, expect, it } from "vitest";
import type { UsageTurn } from "@/lib/types/conversation-usage";
import {
  formatUsageCost,
  formatUsageTokens,
  latestUsageTurn,
  usageDetailForTurn,
} from "./conversation-usage";

describe("conversation usage formatting", () => {
  it("formats token counts and int64 subcent prices without float conversion", () => {
    expect(formatUsageTokens(1200)).toBe("1,200");
    expect(formatUsageCost("150000")).toBe("$15.00");
    expect(formatUsageCost("9223372036854775807")).toBe("$922,337,203,685,477.58");
    expect(formatUsageCost("0")).toBe("$0.00");
  });

  it("returns unavailable for missing or malformed prices", () => {
    expect(formatUsageCost(undefined)).toBeNull();
    expect(formatUsageCost("not-a-number")).toBeNull();
  });

  it("uses the first turn from the newest-first API page", () => {
    const turns = [{ turn_id: "new" }, { turn_id: "old" }] as UsageTurn[];
    expect(latestUsageTurn(turns)?.turn_id).toBe("new");
    expect(latestUsageTurn([])).toBeUndefined();
  });

  it("uses detail only when it belongs to the refreshed latest turn", () => {
    const latest = { turn_id: "new" } as UsageTurn;
    const stale = {
      turnId: "old",
      turn: { turn_id: "old" } as UsageTurn,
      loading: false,
    };
    expect(usageDetailForTurn(latest, stale)).toBeNull();

    const loading = { turnId: "new", turn: null, loading: true };
    expect(usageDetailForTurn(latest, loading)).toBe(loading);

    const mismatchedResponse = {
      turnId: "new",
      turn: { turn_id: "old" } as UsageTurn,
      loading: false,
    };
    expect(usageDetailForTurn(latest, mismatchedResponse)).toBeNull();
  });
});
