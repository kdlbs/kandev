import { describe, it, expect } from "vitest";
import {
  cleanLinearErrorMessage,
  extractLinearKey,
  isLinearAuthError,
  LINEAR_KEY_RE,
  priorityClass,
  stateBadgeClass,
} from "./linear-issue-common";

describe("LINEAR_KEY_RE / extractLinearKey", () => {
  it("matches uppercase TEAM-N identifiers anywhere in the string", () => {
    expect(extractLinearKey("ENG-123")).toBe("ENG-123");
    expect(extractLinearKey("[ABC-1] fix login")).toBe("ABC-1");
    expect(extractLinearKey("Migrate Z9-42 to new shape")).toBe("Z9-42");
  });

  it("rejects identifiers that don't start with a capital letter", () => {
    expect(extractLinearKey("eng-123")).toBeNull();
    expect(extractLinearKey("123-456")).toBeNull();
  });

  it("does not match version-style strings like v1-2", () => {
    expect(extractLinearKey("upgrade v1-2 to v1-3")).toBeNull();
  });

  it("returns null for nullish or empty input", () => {
    expect(extractLinearKey(null)).toBeNull();
    expect(extractLinearKey(undefined)).toBeNull();
    expect(extractLinearKey("")).toBeNull();
  });

  it("requires a word-boundary after the numeric tail", () => {
    // The regex is digit-greedy, so a trailing "999" is part of the key.
    expect("ENG-123999".match(LINEAR_KEY_RE)?.[0]).toBe("ENG-123999");
    // A trailing letter breaks the closing word boundary, so the whole
    // candidate is rejected rather than being truncated.
    expect(extractLinearKey("ENG-123abc")).toBeNull();
  });
});

describe("isLinearAuthError", () => {
  it("flags 401 and 403 as auth errors", () => {
    expect(isLinearAuthError("linear api: status 401: invalid api key")).toBe(true);
    expect(isLinearAuthError("linear api: status 403: forbidden")).toBe(true);
  });

  it("does not flag 400 / 404 / 500 as auth errors", () => {
    expect(isLinearAuthError("linear api: status 400: bad input")).toBe(false);
    expect(isLinearAuthError("linear api: status 404: not found")).toBe(false);
    expect(isLinearAuthError("linear api: status 500: server error")).toBe(false);
  });

  it("matches case-insensitively on the status keyword", () => {
    expect(isLinearAuthError("STATUS 401")).toBe(true);
  });
});

describe("cleanLinearErrorMessage", () => {
  it("strips URLs", () => {
    expect(cleanLinearErrorMessage("Error see https://example.com/docs/x for details")).toBe(
      "Error see for details",
    );
  });

  it("collapses runs of whitespace", () => {
    expect(cleanLinearErrorMessage("foo    bar\n\nbaz")).toBe("foo bar baz");
  });

  it("trims leading and trailing whitespace", () => {
    expect(cleanLinearErrorMessage("   hi   ")).toBe("hi");
  });
});

describe("stateBadgeClass", () => {
  it("returns the green palette for done", () => {
    expect(stateBadgeClass("done")).toContain("green");
  });

  it("returns the amber palette for indeterminate (in-progress)", () => {
    expect(stateBadgeClass("indeterminate")).toContain("amber");
  });

  it("returns the blue palette for new", () => {
    expect(stateBadgeClass("new")).toContain("blue");
  });

  it("returns an empty string for unknown / empty category", () => {
    expect(stateBadgeClass("")).toBe("");
    expect(stateBadgeClass(undefined)).toBe("");
  });
});

describe("priorityClass", () => {
  it("flags urgent, high, medium with distinct color tones", () => {
    expect(priorityClass(1)).toContain("red");
    expect(priorityClass(2)).toContain("orange");
    expect(priorityClass(3)).toContain("amber");
  });

  it("falls back to muted for low (4), none (0), and undefined", () => {
    expect(priorityClass(4)).toContain("muted");
    expect(priorityClass(0)).toContain("muted");
    expect(priorityClass(undefined)).toContain("muted");
  });
});
