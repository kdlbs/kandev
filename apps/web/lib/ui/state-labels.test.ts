import { describe, it, expect } from "vitest";
import { formatTaskStateLabel, formatTaskSessionStateLabel } from "./state-labels";
import type { TaskState, TaskSessionState } from "@/lib/types/http";

describe("formatTaskStateLabel", () => {
  it("maps known task states to human labels", () => {
    expect(formatTaskStateLabel("IN_PROGRESS")).toBe("In progress");
    expect(formatTaskStateLabel("WAITING_FOR_INPUT")).toBe("Waiting for input");
    expect(formatTaskStateLabel("TODO")).toBe("To do");
    expect(formatTaskStateLabel("COMPLETED")).toBe("Completed");
    expect(formatTaskStateLabel("FAILED")).toBe("Failed");
    expect(formatTaskStateLabel("CANCELLED")).toBe("Cancelled");
    expect(formatTaskStateLabel("BLOCKED")).toBe("Blocked");
    expect(formatTaskStateLabel("REVIEW")).toBe("Review");
    expect(formatTaskStateLabel("CREATED")).toBe("Created");
    expect(formatTaskStateLabel("SCHEDULING")).toBe("Scheduling");
  });

  it("returns 'Not started' for null/undefined", () => {
    expect(formatTaskStateLabel(null)).toBe("Not started");
    expect(formatTaskStateLabel(undefined)).toBe("Not started");
  });

  it("falls back to the raw value for unknown states", () => {
    expect(formatTaskStateLabel("UNKNOWN_FUTURE" as TaskState)).toBe("UNKNOWN_FUTURE");
  });
});

describe("formatTaskSessionStateLabel", () => {
  it("maps known session states", () => {
    expect(formatTaskSessionStateLabel("RUNNING")).toBe("Running");
    expect(formatTaskSessionStateLabel("STARTING")).toBe("Starting");
    expect(formatTaskSessionStateLabel("WAITING_FOR_INPUT")).toBe("Waiting for input");
    expect(formatTaskSessionStateLabel("COMPLETED")).toBe("Completed");
    expect(formatTaskSessionStateLabel("FAILED")).toBe("Failed");
    expect(formatTaskSessionStateLabel("CANCELLED")).toBe("Cancelled");
    expect(formatTaskSessionStateLabel("CREATED")).toBe("Created");
  });

  it("returns empty string for null/undefined", () => {
    expect(formatTaskSessionStateLabel(null)).toBe("");
    expect(formatTaskSessionStateLabel(undefined)).toBe("");
  });

  it("falls back to the raw value for unknown states", () => {
    expect(formatTaskSessionStateLabel("UNKNOWN" as TaskSessionState)).toBe("UNKNOWN");
  });
});
