import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ResultsPagination } from "./results-pagination";

const CHOOSE_PAGE = "Choose results page";
let mobile = true;
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mobile }),
}));
beforeEach(() => {
  mobile = true;
});
afterEach(cleanup);

// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.4
describe("phone pagination", () => {
  it("keeps focus in the page drawer when the current page disappears after a refresh", async () => {
    const onPageChange = vi.fn();
    const { rerender } = render(
      <ResultsPagination page={40} pageSize={25} total={1000} onPageChange={onPageChange} />,
    );
    rerender(<ResultsPagination page={40} pageSize={25} total={50} onPageChange={onPageChange} />);
    fireEvent.click(screen.getByRole("button", { name: CHOOSE_PAGE }));
    const dialog = screen.getByRole("dialog", { name: CHOOSE_PAGE });
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
    expect(onPageChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Page 2 of 2" }));
    expect(onPageChange).toHaveBeenCalledExactlyOnceWith(2);
  });

  it("offers direct selection up to GitHub's 1000-result cap", () => {
    const onPageChange = vi.fn();
    render(<ResultsPagination page={1} pageSize={25} total={1700} onPageChange={onPageChange} />);
    const selector = screen.getByLabelText(CHOOSE_PAGE, { selector: "button,select" });
    expect(selector.tagName).toBe("BUTTON");
    expect(screen.getByText("1–25 of 1000+")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Previous page" }).hasAttribute("disabled")).toBe(
      true,
    );
    fireEvent.click(selector);
    expect(screen.getByRole("dialog", { name: CHOOSE_PAGE })).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /^Page \d+ of 40$/ })).toHaveLength(40);
    fireEvent.click(screen.getByRole("button", { name: "Page 5 of 40" }));
    expect(onPageChange).toHaveBeenCalledExactlyOnceWith(5);
    expect(selector.getAttribute("aria-expanded")).toBe("false");
  });

  it("bounds next and previous at the last available page", () => {
    const onPageChange = vi.fn();
    render(<ResultsPagination page={40} pageSize={25} total={1700} onPageChange={onPageChange} />);
    expect(screen.getByLabelText(CHOOSE_PAGE, { selector: "button,select" }).textContent).toBe(
      "Page 40 of 40",
    );
    expect(screen.getByRole("button", { name: "Next page" }).hasAttribute("disabled")).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
    expect(onPageChange).toHaveBeenCalledExactlyOnceWith(39);
  });

  it("marks the current page and dismisses without fetching it again", () => {
    const onPageChange = vi.fn();
    render(<ResultsPagination page={5} pageSize={25} total={1700} onPageChange={onPageChange} />);
    const selector = screen.getByLabelText(CHOOSE_PAGE, { selector: "button,select" });
    fireEvent.click(selector);
    const current = screen.getByRole("button", { name: "Page 5 of 40" });
    expect(current.getAttribute("aria-current")).toBe("page");
    fireEvent.click(current);
    expect(selector.getAttribute("aria-expanded")).toBe("false");
    expect(onPageChange).not.toHaveBeenCalled();
  });

  it("retains numbered pagination on desktop", () => {
    mobile = false;
    render(<ResultsPagination page={5} pageSize={25} total={1700} onPageChange={vi.fn()} />);
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.getByRole("link", { name: "40" })).not.toBeNull();
  });
});
