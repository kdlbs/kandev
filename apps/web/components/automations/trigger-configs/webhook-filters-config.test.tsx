import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { WebhookFilter } from "@/lib/types/automation";
import { WebhookFiltersConfig } from "./webhook-filters-config";

beforeAll(() => {
  // Radix Select needs these in jsdom; the repo does not otherwise polyfill them.
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

afterEach(cleanup);

function renderFilters(filters: WebhookFilter[]) {
  const onChange = vi.fn();
  render(<WebhookFiltersConfig filters={filters} onChange={onChange} />);
  return onChange;
}

describe("WebhookFiltersConfig", () => {
  it("renders one row per filter with its path and operator label", () => {
    renderFilters([{ path: "severity", op: "eq", values: ["critical"] }]);

    expect(screen.getByDisplayValue("severity")).toBeInstanceOf(HTMLInputElement);
    expect(screen.getByText("Equals")).toBeTruthy();
    expect(screen.getByDisplayValue("critical")).toBeInstanceOf(HTMLInputElement);
  });

  it("appends a blank eq filter when 'Add filter' is clicked", () => {
    const onChange = renderFilters([]);

    fireEvent.click(screen.getByText("Add filter"));

    expect(onChange).toHaveBeenCalledWith([{ path: "", op: "eq", values: [] }]);
  });

  it("removes only the targeted filter", () => {
    const onChange = renderFilters([
      { path: "severity", op: "eq", values: ["critical"] },
      { path: "service", op: "eq", values: ["api"] },
    ]);

    fireEvent.click(screen.getAllByTitle("Remove filter")[0]);

    expect(onChange).toHaveBeenCalledWith([{ path: "service", op: "eq", values: ["api"] }]);
  });

  it("commits path edits immediately", () => {
    const onChange = renderFilters([{ path: "severity", op: "eq", values: [] }]);

    fireEvent.change(screen.getByDisplayValue("severity"), { target: { value: "status" } });

    expect(onChange).toHaveBeenCalledWith([{ path: "status", op: "eq", values: [] }]);
  });

  it("switches the operator via the select and preserves the path", () => {
    const onChange = renderFilters([{ path: "severity", op: "eq", values: ["critical"] }]);

    fireEvent.click(screen.getByText("Equals"));
    fireEvent.click(screen.getByText("Not equals"));

    expect(onChange).toHaveBeenCalledWith([{ path: "severity", op: "ne", values: ["critical"] }]);
  });

  it("hides the values input for exists and not_exists operators", () => {
    renderFilters([{ path: "severity", op: "exists" }]);

    expect(screen.queryByPlaceholderText("critical, fatal")).toBeNull();
  });

  it("commits comma-separated values as a trimmed array on blur", () => {
    const onChange = renderFilters([{ path: "severity", op: "in", values: [] }]);

    const input = screen.getByPlaceholderText("critical, fatal");
    fireEvent.change(input, { target: { value: "critical, fatal ," } });
    fireEvent.blur(input);

    expect(onChange).toHaveBeenCalledWith([
      { path: "severity", op: "in", values: ["critical", "fatal"] },
    ]);
  });
});
