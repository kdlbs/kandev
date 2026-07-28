import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { TaskReviewFinding } from "@/lib/types/review";
import { ReviewFindingsOverview, groupOpenFindingsByFile } from "./review-findings-overview";

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
    severity: "minor",
    category: "correctness",
    title: "A finding",
    body: "body",
    suggestion: "",
    anchor_text: "",
    file_diff_hash: "h",
    status: "open",
    created_at: "2026-07-24T10:00:00Z",
    updated_at: "2026-07-24T10:00:00Z",
    ...overrides,
  };
}

afterEach(cleanup);

describe("groupOpenFindingsByFile", () => {
  it("drops non-open findings and groups by file with the most-severe file first", () => {
    const groups = groupOpenFindingsByFile([
      finding({ id: "a", file_path: "minor.ts", severity: "minor" }),
      finding({ id: "b", file_path: "blocker.ts", severity: "blocker" }),
      finding({ id: "c", file_path: "blocker.ts", severity: "major", start_line: 3 }),
      finding({ id: "d", file_path: "resolved.ts", status: "resolved" }),
    ]);
    expect(groups.map((g) => g.filePath)).toEqual(["blocker.ts", "minor.ts"]);
    // Within the blocker file, the blocker sorts before the major.
    expect(groups[0].findings.map((f) => f.id)).toEqual(["b", "c"]);
  });

  it("keeps same-named files in different repos separate", () => {
    const groups = groupOpenFindingsByFile([
      finding({ id: "a", repository_name: "web", file_path: "x.ts" }),
      finding({ id: "b", repository_name: "api", file_path: "x.ts" }),
    ]);
    expect(groups).toHaveLength(2);
    expect(groups.map((g) => g.filePath).sort()).toEqual(["api/x.ts", "web/x.ts"]);
  });
});

describe("ReviewFindingsOverview", () => {
  it("shows an empty state when there is nothing open", () => {
    render(
      <ReviewFindingsOverview findings={[finding({ status: "dismissed" })]} onNavigate={vi.fn()} />,
    );
    expect(screen.getByText("No open findings.")).toBeTruthy();
  });

  it("navigates to the clicked finding", () => {
    const onNavigate = vi.fn();
    const target = finding({ id: "clickme", title: "Click me" });
    render(<ReviewFindingsOverview findings={[target]} onNavigate={onNavigate} />);
    fireEvent.click(screen.getByText("Click me"));
    expect(onNavigate).toHaveBeenCalledWith(target);
  });

  it("summarizes the open count across files", () => {
    render(
      <ReviewFindingsOverview
        findings={[
          finding({ id: "a", file_path: "a.ts" }),
          finding({ id: "b", file_path: "b.ts" }),
        ]}
        onNavigate={vi.fn()}
      />,
    );
    expect(screen.getByText("2 open findings")).toBeTruthy();
    expect(screen.getByText("across 2 files")).toBeTruthy();
  });
});
