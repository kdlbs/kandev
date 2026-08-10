import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";

import { BUDGET_ACTION_LABEL_KEYS, BUDGET_PERIOD_LABEL_KEYS } from "./label-keys";

/**
 * Both maps back a `?? raw wire value` fallback at the call site rather than a
 * `?? someOtherKey` one. An earlier version defaulted an unmapped
 * `actionOnExceed` to "notify only", which reports the mildest possible
 * behaviour for an action the server may treat as blocking — a wrong label is
 * worse here than an unfamiliar identifier.
 */
describe("budget label maps", () => {
  it("covers every wire value the BudgetPolicy type allows", () => {
    expect(Object.keys(BUDGET_ACTION_LABEL_KEYS).sort()).toEqual([
      "block_new_tasks",
      "notify_only",
      "pause_agent",
    ]);
    expect(Object.keys(BUDGET_PERIOD_LABEL_KEYS).sort()).toEqual(["monthly", "total"]);
  });

  it("resolves every mapped key to real copy", () => {
    for (const key of [
      ...Object.values(BUDGET_ACTION_LABEL_KEYS),
      ...Object.values(BUDGET_PERIOD_LABEL_KEYS),
    ]) {
      const value = t(key);
      expect(value).not.toBe(key);
      expect(value).not.toContain(":");
    }
  });

  it("has no entry for an unknown action, so the caller falls back to the raw value", () => {
    expect(BUDGET_ACTION_LABEL_KEYS["freeze_workspace"]).toBeUndefined();
    expect(BUDGET_PERIOD_LABEL_KEYS["weekly"]).toBeUndefined();
  });
});
