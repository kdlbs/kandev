import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ChangeRequestTaskStatusSummary } from "./change-request-task-status-summary";

describe("ChangeRequestTaskStatusSummary layout", () => {
  it("aligns labels, status icons, and values on one shared grid", () => {
    render(
      <ChangeRequestTaskStatusSummary
        summaries={[
          {
            number: 42,
            title: "Align review details",
            rows: [
              { kind: "review", status: "approved", tone: "success" },
              {
                kind: "ci",
                status: "passed",
                tone: "success",
                detail: { key: "github:checksPassed", values: { count: 3 } },
              },
              { kind: "merge", status: "mergeable", tone: "success" },
            ],
          },
        ]}
      />,
    );

    const rows = screen.getByTestId("pr-task-status-rows");
    expect(rows.className).toContain("grid-cols-[minmax(0,max-content)_auto_minmax(0,1fr)]");
    expect(screen.getByTestId("pr-task-status-review-value")).toBeTruthy();
    expect(screen.getByTestId("pr-task-status-ci-detail")).toBeTruthy();
  });

  it("keeps translated labels shrinkable when the value column is narrow", () => {
    render(
      <ChangeRequestTaskStatusSummary
        summaries={[
          {
            number: 7,
            title: "Narrow summary",
            rows: [{ kind: "review", status: "approved", tone: "success" }],
          },
        ]}
      />,
    );

    const label = screen.getAllByText("Review").at(-1)!;
    expect(label.className).toContain("min-w-0");
    expect(label.className).toContain("[overflow-wrap:anywhere]");
  });
});
