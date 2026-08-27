import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationCursorPagination } from "./integration-cursor-pagination";

afterEach(cleanup);

describe("IntegrationCursorPagination", () => {
  it("provides stable previous and next controls without inventing a total", () => {
    const onPrevious = vi.fn();
    const onNext = vi.fn();
    render(
      <IntegrationCursorPagination
        page={2}
        itemCount={25}
        hasPrevious
        hasNext
        onPrevious={onPrevious}
        onNext={onNext}
      />,
    );

    expect(screen.getByText("Page 2 · 25 results")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Go to previous page"));
    fireEvent.click(screen.getByLabelText("Go to next page"));
    expect(onPrevious).toHaveBeenCalledOnce();
    expect(onNext).toHaveBeenCalledOnce();
  });

  it("hides when the first cursor page has no continuation", () => {
    const { container } = render(
      <IntegrationCursorPagination
        page={1}
        itemCount={3}
        hasPrevious={false}
        hasNext={false}
        onPrevious={() => undefined}
        onNext={() => undefined}
      />,
    );
    expect(container.innerHTML).toBe("");
  });
});
