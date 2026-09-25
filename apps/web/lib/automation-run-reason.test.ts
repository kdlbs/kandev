import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import { buildRunOutcomeReasonSuffix } from "./automation-run-reason";
import type { AutomationRun } from "@/lib/types/automation";

// A stand-in translator that returns the key (plus any interpolated value)
// rather than real copy — these tests assert dispatch and ordering, not the
// English strings, which the locale catalogs own.
const fakeT = ((key: string, options?: { value?: string }) =>
  options?.value !== undefined ? `${key}:${options.value}` : key) as TFunction;

function mkRun(overrides: Partial<AutomationRun> = {}): AutomationRun {
  return {
    id: "run-1",
    automation_id: "auto-1",
    trigger_id: "trig-1",
    trigger_type: "webhook",
    task_id: "",
    status: "triggered",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("buildRunOutcomeReasonSuffix", () => {
  it("returns empty when neither reason is set", () => {
    expect(buildRunOutcomeReasonSuffix(fakeT, mkRun())).toBe("");
  });

  it("excludes dedup_not_configured", () => {
    expect(
      buildRunOutcomeReasonSuffix(fakeT, mkRun({ dedup_reason: "dedup_not_configured" })),
    ).toBe("");
  });

  it("renders the repository reason before the dedup reason", () => {
    const suffix = buildRunOutcomeReasonSuffix(
      fakeT,
      mkRun({
        repository_reason: "repository_none_configured",
        dedup_reason: "dedup_unresolved",
      }),
    );
    expect(suffix).toBe(
      "automations:runReasonRepositoryNoneConfigured · automations:runReasonDedupUnresolved",
    );
  });

  it("interpolates the value for a value-bearing token", () => {
    const suffix = buildRunOutcomeReasonSuffix(
      fakeT,
      mkRun({ repository_reason: "selector_no_match: acme/other" }),
    );
    expect(suffix).toBe("automations:runReasonSelectorNoMatch:acme/other");
  });

  it("passes an unrecognized token through verbatim rather than dropping it", () => {
    const suffix = buildRunOutcomeReasonSuffix(fakeT, mkRun({ repository_reason: "future_token" }));
    expect(suffix).toBe("future_token");
  });
});
