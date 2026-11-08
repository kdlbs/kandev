import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import {
  citationComplete,
  citationSummary,
  noteFailureMessage,
  noteStatusLabel,
  noteTextLength,
  normalizedCitation,
} from "./notes";

const t = ((key: string) => key) as unknown as TFunction;

describe("note helpers", () => {
  it("counts code points of the trimmed text", () => {
    expect(noteTextLength("  ab  ")).toBe(2);
    expect(noteTextLength("😀😀")).toBe(2);
  });

  it("labels every status and failure code", () => {
    const statuses = [
      "deleted",
      "expired",
      "stale",
      "unavailable",
      "not_validated",
      "over_cap",
      "live",
    ] as const;
    expect(new Set(statuses.map((s) => noteStatusLabel(t, s))).size).toBe(statuses.length);
    const codes = [
      "invalid_type",
      "invalid_text_length",
      "invalid_citation",
      "citation_unresolved",
      "credential_detected",
      "note_limit_reached",
      "invalid_request",
      "unavailable",
      "conflict",
      "unknown",
    ] as const;
    for (const code of codes)
      expect(noteFailureMessage(t, { code })).toMatch(/^coordinator:noteError/);
  });

  it("summarizes and validates typed citations per kind", () => {
    expect(citationSummary({ kind: "pr", repository_id: "r", number: 4 })).toBe("r#4");
    expect(
      citationSummary({ kind: "file", repository_id: "r", path: "a.go", sha: "0123456789abcdef" }),
    ).toBe("r:a.go@0123456789ab");
    expect(citationComplete({ kind: "task", id: " " })).toBe(false);
    expect(citationComplete({ kind: "pr", repository_id: "r", number: 0 })).toBe(false);
    expect(citationComplete({ kind: "pr", repository_id: "r", number: 2 })).toBe(true);
    expect(citationComplete({ kind: "file", repository_id: "r", path: "p", sha: "" })).toBe(false);
  });

  it("drops fields foreign to the citation kind", () => {
    expect(normalizedCitation({ kind: "task", id: " t ", path: "x", number: 3 })).toEqual({
      kind: "task",
      id: "t",
    });
    expect(normalizedCitation({ kind: "pr", repository_id: " r ", number: 3, id: "x" })).toEqual({
      kind: "pr",
      repository_id: "r",
      number: 3,
    });
  });
});
