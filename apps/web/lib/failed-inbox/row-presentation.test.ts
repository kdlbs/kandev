import { describe, expect, it } from "vitest";
import { failedInboxOriginMarkerKey, hasResolvableFailedInboxReason } from "./row-presentation";

// AC-UI-INBOX-FAILED-001.9a: `manual` is the only origin treated as a
// person and renders no marker; each of the five non-person origins gets its
// own distinct marker; anything else (including absent/empty) either takes
// the `manual` path (no marker) or, if non-empty and unrecognised, the
// generic marker.
describe("failedInboxOriginMarkerKey", () => {
  it("renders no marker for the manual (person) origin", () => {
    expect(failedInboxOriginMarkerKey("manual")).toBeNull();
  });

  it("renders no marker for an absent or empty origin", () => {
    expect(failedInboxOriginMarkerKey("")).toBeNull();
    expect(failedInboxOriginMarkerKey(undefined)).toBeNull();
  });

  it("gives each of the five non-person origins its own distinct marker key", () => {
    const keys = [
      "agent_created",
      "routine",
      "onboarding",
      "automation_run",
      "automation_task",
    ].map((origin) => failedInboxOriginMarkerKey(origin));
    expect(new Set(keys).size).toBe(5);
    for (const key of keys) expect(key).toBeTruthy();
  });

  it("renders the generic marker for a non-empty, unenumerated origin", () => {
    expect(failedInboxOriginMarkerKey("some_future_origin")).toBe("failedInbox:originGeneric");
  });
});

describe("hasResolvableFailedInboxReason", () => {
  it("is false for an empty string", () => {
    expect(hasResolvableFailedInboxReason("")).toBe(false);
  });

  it("is false for a whitespace-only string", () => {
    expect(hasResolvableFailedInboxReason("   \n\t ")).toBe(false);
  });

  it("is true for a non-blank reason", () => {
    expect(hasResolvableFailedInboxReason("connection refused")).toBe(true);
  });
});
