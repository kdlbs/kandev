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
    expect(rows.className).toContain("grid-cols-[max-content_auto_minmax(0,1fr)]");
    expect(screen.getByTestId("pr-task-status-review-value")).toBeTruthy();
    expect(screen.getByTestId("pr-task-status-ci-detail")).toBeTruthy();
  });
});
