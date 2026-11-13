import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { ClarificationQuestion } from "@/lib/types/http";
import { AnswerRegionSummary } from "./answer-region-summary";

afterEach(cleanup);

const question: ClarificationQuestion = {
  id: "header",
  title: "",
  prompt: "Header?",
  images: [{ id: "before", path: "before.png", alt: "Before" }],
  options: [
    {
      option_id: "o1",
      label: "Approve",
      description: "",
      images: [{ id: "after", path: "after.png", alt: "After" }],
    },
  ],
};

const region = {
  image_id: "after",
  x: 1,
  y: 2,
  width: 3,
  height: 4,
  image_width: 10,
  image_height: 10,
  comment: "<b>Contrast</b> too low",
};

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.6
it("lists each region with its number, image description, and plain-text comment", () => {
  render(
    <AnswerRegionSummary
      question={question}
      annotations={[region, { ...region, image_id: "missing", comment: "Gone" }]}
    />,
  );
  const items = screen.getAllByRole("listitem").map((item) => item.textContent);
  expect(items).toEqual([
    "Region 1 on After: <b>Contrast</b> too low",
    "Region 2 on missing: Gone",
  ]);
  expect(screen.queryByText("Contrast", { selector: "b" })).toBeNull();
});

it("renders nothing without regions and skips malformed entries", () => {
  const { container } = render(<AnswerRegionSummary question={question} annotations={undefined} />);
  expect(container.textContent).toBe("");
  render(
    <AnswerRegionSummary
      question={question}
      annotations={[null, { image_id: "after" }] as unknown as (typeof region)[]}
    />,
  );
  expect(screen.queryAllByRole("listitem")).toHaveLength(0);
});
