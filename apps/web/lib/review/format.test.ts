import { describe, expect, it } from "vitest";
import type { TaskReviewFinding } from "@/lib/types/review";
import { findingLocation, formatFindingAsMarkdown, severityLabel } from "./format";

function finding(overrides: Partial<TaskReviewFinding> = {}): TaskReviewFinding {
  return {
    id: "f1",
    run_id: "r1",
    task_id: "t1",
    repository_id: "",
    repository_name: "",
    file_path: "apps/web/a.ts",
    start_line: 12,
    end_line: 12,
    side: "additions",
    severity: "blocker",
    category: "correctness",
    title: "Nil dereference",
    body: "`x` can be nil here.",
    suggestion: "",
    anchor_text: "",
    file_diff_hash: "h",
    status: "open",
    created_at: "2026-07-24T10:00:00Z",
    updated_at: "2026-07-24T10:00:00Z",
    ...overrides,
  };
}

describe("findingLocation", () => {
  it("renders a single line", () => {
    expect(findingLocation(finding())).toBe("apps/web/a.ts:12");
  });

  it("renders a range", () => {
    expect(findingLocation(finding({ end_line: 15 }))).toBe("apps/web/a.ts:12-15");
  });

  it("qualifies the path with the repository in a multi-repository task", () => {
    expect(findingLocation(finding({ repository_name: "backend" }))).toBe(
      "backend/apps/web/a.ts:12",
    );
  });
});

describe("severityLabel", () => {
  it("labels known severities", () => {
    expect(severityLabel("blocker")).toBe("Blocker");
    expect(severityLabel("nit")).toBe("Nit");
  });

  it("falls back to the raw value", () => {
    expect(severityLabel("mystery")).toBe("mystery");
  });
});

describe("formatFindingAsMarkdown", () => {
  it("includes the location, severity, title, and body", () => {
    const markdown = formatFindingAsMarkdown(finding());
    expect(markdown).toContain("apps/web/a.ts:12");
    expect(markdown).toContain("Blocker");
    expect(markdown).toContain("Nil dereference");
    expect(markdown).toContain("`x` can be nil here.");
    expect(markdown).toContain("Category: correctness");
  });

  it("marks a suggestion as not applied", () => {
    // The agent must not assume the fix is already in the working tree.
    const markdown = formatFindingAsMarkdown(finding({ suggestion: "if x != nil {" }));
    expect(markdown).toContain("not applied");
    expect(markdown).toContain("if x != nil {");
  });

  it("omits the suggestion block when there is none", () => {
    expect(formatFindingAsMarkdown(finding())).not.toContain("not applied");
  });

  it("omits the category line when the category is empty", () => {
    expect(formatFindingAsMarkdown(finding({ category: "" }))).not.toContain("Category:");
  });
});
