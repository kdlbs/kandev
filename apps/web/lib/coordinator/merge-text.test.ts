import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import {
  conflictTri,
  countText,
  isRefusalCode,
  mergeNoteText,
  mergeReasonText,
  mergeStatusText,
  parseChangeKey,
  shortSha,
} from "./merge-text";

const t = ((key: string, params?: Record<string, unknown>) =>
  params ? `${key}|${JSON.stringify(params)}` : key) as unknown as TFunction;

describe("parseChangeKey", () => {
  it("parses the four-part key", () => {
    expect(parseChangeKey("github|github.com|r1|42")).toEqual({
      provider: "github",
      host: "github.com",
      repositoryId: "r1",
      number: 42,
    });
  });
  it("rejects other shapes", () => {
    expect(parseChangeKey("a|b|c")).toBeNull();
    expect(parseChangeKey("a|b|c|x")).toBeNull();
    expect(parseChangeKey("a|b|c|0")).toBeNull();
  });
});

describe("texts", () => {
  it("shortens a sha", () => {
    expect(shortSha("0123456789")).toBe("0123456");
  });

  it("maps reasons, unavailable conditions and unknown codes", () => {
    expect(mergeReasonText(null, t)).toBeNull();
    expect(mergeReasonText("stale_head", t)).toBe("coordinator:mergeReasonStaleHead");
    expect(mergeReasonText("unavailable:pipeline", t)).toBe(
      'coordinator:mergeReasonUnavailable|{"condition":"coordinator:mergeCondPipeline"}',
    );
    expect(mergeReasonText("unavailable:zzz", t)).toContain("coordinator:mergeCondUnknown");
    expect(mergeReasonText("zzz", t)).toBe("coordinator:mergeReasonOther");
  });

  it("maps manager notes", () => {
    expect(mergeNoteText("Daily limit reached", t)).toBe("coordinator:mergeNoteDailyLimit");
    expect(mergeNoteText("Lowering automatic merge policy is pending", t)).toBe(
      "coordinator:mergeNoteLowering",
    );
    expect(mergeNoteText("something else", t)).toBe("coordinator:mergeNoteOther");
    expect(mergeNoteText(undefined, t)).toBeNull();
  });

  it("never renders a missing count as zero", () => {
    expect(countText("coordinator:mergeValBehind", null, t)).toBe(
      "coordinator:mergeValUnavailable",
    );
    expect(countText("coordinator:mergeValBehind", 0, t)).toContain('"count":0');
  });

  it("normalises the conflict reading", () => {
    expect(conflictTri("clear")).toBe("met");
    expect(conflictTri("present")).toBe("unmet");
    expect(conflictTri("unknown")).toBe("unavailable");
  });
});

describe("mergeStatusText", () => {
  it("tells a refusal from a manager's rejection", () => {
    expect(isRefusalCode("stale_head")).toBe(true);
    expect(isRefusalCode("unavailable:pipeline")).toBe(true);
    expect(isRefusalCode("not good enough")).toBe(false);
    expect(mergeStatusText({ status: "rejected", reject_reason: "draft" }, t)).toContain(
      "mergeStatusRefused",
    );
    expect(mergeStatusText({ status: "rejected", reject_reason: "no thanks" }, t)).toBe(
      "coordinator:mergeStatusRejected",
    );
  });

  it("covers the open and settled statuses", () => {
    const base = { reject_reason: null };
    expect(mergeStatusText({ ...base, status: "uncertain" }, t)).toBe(
      "coordinator:mergeStatusUncertain",
    );
    expect(mergeStatusText({ ...base, status: "approved" }, t)).toBe(
      "coordinator:mergeStatusMerged",
    );
    expect(mergeStatusText({ ...base, status: "pending", merge: { manager_only: true } }, t)).toBe(
      "coordinator:mergeStatusManagerOnly",
    );
    expect(mergeStatusText({ ...base, status: "pending" }, t)).toBe(
      "coordinator:mergeStatusPending",
    );
  });
});
