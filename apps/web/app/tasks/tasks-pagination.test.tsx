import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { TasksPagination } from "./tasks-pagination";

afterEach(cleanup);

function PaginationExample({ initialPage = 3 }: { initialPage?: number }) {
  const [pagination, setPagination] = useState({ pageIndex: initialPage, pageSize: 10 });
  return (
    <TasksPagination
      total={70}
      pageCount={Math.ceil(70 / pagination.pageSize)}
      pagination={pagination}
      onPaginationChange={setPagination}
    />
  );
}

// @covers AC-UI-TASK-PAGINATION-PRESENTATION-001.1
it("updates range and current page through previous, next and direct selection", () => {
  render(<PaginationExample />);
  expect(screen.getByText("Showing 31 to 40 of 70 results")).toBeTruthy();
  expect(screen.getByRole("button", { name: "4" }).getAttribute("aria-current")).toBe("page");
  fireEvent.click(screen.getByRole("button", { name: "Go to next page" }));
  expect(screen.getByText("Showing 41 to 50 of 70 results")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Go to previous page" }));
  fireEvent.click(screen.getByRole("button", { name: "3" }));
  expect(screen.getByText("Showing 21 to 30 of 70 results")).toBeTruthy();
});

it("disables boundary actions and resets the page when the page size changes", () => {
  render(<PaginationExample initialPage={0} />);
  expect(screen.getByRole("button", { name: "Go to previous page" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("button", { name: "Go to first page" })).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("button", { name: "Go to last page" }));
  expect(screen.getByRole("button", { name: "Go to next page" })).toHaveProperty("disabled", true);
  expect(screen.getByRole("button", { name: "Go to last page" })).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByTestId("tasks-pagination-page-size"));
  fireEvent.click(screen.getByRole("option", { name: "25" }));
  expect(screen.getByText("Showing 1 to 25 of 70 results")).toBeTruthy();
});
