import { describe, expect, it } from "vitest";
import { canonicalStatusesToBackend, normalizeTaskStatus } from "./normalize-status";
import type { OfficeTaskStatus } from "@/lib/state/slices/office/types";

describe("normalizeTaskStatus", () => {
  it("returns 'backlog' for null/undefined/empty input", () => {
    expect(normalizeTaskStatus(undefined)).toBe("backlog");
    expect(normalizeTaskStatus(null)).toBe("backlog");
    expect(normalizeTaskStatus("")).toBe("backlog");
  });

  it("maps backend uppercase enums to canonical office statuses", () => {
    expect(normalizeTaskStatus("TODO")).toBe("todo");
    expect(normalizeTaskStatus("CREATED")).toBe("todo");
    expect(normalizeTaskStatus("SCHEDULING")).toBe("todo");
    expect(normalizeTaskStatus("IN_PROGRESS")).toBe("in_progress");
    expect(normalizeTaskStatus("WAITING_FOR_INPUT")).toBe("in_progress");
    expect(normalizeTaskStatus("REVIEW")).toBe("in_review");
    expect(normalizeTaskStatus("IN_REVIEW")).toBe("in_review");
    expect(normalizeTaskStatus("BLOCKED")).toBe("blocked");
    expect(normalizeTaskStatus("FAILED")).toBe("blocked");
    expect(normalizeTaskStatus("COMPLETED")).toBe("done");
    expect(normalizeTaskStatus("DONE")).toBe("done");
    expect(normalizeTaskStatus("CANCELLED")).toBe("cancelled");
    expect(normalizeTaskStatus("CANCELED")).toBe("cancelled");
    expect(normalizeTaskStatus("BACKLOG")).toBe("backlog");
  });

  it("accepts already-canonical lowercase office statuses", () => {
    const canonical: OfficeTaskStatus[] = [
      "todo",
      "in_progress",
      "in_review",
      "blocked",
      "done",
      "cancelled",
      "backlog",
    ];
    for (const s of canonical) {
      expect(normalizeTaskStatus(s)).toBe(s);
    }
  });

  it("falls back to 'backlog' for unknown statuses", () => {
    expect(normalizeTaskStatus("UNKNOWN_STATE")).toBe("backlog");
    expect(normalizeTaskStatus("garbage")).toBe("backlog");
  });
});

describe("canonicalStatusesToBackend", () => {
  it("returns an empty array for empty input", () => {
    expect(canonicalStatusesToBackend([])).toEqual([]);
  });

  it("expands each canonical status to all backend enum aliases", () => {
    expect(canonicalStatusesToBackend(["todo"])).toEqual(["TODO", "CREATED", "SCHEDULING"]);
    expect(canonicalStatusesToBackend(["in_progress"])).toEqual([
      "IN_PROGRESS",
      "WAITING_FOR_INPUT",
    ]);
    expect(canonicalStatusesToBackend(["in_review"])).toEqual(["REVIEW", "IN_REVIEW"]);
    expect(canonicalStatusesToBackend(["blocked"])).toEqual(["BLOCKED", "FAILED"]);
    expect(canonicalStatusesToBackend(["done"])).toEqual(["COMPLETED", "DONE"]);
    expect(canonicalStatusesToBackend(["cancelled"])).toEqual(["CANCELLED", "CANCELED"]);
    expect(canonicalStatusesToBackend(["backlog"])).toEqual(["BACKLOG"]);
  });

  it("concatenates expansions for multiple canonical statuses", () => {
    expect(canonicalStatusesToBackend(["todo", "done"])).toEqual([
      "TODO",
      "CREATED",
      "SCHEDULING",
      "COMPLETED",
      "DONE",
    ]);
  });

  it("round-trips: every backend value produced normalises back to the source canonical", () => {
    const all: OfficeTaskStatus[] = [
      "backlog",
      "todo",
      "in_progress",
      "in_review",
      "blocked",
      "done",
      "cancelled",
    ];
    for (const canonical of all) {
      const backendValues = canonicalStatusesToBackend([canonical]);
      expect(backendValues.length).toBeGreaterThan(0);
      for (const v of backendValues) {
        expect(normalizeTaskStatus(v)).toBe(canonical);
      }
    }
  });
});
