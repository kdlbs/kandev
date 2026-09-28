import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SidebarTaskQueryStatus } from "./sidebar-task-query-status";
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(cleanup);
it("renders one actionable query error and disables duplicate retries while pending", () => {
  const retry = vi.fn();
  const props = {
    pending: false,
    hasPage: true,
    onRetry: retry,
    error: "Localized read error",
    touchTargets: true,
  };
  const { rerender } = render(<SidebarTaskQueryStatus {...props} />);
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  const button = screen.getByRole("button", { name: "sidebar:retry" });
  expect(button.className).toContain("min-h-11");
  fireEvent.click(button);
  expect(retry).toHaveBeenCalledOnce();
  rerender(<SidebarTaskQueryStatus {...props} pending />);
  expect((button as HTMLButtonElement).disabled).toBe(true);
  rerender(<SidebarTaskQueryStatus {...props} canRetry={false} />);
  expect(screen.queryByRole("button")).toBeNull();
});
it("announces background refresh without a blocking loading surface", () => {
  const { rerender } = render(
    <SidebarTaskQueryStatus error={null} pending hasPage onRetry={vi.fn()} />,
  );
  expect(screen.getByRole("status").textContent).toBe("sidebar:queryRefreshing");
  rerender(<SidebarTaskQueryStatus error={null} pending={false} hasPage onRetry={vi.fn()} />);
  expect(screen.queryByRole("status")).toBeNull();
});
