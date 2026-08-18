import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ChangeRequestList, ChangeRequestRow } from "./change-request-list";

afterEach(cleanup);

describe("ChangeRequestList", () => {
  it("renders its rows inside the native bordered list surface", () => {
    render(
      <ChangeRequestList loading={false} error={null} emptyMessage="No matches" isEmpty={false}>
        <div>row</div>
      </ChangeRequestList>,
    );

    expect(screen.getByText("row").parentElement?.className).toContain("divide-y");
    expect(screen.getByText("row").parentElement?.parentElement?.className).toContain("border");
  });

  it("renders the shared loading, error, and empty states", () => {
    const { rerender } = render(
      <ChangeRequestList loading error={null} emptyMessage="No matches" isEmpty={false}>
        rows
      </ChangeRequestList>,
    );
    expect(screen.getByRole("status")).toBeTruthy();

    rerender(
      <ChangeRequestList loading={false} error="Search failed" emptyMessage="No matches" isEmpty>
        rows
      </ChangeRequestList>,
    );
    expect(screen.getByText("Search failed").className).toContain("text-destructive");

    rerender(
      <ChangeRequestList loading={false} error={null} emptyMessage="No matches" isEmpty>
        rows
      </ChangeRequestList>,
    );
    expect(screen.getByText("No matches")).toBeTruthy();
  });
});

describe("ChangeRequestRow", () => {
  it("keeps the row inert while exposing an external title link and independent action", () => {
    render(
      <ChangeRequestRow
        stateIcon={<span>state</span>}
        title="Change title"
        href="https://example.com/change/7"
        metadata={<span>owner/repo#7</span>}
        taskIndicator={<button type="button">Linked task</button>}
        action={<button type="button">Task</button>}
        testId="change-row"
        dataAttributes={{ "data-change-id": 7 }}
      />,
    );

    const row = screen.getByTestId("change-row");
    expect(row.tagName).toBe("DIV");
    expect(row.getAttribute("data-change-id")).toBe("7");
    expect(row.getAttribute("role")).toBeNull();
    expect(screen.getByRole("link", { name: /Change title/ }).getAttribute("href")).toBe(
      "https://example.com/change/7",
    );
    expect(screen.getByRole("link").getAttribute("target")).toBe("_blank");
    expect(screen.getByRole("button", { name: "Task" })).toBeTruthy();
  });
});
