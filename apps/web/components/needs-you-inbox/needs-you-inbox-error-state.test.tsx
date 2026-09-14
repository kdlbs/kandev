import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { NeedsYouInboxErrorState } from "./needs-you-inbox-error-state";

afterEach(() => cleanup());

describe("NeedsYouInboxErrorState", () => {
  it("renders as an alert with a retry action (AC .21)", () => {
    render(<NeedsYouInboxErrorState onRetry={vi.fn()} />);

    expect(screen.getByRole("alert")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Retry" })).not.toBeNull();
  });

  it("calls onRetry when the retry button is clicked", () => {
    const onRetry = vi.fn();
    render(<NeedsYouInboxErrorState onRetry={onRetry} />);

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(onRetry).toHaveBeenCalledOnce();
  });
});
