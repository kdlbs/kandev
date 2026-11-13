import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReviewImage } from "@/lib/question-review/segments";
import { ReviewImageOptions } from "./review-image-option";

afterEach(cleanup);

const images: ReviewImage[] = [
  {
    id: "before",
    alt: "Settings header before the change",
    label: "before",
    ref: { id: "before", path: "b.png", alt: "x" },
  },
  {
    id: "after",
    alt: "Settings header after the change",
    choiceLabel: "Approve",
    ref: { id: "after", path: "a.png", alt: "y" },
  },
];

describe("ReviewImageOptions", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-009.1
  it("shows thumbnail, nickname, description, choice, and slot markers", () => {
    const onPick = vi.fn();
    render(
      <ReviewImageOptions
        slot="b"
        images={images}
        aId="before"
        bId="after"
        urls={{ before: "blob:before" }}
        onPick={onPick}
      />,
    );
    const rows = screen.getAllByTestId("question-review-pick-b-option");
    expect(rows).toHaveLength(2);
    expect(rows[0].querySelector("img")?.getAttribute("src")).toBe("blob:before");
    expect(rows[0].textContent).toContain("before");
    expect(rows[0].textContent).toContain("Settings header before the change");
    expect(rows[0].textContent).toContain("In A");
    expect(rows[1].querySelector("img")).toBeNull();
    expect(rows[1].textContent).toContain("2");
    expect(rows[1].textContent).toContain("Choice: Approve");
    expect(rows[1].getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(rows[0]);
    expect(onPick).toHaveBeenCalledWith("b", "before");
  });
});
