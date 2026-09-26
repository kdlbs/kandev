import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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
      error: null,
      onPageChange: vi.fn(),
      onRetry: vi.fn(),
    };
    const { rerender } = render(<SidebarTaskPagination page={response(99)} {...props} />);
    expect(screen.queryByTestId("sidebar-page-controls")).toBeNull();

    rerender(<SidebarTaskPagination page={response(100)} {...props} />);
    expect(screen.queryByTestId("sidebar-page-controls")).toBeNull();

    rerender(<SidebarTaskPagination page={response(101)} {...props} />);
    expect(screen.getByTestId("sidebar-page-controls")).not.toBeNull();
  });

  it("provides reachable phone controls and keeps Retry separate from paging", () => {
    const onPageChange = vi.fn();
    const onRetry = vi.fn();
    render(
      <SidebarTaskPagination
        page={response(101)}
        pending={false}
        error="network"
        onPageChange={onPageChange}
        onRetry={onRetry}
        touchTargets
      />,
    );

    const next = screen.getByRole("button", { name: "sidebar:nextPage" });
    expect(next.className).toContain("min-h-11");
    fireEvent.click(next);
    expect(onPageChange).toHaveBeenCalledWith(2);
    expect(screen.getByRole("alert").textContent).toContain("sidebar:pageLoadFailed");
    fireEvent.click(screen.getByRole("button", { name: "sidebar:retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
    expect(onPageChange).toHaveBeenCalledOnce();
  });
});
