import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SidebarTaskPageResponse } from "@/lib/types/http";
import { SidebarTaskPagination } from "./sidebar-task-pagination";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

afterEach(cleanup);

function response(total: number): SidebarTaskPageResponse {
  return {
    query_key: "query",
    page: 1,
    page_size: 100,
    total_entries: total + 1,
    total_tasks: total,
    total_visible_tasks: total,
    has_previous: false,
    has_next: total > 100,
    entries: [],
  };
}

describe("SidebarTaskPagination", () => {
  it("hides controls through 100 task rows, regardless of entry headings", () => {
    const props = {
      pending: false,
      onPageChange: vi.fn(),
    };
    const { rerender } = render(<SidebarTaskPagination page={response(99)} {...props} />);
    expect(screen.queryByTestId("sidebar-page-controls")).toBeNull();

    rerender(<SidebarTaskPagination page={response(100)} {...props} />);
    expect(screen.queryByTestId("sidebar-page-controls")).toBeNull();

    rerender(<SidebarTaskPagination page={response(101)} {...props} />);
    expect(screen.getByTestId("sidebar-page-controls")).not.toBeNull();
  });
});
