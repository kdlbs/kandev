import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SidebarTaskPageResponse } from "@/lib/types/http";
import { SidebarTaskPagination } from "./sidebar-task-pagination";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const PAGE_CONTROLS = "sidebar-page-controls";
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
    expect(screen.queryByTestId(PAGE_CONTROLS)).toBeNull();

    rerender(<SidebarTaskPagination page={response(100)} {...props} />);
    expect(screen.queryByTestId(PAGE_CONTROLS)).toBeNull();

    rerender(<SidebarTaskPagination page={response(101)} {...props} />);
    expect(screen.getByTestId(PAGE_CONTROLS)).not.toBeNull();
  });

  it("shows plugin totals without disabled paging buttons through 100 occurrences", () => {
    const props = { pending: false, onPageChange: vi.fn() };
    const { rerender } = render(
      <SidebarTaskPagination page={{ ...response(100), grouping_revision: "r1" }} {...props} />,
    );
    expect(screen.getByTestId(PAGE_CONTROLS)).not.toBeNull();
    expect(screen.queryByRole("button", { name: "sidebar:previousPage" })).toBeNull();
    expect(screen.queryByRole("button", { name: "sidebar:nextPage" })).toBeNull();
    rerender(
      <SidebarTaskPagination page={{ ...response(101), grouping_revision: "r1" }} {...props} />,
    );
    expect(screen.getByRole("button", { name: "sidebar:nextPage" })).not.toBeNull();
    rerender(
      <SidebarTaskPagination page={{ ...response(0), grouping_revision: "r1" }} {...props} />,
    );
    expect(screen.queryByRole("button", { name: "sidebar:nextPage" })).toBeNull();
  });
});
