import { describe, expect, it } from "vitest";
import type { EligibilitySettings } from "@/lib/api/domains/coordinator-api";
import {
  buildPutRequest,
  isEligibilityInvalid,
  isEligibilityValueValid,
  isPolicyDirty,
  mergeStored,
  settleAfterSave,
  type ControlDraft,
} from "./control-draft";

const defaults: EligibilitySettings = {
  evidence_window_hours: 720,
  min_decisions: 20,
  unedited_floor_percent: 90,
  review_max_age_hours: 168,
  daily_limit: 10,
};

const draft = (eligibility: EligibilitySettings | null): ControlDraft => ({
  eligibility,
  actions: {
    create_task: "requires_approval",
    start_agent: "denied",
    message: "denied",
    move: "denied",
    resume: "denied",
    stop: "denied",
  },
  watches: { scope: "all", workflowIds: [] },
  projects: null,
});

describe("eligibility bounds", () => {
  it("accepts the edges and refuses outside values and non-integers", () => {
    expect(isEligibilityValueValid("daily_limit", 1)).toBe(true);
    expect(isEligibilityValueValid("daily_limit", 100)).toBe(true);
    expect(isEligibilityValueValid("daily_limit", 0)).toBe(false);
    expect(isEligibilityValueValid("daily_limit", 101)).toBe(false);
    expect(isEligibilityValueValid("min_decisions", 2)).toBe(false);
    expect(isEligibilityValueValid("evidence_window_hours", 24.5)).toBe(false);
    expect(isEligibilityValueValid("unedited_floor_percent", Number.NaN)).toBe(false);
  });

  it("blocks a dirty draft holding an out-of-bounds value only", () => {
    const stored = draft(defaults);
    expect(isEligibilityInvalid(stored, stored)).toBe(false);
    expect(isEligibilityInvalid(draft({ ...defaults, daily_limit: 0 }), stored)).toBe(true);
    expect(isEligibilityInvalid(draft({ ...defaults, daily_limit: 5 }), stored)).toBe(false);
    expect(isEligibilityInvalid(draft(null), draft(null))).toBe(false);
  });
});

describe("eligibility in the PUT request", () => {
  it("omits eligibility when only actions changed", () => {
    const stored = draft(defaults);
    const next = {
      ...draft(defaults),
      actions: { ...stored.actions, resume: "requires_approval" as const },
    };
    expect(buildPutRequest(next, stored).policy?.eligibility).toBeUndefined();
  });

  it("sends eligibility alone with the unchanged actions when only it changed", () => {
    const stored = draft(defaults);
    const next = draft({ ...defaults, daily_limit: 4 });
    expect(isPolicyDirty(next, stored)).toBe(true);
    const request = buildPutRequest(next, stored);
    expect(request.policy?.eligibility?.daily_limit).toBe(4);
    expect(request.policy?.actions).toEqual(stored.actions);
  });

  it("never sends eligibility while the stored read carried none", () => {
    expect(buildPutRequest(draft(null), draft(null))).toEqual({});
  });
});

describe("eligibility merge and settle", () => {
  it("keeps an edited eligibility over a re-read and follows it otherwise", () => {
    const old = draft(defaults);
    const reread = draft({ ...defaults, min_decisions: 50 });
    const edited = draft({ ...defaults, daily_limit: 4 });
    expect(mergeStored(edited, old, reread).eligibility?.daily_limit).toBe(4);
    expect(mergeStored(old, old, reread).eligibility?.min_decisions).toBe(50);
  });

  it("keeps an edit made while the save was in flight", () => {
    const sent = draft({ ...defaults, daily_limit: 4 });
    const current = draft({ ...defaults, daily_limit: 6 });
    const response = draft({ ...defaults, daily_limit: 4 });
    expect(settleAfterSave(current, sent, response).eligibility?.daily_limit).toBe(6);
    expect(settleAfterSave(sent, sent, response).eligibility?.daily_limit).toBe(4);
  });
});
