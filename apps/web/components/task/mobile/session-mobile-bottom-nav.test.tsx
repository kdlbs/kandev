import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SessionMobileBottomNav } from "./session-mobile-bottom-nav";

afterEach(cleanup);

describe("SessionMobileBottomNav GitLab review", () => {
  it("offers a touch-sized review route for linked merge requests", () => {
    const onPanelChange = vi.fn();
    render(<SessionMobileBottomNav activePanel="chat" onPanelChange={onPanelChange} hasReview />);

    const review = screen.getByRole("button", { name: "Review" });
    expect(review.className).toContain("min-h-11");
    fireEvent.click(review);
    expect(onPanelChange).toHaveBeenCalledWith("review");
  });

  it("does not consume navigation space without a linked merge request", () => {
    render(<SessionMobileBottomNav activePanel="chat" onPanelChange={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Review" })).toBeNull();
  });
});
