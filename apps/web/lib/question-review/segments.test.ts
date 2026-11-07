import { describe, expect, it } from "vitest";
import type { ClarificationQuestion } from "@/lib/types/http";
import {
  buildReviewSegments,
  buildSegmentImages,
  hasReviewImages,
  segmentImageIndex,
} from "./segments";

const before = { id: "before", path: "before.png", alt: "Before" };
const after = { id: "after", path: "after.png", alt: "After" };

function question(overrides: Partial<ClarificationQuestion> = {}): ClarificationQuestion {
  return {
    id: "q1",
    title: "",
    prompt: "Is the  new\nspacing acceptable?",
    options: [
      { option_id: "o1", label: "Approve", description: "" },
      { option_id: "o2", label: "Revise", description: "" },
    ],
    ...overrides,
  };
}

describe("buildSegmentImages", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-003.2
  it("orders question images first, then choice images in option order with labels", () => {
    const q = question({
      images: [before],
      options: [
        { option_id: "o1", label: "Approve", description: "", images: [after] },
        { option_id: "o2", label: "Revise", description: "" },
      ],
    });
    expect(buildSegmentImages(q)).toEqual([
      { id: "before", alt: "Before", ref: before },
      { id: "after", alt: "After", choiceLabel: "Approve", ref: after },
    ]);
  });

  it("keeps a repeated image once, at its first placement", () => {
    const q = question({
      images: [before],
      options: [{ option_id: "o1", label: "Approve", description: "", images: [before] }],
    });
    expect(buildSegmentImages(q).map((image) => image.id)).toEqual(["before"]);
  });

  it("ignores malformed image entries and caps a segment at four images", () => {
    const malformed = [null, { id: 3 }, before] as unknown as ClarificationQuestion["images"];
    const many = ["a", "b", "c", "d", "e"].map((id) => ({ id, path: `${id}.png`, alt: id }));
    expect(buildSegmentImages(question({ images: malformed })).map((i) => i.id)).toEqual([
      "before",
    ]);
    expect(buildSegmentImages(question({ images: many }))).toHaveLength(4);
  });
});

describe("buildReviewSegments", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-003.1
  it("creates one segment per question in order, labelled by title or prompt", () => {
    const segments = buildReviewSegments([
      { questionId: "q1", question: question({ title: "Header", images: [before] }) },
      { questionId: "q2", question: question({ id: "q2" }) },
    ]);
    expect(segments.map((s) => [s.index, s.questionId, s.label, s.images.length])).toEqual([
      [0, "q1", "Header", 1],
      [1, "q2", "Is the new spacing acceptable?", 0],
    ]);
  });

  it("truncates long prompt labels", () => {
    const [segment] = buildReviewSegments([
      { questionId: "q1", question: question({ prompt: "x".repeat(200) }) },
    ]);
    expect(segment.label.length).toBeLessThanOrEqual(48);
    expect(segment.label.endsWith("…")).toBe(true);
  });
});

describe("hasReviewImages", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.1
  it("is true for question or choice images and false for image-free bundles", () => {
    expect(hasReviewImages([question()])).toBe(false);
    expect(hasReviewImages([question(), question({ images: [before] })])).toBe(true);
    expect(
      hasReviewImages([
        question({
          options: [{ option_id: "o1", label: "A", description: "", images: [after] }],
        }),
      ]),
    ).toBe(true);
  });
});

describe("segmentImageIndex", () => {
  it("returns the one-based stage position of an image", () => {
    const images = buildSegmentImages(question({ images: [before, after] }));
    expect(segmentImageIndex(images, "after")).toBe(2);
    expect(segmentImageIndex(images, "missing")).toBe(0);
  });
});
